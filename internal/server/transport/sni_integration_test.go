package transport

import (
	"bytes"
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// mockTarget is a test InboundTarget that records the connections enqueued to
// it. ready and enqueueOK make it easy to simulate "not ready" and "enqueue
// failed" conditions.
type mockTarget struct {
	mu         sync.Mutex
	ready      bool
	enqueueOK  bool
	calls      int
	lastConn   net.Conn
	lastTarget string
	lastReport int
}

func (m *mockTarget) IsReady() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ready
}

func (m *mockTarget) EnqueueInbound(conn net.Conn, target string, reportPort int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if !m.enqueueOK {
		return false
	}
	m.lastConn = conn
	m.lastTarget = target
	m.lastReport = reportPort
	return true
}

func (m *mockTarget) snapshot() (int, net.Conn, string, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls, m.lastConn, m.lastTarget, m.lastReport
}

func newTestGateway(t *testing.T, routes map[string]GatewayRoute, reg *Registry) string {
	t.Helper()
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)

	addr := freeAddr(t)
	gw := NewGateway(GatewayConfig{
		Name:           "TEST",
		ListenAddr:     addr,
		InspectTimeout: time.Second,
		DefaultAction:  "reject",
		Routes:         routes,
	}, reg, logger)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go gw.Start(ctx)
	return addr
}

// TestGateway_DispatchToServer verifies that a ClientHello is routed to the
// correct server runtime, with the right target/reportPort, and that the
// inspected ClientHello bytes are replayed intact (PrefixedConn).
func TestGateway_DispatchToServer(t *testing.T) {
	tr1 := &mockTarget{ready: true, enqueueOK: true}
	us1 := &mockTarget{ready: true, enqueueOK: true}

	reg := NewRegistry()
	reg.Register("TR1", tr1)
	reg.Register("US1", us1)

	addr := newTestGateway(t, map[string]GatewayRoute{
		"tr.example.com": {Server: "TR1", Target: "443"},
		"us.example.com": {Server: "US1", Target: "8443"},
	}, reg)

	// Connection for TR1.
	client := dialWithRetry(t, addr)
	defer client.Close()
	hello := buildClientHello("tr.example.com")
	if _, err := client.Write(hello); err != nil {
		t.Fatalf("failed to write ClientHello: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		calls, conn, target, report := tr1.snapshot()
		if calls > 0 {
			if target != "443" {
				t.Fatalf("expected target 443, got %q", target)
			}
			if report != 443 {
				t.Fatalf("expected reportPort 443, got %d", report)
			}
			got := make([]byte, len(hello))
			conn.SetReadDeadline(time.Now().Add(time.Second))
			if _, err := readFull(conn, got); err != nil {
				t.Fatalf("failed to read replayed ClientHello: %v", err)
			}
			if !bytes.Equal(got, hello) {
				t.Fatal("replayed ClientHello mismatch")
			}
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("TR1 was not dispatched a connection")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if calls, _, _, _ := us1.snapshot(); calls != 0 {
		t.Fatalf("US1 should not have been called, got %d", calls)
	}
}

// TestGateway_UnknownSNIRejected verifies unknown SNIs are closed and never
// dispatched.
func TestGateway_UnknownSNIRejected(t *testing.T) {
	tr1 := &mockTarget{ready: true, enqueueOK: true}
	reg := NewRegistry()
	reg.Register("TR1", tr1)

	addr := newTestGateway(t, map[string]GatewayRoute{
		"tr.example.com": {Server: "TR1", Target: "443"},
	}, reg)

	client := dialWithRetry(t, addr)
	defer client.Close()
	if _, err := client.Write(buildClientHello("unknown.example")); err != nil {
		t.Fatalf("failed to write ClientHello: %v", err)
	}

	time.Sleep(300 * time.Millisecond)
	if calls, _, _, _ := tr1.snapshot(); calls != 0 {
		t.Fatalf("enqueue should not be called for unknown SNI, got %d", calls)
	}

	client.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	if _, err := client.Read(buf); err == nil {
		t.Fatal("expected connection to be closed for unknown SNI")
	}
}

// TestGateway_MalformedClientHello verifies a malformed handshake is rejected
// without panic and without dispatch.
func TestGateway_MalformedClientHello(t *testing.T) {
	tr1 := &mockTarget{ready: true, enqueueOK: true}
	reg := NewRegistry()
	reg.Register("TR1", tr1)

	addr := newTestGateway(t, map[string]GatewayRoute{
		"tr.example.com": {Server: "TR1", Target: "443"},
	}, reg)

	client := dialWithRetry(t, addr)
	defer client.Close()
	// Not a TLS record at all.
	if _, err := client.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n")); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	time.Sleep(300 * time.Millisecond)
	if calls, _, _, _ := tr1.snapshot(); calls != 0 {
		t.Fatalf("enqueue should not be called for malformed ClientHello, got %d", calls)
	}
}

// TestGateway_EnqueueFailClosesConn verifies that when the target refuses the
// connection (channel full / not ready), the gateway closes it without panic.
func TestGateway_EnqueueFailClosesConn(t *testing.T) {
	// ready but enqueue fails.
	tr1 := &mockTarget{ready: true, enqueueOK: false}
	reg := NewRegistry()
	reg.Register("TR1", tr1)

	addr := newTestGateway(t, map[string]GatewayRoute{
		"tr.example.com": {Server: "TR1", Target: "443"},
	}, reg)

	client := dialWithRetry(t, addr)
	defer client.Close()
	if _, err := client.Write(buildClientHello("tr.example.com")); err != nil {
		t.Fatalf("failed to write ClientHello: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if calls, _, _, _ := tr1.snapshot(); calls > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("enqueue was never attempted")
		}
		time.Sleep(10 * time.Millisecond)
	}

	client.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	if _, err := client.Read(buf); err == nil {
		t.Fatal("expected connection to be closed after enqueue failure")
	}
}

// TestGateway_NotReadyRejected verifies that when the routed server is not
// ready, the connection is closed and enqueue is never attempted.
func TestGateway_NotReadyRejected(t *testing.T) {
	tr1 := &mockTarget{ready: false, enqueueOK: true}
	reg := NewRegistry()
	reg.Register("TR1", tr1)

	addr := newTestGateway(t, map[string]GatewayRoute{
		"tr.example.com": {Server: "TR1", Target: "443"},
	}, reg)

	client := dialWithRetry(t, addr)
	defer client.Close()
	if _, err := client.Write(buildClientHello("tr.example.com")); err != nil {
		t.Fatalf("failed to write ClientHello: %v", err)
	}

	time.Sleep(300 * time.Millisecond)
	if calls, _, _, _ := tr1.snapshot(); calls != 0 {
		t.Fatalf("enqueue should not be called when target not ready, got %d", calls)
	}
}
