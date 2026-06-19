package transport

import (
	"bytes"
	"testing"
)

func TestPrefixedConn_ReadsPrefixThenConn(t *testing.T) {
	conn := newMemConn([]byte("WORLD"))
	pc := NewPrefixedConn(conn, []byte("HELLO"))

	got := make([]byte, len("HELLOWORLD"))
	if _, err := readFull(pc, got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "HELLOWORLD" {
		t.Fatalf("expected HELLOWORLD, got %q", string(got))
	}
}

func TestPrefixedConn_EmptyPrefixReturnsConn(t *testing.T) {
	conn := newMemConn([]byte("data"))
	pc := NewPrefixedConn(conn, nil)
	if pc != conn {
		t.Fatal("expected NewPrefixedConn with empty prefix to return the original conn")
	}
}

func TestPrefixedConn_PartialPrefixReads(t *testing.T) {
	conn := newMemConn([]byte("XY"))
	pc := NewPrefixedConn(conn, []byte("ABCD"))

	// Read in small chunks to exercise the prefix offset handling.
	buf := make([]byte, 2)
	n, err := pc.Read(buf)
	if err != nil || n != 2 || string(buf[:n]) != "AB" {
		t.Fatalf("first read: n=%d err=%v data=%q", n, err, string(buf[:n]))
	}
	n, err = pc.Read(buf)
	if err != nil || n != 2 || string(buf[:n]) != "CD" {
		t.Fatalf("second read: n=%d err=%v data=%q", n, err, string(buf[:n]))
	}
	n, err = pc.Read(buf)
	if err != nil || string(buf[:n]) != "XY" {
		t.Fatalf("third read: n=%d err=%v data=%q", n, err, string(buf[:n]))
	}
}

func TestPrefixedConn_PrefixReleasedAfterConsumption(t *testing.T) {
	conn := newMemConn([]byte("XY"))
	pc := NewPrefixedConn(conn, []byte("ABCD")).(*PrefixedConn)

	// Drain the entire prefix.
	buf := make([]byte, 4)
	if _, err := readFull(pc, buf); err != nil {
		t.Fatalf("unexpected error draining prefix: %v", err)
	}
	if string(buf) != "ABCD" {
		t.Fatalf("expected ABCD, got %q", string(buf))
	}
	if pc.prefix != nil {
		t.Fatalf("expected prefix to be released after consumption, got %q", string(pc.prefix))
	}
	if pc.off != 0 {
		t.Fatalf("expected off to be reset to 0, got %d", pc.off)
	}

	// Subsequent reads must come from the underlying connection.
	n, err := pc.Read(buf)
	if err != nil || string(buf[:n]) != "XY" {
		t.Fatalf("expected to read XY from underlying conn: n=%d err=%v data=%q", n, err, string(buf[:n]))
	}
}

func TestPrefixedConn_DoesNotAliasCallerPrefix(t *testing.T) {
	conn := newMemConn([]byte("Z"))
	orig := []byte("ABCD")
	pc := NewPrefixedConn(conn, orig).(*PrefixedConn)

	// Mutating the caller's slice must not affect the wrapped prefix.
	orig[0] = 'X'

	buf := make([]byte, 4)
	if _, err := readFull(pc, buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(buf) != "ABCD" {
		t.Fatalf("prefix was aliased to caller slice: got %q", string(buf))
	}
}

func TestPrefixedConn_DelegatesWriteAndClose(t *testing.T) {
	conn := newMemConn([]byte("rest"))
	pc := NewPrefixedConn(conn, []byte("p"))

	if _, err := pc.Write([]byte("payload")); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if !bytes.Equal(conn.written.Bytes(), []byte("payload")) {
		t.Fatalf("write was not delegated, got %q", conn.written.String())
	}

	if err := pc.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
	if !conn.closed {
		t.Fatal("close was not delegated to underlying conn")
	}

	// Address methods should delegate too.
	if pc.LocalAddr().String() != conn.LocalAddr().String() {
		t.Fatal("LocalAddr not delegated")
	}
	if pc.RemoteAddr().String() != conn.RemoteAddr().String() {
		t.Fatal("RemoteAddr not delegated")
	}
}
