package transport

import (
	"bytes"
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
