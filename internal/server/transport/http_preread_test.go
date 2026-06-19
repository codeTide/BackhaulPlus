package transport

import (
	"bytes"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

// httpRequest builds a cleartext HTTP request with CRLF line endings.
func httpRequest(lines ...string) []byte {
	return []byte(strings.Join(lines, "\r\n") + "\r\n\r\n")
}

func TestReadHTTPHost_SimpleGET(t *testing.T) {
	req := httpRequest("GET / HTTP/1.1", "Host: tr.example.com")
	conn := newMemConn(req)

	host, first, err := ReadHTTPHost(conn, time.Second, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "tr.example.com" {
		t.Fatalf("expected host tr.example.com, got %q", host)
	}
	if !bytes.Equal(first, req) {
		t.Fatalf("firstBytes mismatch:\n got: %q\nwant: %q", first, req)
	}
}

func TestReadHTTPHost_PostXHTTP(t *testing.T) {
	req := httpRequest("POST /xhttp HTTP/1.1", "Host: tr.example.com", "Content-Length: 0")
	host, _, err := ReadHTTPHost(newMemConn(req), time.Second, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "tr.example.com" {
		t.Fatalf("expected host tr.example.com, got %q", host)
	}
}

func TestReadHTTPHost_CaseInsensitive(t *testing.T) {
	for _, hdr := range []string{"Host: a.com", "host: a.com", "HOST: a.com", "HoSt: a.com"} {
		req := httpRequest("GET / HTTP/1.1", hdr)
		host, _, err := ReadHTTPHost(newMemConn(req), time.Second, 0)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", hdr, err)
		}
		if host != "a.com" {
			t.Fatalf("expected a.com for %q, got %q", hdr, host)
		}
	}
}

func TestReadHTTPHost_PortAndTrailingDotNormalized(t *testing.T) {
	cases := map[string]string{
		"tr.example.com:443": "tr.example.com",
		"us.example.com:80":  "us.example.com",
		"Example.COM.":       "example.com",
		"Example.COM:443":    "example.com",
	}
	for in, want := range cases {
		req := httpRequest("GET / HTTP/1.1", "Host: "+in)
		host, _, err := ReadHTTPHost(newMemConn(req), time.Second, 0)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", in, err)
		}
		if host != want {
			t.Fatalf("Host %q normalized to %q, want %q", in, host, want)
		}
	}
}

func TestReadHTTPHost_MissingHostRejected(t *testing.T) {
	req := httpRequest("GET / HTTP/1.1", "User-Agent: x")
	if _, _, err := ReadHTTPHost(newMemConn(req), time.Second, 0); err == nil {
		t.Fatal("expected error for missing Host header")
	}
}

func TestReadHTTPHost_DuplicateHostRejected(t *testing.T) {
	req := httpRequest("GET / HTTP/1.1", "Host: a.com", "Host: b.com")
	if _, _, err := ReadHTTPHost(newMemConn(req), time.Second, 0); err == nil {
		t.Fatal("expected error for duplicate Host headers")
	}
}

func TestReadHTTPHost_MalformedRequestLineRejected(t *testing.T) {
	for _, bad := range []string{"GARBAGE", "GET /", "GET / HTTP/1.1 extra"} {
		req := httpRequest(bad, "Host: a.com")
		if _, _, err := ReadHTTPHost(newMemConn(req), time.Second, 0); err == nil {
			t.Fatalf("expected error for malformed request line %q", bad)
		}
	}
}

func TestReadHTTPHost_HTTP2PrefaceRejected(t *testing.T) {
	// HTTP/2 connection preface; reader stops at the first CRLFCRLF.
	req := []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")
	if _, _, err := ReadHTTPHost(newMemConn(req), time.Second, 0); err == nil {
		t.Fatal("expected error for HTTP/2 preface")
	}
}

func TestReadHTTPHost_HeaderTooLargeRejected(t *testing.T) {
	req := []byte("GET / HTTP/1.1\r\nHost: " + strings.Repeat("a", 300) + "\r\n\r\n")
	if _, _, err := ReadHTTPHost(newMemConn(req), time.Second, 128); err == nil {
		t.Fatal("expected error for header exceeding max_header_bytes")
	}
}

func TestReadHTTPHost_IncompleteHeaderTimeout(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	// Write a partial header that never terminates with CRLFCRLF.
	go func() {
		_, _ = c2.Write([]byte("GET / HTTP/1.1\r\nHost: a.com\r\n"))
	}()

	if _, _, err := ReadHTTPHost(c1, 200*time.Millisecond, 0); err == nil {
		t.Fatal("expected timeout error for incomplete header")
	}
}

func TestReadHTTPHost_PostXHTTPHostNormalized(t *testing.T) {
	req := httpRequest("POST /xhttp HTTP/1.1", "Host: Example.COM:443", "User-Agent: test")
	host, _, err := ReadHTTPHost(newMemConn(req), time.Second, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "example.com" {
		t.Fatalf("expected example.com, got %q", host)
	}
}

// TestReadHTTPHost_PreservesReadAheadBody ensures that when a chunk reads part of
// the request body past the header terminator, those bytes are included in
// firstBytes and replayed downstream verbatim via PrefixedConn.
func TestReadHTTPHost_PreservesReadAheadBody(t *testing.T) {
	body := []byte("hello-body-bytes")
	req := append(httpRequest("POST /xhttp HTTP/1.1", "Host: Example.COM:443", "Content-Length: 16"), body...)
	conn := newMemConn(req)

	host, first, err := ReadHTTPHost(conn, time.Second, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "example.com" {
		t.Fatalf("expected example.com, got %q", host)
	}

	// Replay through PrefixedConn and confirm the exact original byte stream
	// (header + read-ahead body) is delivered downstream.
	wrapped := NewPrefixedConn(conn, first)
	got := make([]byte, len(req))
	if _, err := readFull(wrapped, got); err != nil {
		t.Fatalf("unexpected error reading replayed stream: %v", err)
	}
	if !bytes.Equal(got, req) {
		t.Fatalf("replayed stream mismatch:\n got: %q\nwant: %q", got, req)
	}
}

func TestReadHTTPHost_ErrorIdentities(t *testing.T) {
	cases := []struct {
		name string
		req  []byte
		want error
	}{
		{"missing host", httpRequest("GET / HTTP/1.1", "User-Agent: x"), errNoHostHeader},
		{"duplicate host", httpRequest("GET / HTTP/1.1", "Host: a.com", "Host: b.com"), errMultipleHostHeaders},
		{"empty host", httpRequest("GET / HTTP/1.1", "Host:   "), errEmptyHost},
		{"http2 preface", []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"), errUnsupportedHTTPVersion},
		{"malformed request line", httpRequest("GARBAGE", "Host: a.com"), errMalformedRequestLine},
	}
	for _, tc := range cases {
		_, _, err := ReadHTTPHost(newMemConn(tc.req), time.Second, 0)
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, err)
		}
	}
}

func TestReadHTTPHost_HeaderTooLargeNoTerminator(t *testing.T) {
	// A header that never terminates and grows past maxHeaderBytes.
	req := []byte("GET / HTTP/1.1\r\nHost: " + strings.Repeat("a", 300))
	_, _, err := ReadHTTPHost(newMemConn(req), time.Second, 128)
	if !errors.Is(err, errHTTPHeaderTooLarge) {
		t.Fatalf("expected errHTTPHeaderTooLarge, got %v", err)
	}
}

func TestReadHTTPHost_TerminatorBeyondMaxRejected(t *testing.T) {
	req := []byte("GET / HTTP/1.1\r\nHost: " + strings.Repeat("a", 300) + "\r\n\r\n")
	_, _, err := ReadHTTPHost(newMemConn(req), time.Second, 128)
	if !errors.Is(err, errHTTPHeaderTooLarge) {
		t.Fatalf("expected errHTTPHeaderTooLarge, got %v", err)
	}
}

// TestReadHTTPHost_TLSRejectedEarly verifies a TLS record on the cleartext HTTP
// gateway is rejected as soon as the first byte arrives, not after the inspect
// timeout. It uses a long timeout and a never-completing peer; a correct
// implementation returns well before the deadline.
func TestReadHTTPHost_TLSRejectedEarly(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	// TLS handshake record header (ClientHello). No terminator will ever arrive.
	go func() {
		_, _ = c2.Write([]byte{0x16, 0x03, 0x01, 0x00, 0x05, 0x01, 0x00})
	}()

	start := time.Now()
	_, _, err := ReadHTTPHost(c1, 10*time.Second, 0)
	elapsed := time.Since(start)
	if !errors.Is(err, errNonHTTPTraffic) {
		t.Fatalf("expected errNonHTTPTraffic, got %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("expected early reject, took %v", elapsed)
	}
}

func TestNormalizeHTTPHost(t *testing.T) {
	cases := map[string]string{
		"  Example.COM  ":   "example.com",
		"example.com:443":   "example.com",
		"example.com.":      "example.com",
		"example.com.:8080": "example.com",
		"":                  "",
	}
	for in, want := range cases {
		if got := NormalizeHTTPHost(in); got != want {
			t.Errorf("NormalizeHTTPHost(%q) = %q, want %q", in, got, want)
		}
	}
}

// BenchmarkReadHTTPHost measures the cost of preread + Host parsing on a
// realistic XHTTP request header. It reuses a single in-memory reader so the
// benchmark reflects the preread/parse work rather than goroutine scheduling.
//
//	go test ./internal/server/transport -bench=ReadHTTPHost -benchmem
func BenchmarkReadHTTPHost(b *testing.B) {
	req := httpRequest(
		"POST /xhttp/abcdef0123456789abcdef HTTP/1.1",
		"Host: tr.example.com:443",
		"User-Agent: Mozilla/5.0 (X11; Linux x86_64) realistic-xhttp-client/1.0",
		"Accept: */*",
		"Accept-Encoding: gzip, deflate, br",
		"Content-Type: application/octet-stream",
		"Cache-Control: no-store",
		"X-Forwarded-For: 203.0.113.7",
		"X-Padding: "+strings.Repeat("a", 400),
	)
	conn := newMemConn(req)

	b.ReportAllocs()
	b.SetBytes(int64(len(req)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn.r.Reset(req)
		if _, _, err := ReadHTTPHost(conn, time.Second, 0); err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}
