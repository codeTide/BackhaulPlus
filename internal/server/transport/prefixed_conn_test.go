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
