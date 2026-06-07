package transport

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// --- test helpers -----------------------------------------------------------

type dummyAddr struct{}

func (dummyAddr) Network() string { return "tcp" }
func (dummyAddr) String() string  { return "127.0.0.1:0" }

// memConn is a net.Conn backed by an in-memory reader (for Read) that records
// writes. Deadlines are no-ops.
type memConn struct {
	r       *bytes.Reader
	written bytes.Buffer
	closed  bool
}

func newMemConn(data []byte) *memConn { return &memConn{r: bytes.NewReader(data)} }

func (m *memConn) Read(b []byte) (int, error)         { return m.r.Read(b) }
func (m *memConn) Write(b []byte) (int, error)        { return m.written.Write(b) }
func (m *memConn) Close() error                       { m.closed = true; return nil }
func (m *memConn) LocalAddr() net.Addr                { return dummyAddr{} }
func (m *memConn) RemoteAddr() net.Addr               { return dummyAddr{} }
func (m *memConn) SetDeadline(t time.Time) error      { return nil }
func (m *memConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *memConn) SetWriteDeadline(t time.Time) error { return nil }

// buildClientHello constructs a minimal but valid TLS ClientHello record. If
// sni is empty, no server_name extension is included.
func buildClientHello(sni string) []byte {
	var ext []byte
	if sni != "" {
		name := []byte(sni)
		serverNameList := make([]byte, 0, 3+len(name))
		serverNameList = append(serverNameList, 0x00) // host_name type
		var nl [2]byte
		binary.BigEndian.PutUint16(nl[:], uint16(len(name)))
		serverNameList = append(serverNameList, nl[:]...)
		serverNameList = append(serverNameList, name...)

		var listLen [2]byte
		binary.BigEndian.PutUint16(listLen[:], uint16(len(serverNameList)))
		extData := append(listLen[:], serverNameList...)

		ext = append(ext, 0x00, 0x00) // extension type: server_name
		var el [2]byte
		binary.BigEndian.PutUint16(el[:], uint16(len(extData)))
		ext = append(ext, el[:]...)
		ext = append(ext, extData...)
	}

	body := make([]byte, 0, 64)
	body = append(body, 0x03, 0x03)             // client_version TLS1.2
	body = append(body, make([]byte, 32)...)    // random
	body = append(body, 0x00)                   // session_id length 0
	body = append(body, 0x00, 0x02, 0x00, 0x2f) // cipher_suites: len 2 + one suite
	body = append(body, 0x01, 0x00)             // compression_methods: len 1 + null

	var extTotal [2]byte
	binary.BigEndian.PutUint16(extTotal[:], uint16(len(ext)))
	body = append(body, extTotal[:]...)
	body = append(body, ext...)

	// handshake message: type(1) + length(3) + body
	hs := make([]byte, 0, len(body)+4)
	hs = append(hs, 0x01) // ClientHello
	hs = append(hs, byte(len(body)>>16), byte(len(body)>>8), byte(len(body)))
	hs = append(hs, body...)

	// TLS record: type(0x16) + version(0x0301) + length(2) + handshake
	rec := make([]byte, 0, len(hs)+5)
	rec = append(rec, 0x16, 0x03, 0x01)
	rec = append(rec, byte(len(hs)>>8), byte(len(hs)))
	rec = append(rec, hs...)
	return rec
}

// --- ReadTLSClientHelloSNI tests --------------------------------------------

func TestReadTLSClientHelloSNI_Valid(t *testing.T) {
	hello := buildClientHello("myket.ir")
	conn := newMemConn(hello)

	sni, first, err := ReadTLSClientHelloSNI(conn, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sni != "myket.ir" {
		t.Fatalf("expected sni myket.ir, got %q", sni)
	}
	if !bytes.Equal(first, hello) {
		t.Fatalf("firstBytes does not match the bytes read.\n got: %x\nwant: %x", first, hello)
	}
}

func TestReadTLSClientHelloSNI_CaseInsensitive(t *testing.T) {
	hello := buildClientHello("MyKeT.IR.")
	conn := newMemConn(hello)

	sni, _, err := ReadTLSClientHelloSNI(conn, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sni != "myket.ir" {
		t.Fatalf("expected normalized sni myket.ir, got %q", sni)
	}
}

func TestReadTLSClientHelloSNI_NoSNI(t *testing.T) {
	hello := buildClientHello("")
	conn := newMemConn(hello)

	if _, _, err := ReadTLSClientHelloSNI(conn, time.Second); err == nil {
		t.Fatal("expected error for ClientHello without SNI, got nil")
	}
}

func TestReadTLSClientHelloSNI_NonTLS(t *testing.T) {
	conn := newMemConn([]byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))

	if _, _, err := ReadTLSClientHelloSNI(conn, time.Second); err == nil {
		t.Fatal("expected error for non-TLS input, got nil")
	}
}

func TestReadTLSClientHelloSNI_Malformed(t *testing.T) {
	hello := buildClientHello("myket.ir")
	conn := newMemConn(hello[:len(hello)-10]) // truncate

	if _, _, err := ReadTLSClientHelloSNI(conn, time.Second); err == nil {
		t.Fatal("expected error for truncated ClientHello, got nil")
	}
}

// --- PortFromTarget tests ----------------------------------------------------

func TestPortFromTarget(t *testing.T) {
	cases := map[string]int{
		"10001":        10001,
		" 10002 ":      10002,
		"1.1.1.1:5201": 5201,
		"example.com":  0,
		"0":            0,
		"70000":        0,
		"":             0,
	}
	for in, want := range cases {
		if got := PortFromTarget(in); got != want {
			t.Errorf("PortFromTarget(%q) = %d, want %d", in, got, want)
		}
	}
}

// --- SNI router tests --------------------------------------------------------

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate port: %v", err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

func dialWithRetry(t *testing.T, addr string) net.Conn {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			return conn
		}
		if time.Now().After(deadline) {
			t.Fatalf("failed to dial SNI router at %s: %v", addr, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStartSNIRouter_KnownSNI(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type captured struct {
		conn       net.Conn
		target     string
		reportPort int
	}
	ch := make(chan captured, 1)
	enqueue := func(conn net.Conn, target string, reportPort int) bool {
		ch <- captured{conn, target, reportPort}
		return true
	}

	addr := freeAddr(t)
	go StartSNIRouter(ctx, SNIRouterConfig{
		ListenAddr:     addr,
		InspectTimeout: time.Second,
		DefaultAction:  "reject",
		Routes:         map[string]string{"myket.ir": "10001"},
	}, logger, enqueue)

	client := dialWithRetry(t, addr)
	defer client.Close()

	hello := buildClientHello("MYKET.IR") // upper-case to exercise normalization
	if _, err := client.Write(hello); err != nil {
		t.Fatalf("failed to write ClientHello: %v", err)
	}

	select {
	case c := <-ch:
		if c.target != "10001" {
			t.Fatalf("expected target 10001, got %q", c.target)
		}
		if c.reportPort != 10001 {
			t.Fatalf("expected reportPort 10001, got %d", c.reportPort)
		}
		// The wrapped conn must replay the original ClientHello bytes.
		got := make([]byte, len(hello))
		c.conn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := readFull(c.conn, got); err != nil {
			t.Fatalf("failed to read replayed ClientHello: %v", err)
		}
		if !bytes.Equal(got, hello) {
			t.Fatalf("replayed bytes mismatch:\n got: %x\nwant: %x", got, hello)
		}
		c.conn.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("enqueue was not called for known SNI")
	}
}

func TestStartSNIRouter_UnknownSNIRejected(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan struct{}, 1)
	enqueue := func(conn net.Conn, target string, reportPort int) bool {
		ch <- struct{}{}
		return true
	}

	addr := freeAddr(t)
	go StartSNIRouter(ctx, SNIRouterConfig{
		ListenAddr:     addr,
		InspectTimeout: time.Second,
		DefaultAction:  "reject",
		Routes:         map[string]string{"myket.ir": "10001"},
	}, logger, enqueue)

	client := dialWithRetry(t, addr)
	defer client.Close()

	if _, err := client.Write(buildClientHello("unknown.example")); err != nil {
		t.Fatalf("failed to write ClientHello: %v", err)
	}

	select {
	case <-ch:
		t.Fatal("enqueue should not be called for unknown SNI")
	case <-time.After(500 * time.Millisecond):
		// expected: connection rejected, no enqueue
	}

	// The router should have closed our connection.
	client.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	if _, err := client.Read(buf); err == nil {
		t.Fatal("expected connection to be closed by router for unknown SNI")
	}
}

// readFull is a tiny io.ReadFull replacement to avoid importing io in tests.
func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
