package transport

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func newTestHTTPGateway(t *testing.T, routes map[string]HTTPGatewayRoute, reg *Registry) string {
	t.Helper()
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)

	addr := freeAddr(t)
	gw := NewHTTPGateway(HTTPGatewayConfig{
		Name:           "TEST",
		ListenAddr:     addr,
		InspectTimeout: time.Second,
		MaxHeaderBytes: 32768,
		DefaultAction:  "reject",
		Routes:         routes,
	}, reg, logger)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := gw.Start(ctx); err != nil {
		t.Fatalf("gateway failed to start: %v", err)
	}
	return addr
}

func TestHTTPGateway_BindFailureReturnsError(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to occupy address: %v", err)
	}
	defer occupied.Close()

	reg := NewRegistry()
	reg.Register("TR1", &mockTarget{ready: true, enqueueOK: true})

	gw := NewHTTPGateway(HTTPGatewayConfig{
		Name:           "PUBLIC",
		ListenAddr:     occupied.Addr().String(),
		InspectTimeout: time.Second,
		MaxHeaderBytes: 32768,
		DefaultAction:  "reject",
		Routes:         map[string]HTTPGatewayRoute{"a.com": {Server: "TR1", Target: "443"}},
	}, reg, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := gw.Start(ctx); err == nil {
		t.Fatal("expected Start to return an error when the address is already in use")
	}
}

func TestHTTPGateway_DispatchToServer(t *testing.T) {
	tr1 := &mockTarget{ready: true, enqueueOK: true}
	us1 := &mockTarget{ready: true, enqueueOK: true}
	reg := NewRegistry()
	reg.Register("TR1", tr1)
	reg.Register("US1", us1)

	addr := newTestHTTPGateway(t, map[string]HTTPGatewayRoute{
		"tr.example.com": {Server: "TR1", Target: "443"},
		"us.example.com": {Server: "US1", Target: "1.1.1.1:5201"},
	}, reg)

	client := dialWithRetry(t, addr)
	defer client.Close()
	req := httpRequest("POST /xhttp HTTP/1.1", "Host: tr.example.com")
	if _, err := client.Write(req); err != nil {
		t.Fatalf("failed to write request: %v", err)
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
			got := make([]byte, len(req))
			conn.SetReadDeadline(time.Now().Add(time.Second))
			if _, err := readFull(conn, got); err != nil {
				t.Fatalf("failed to read replayed request: %v", err)
			}
			if !bytes.Equal(got, req) {
				t.Fatal("replayed request bytes mismatch")
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

func TestHTTPGateway_ReportPortFromNonNumericTarget(t *testing.T) {
	us1 := &mockTarget{ready: true, enqueueOK: true}
	reg := NewRegistry()
	reg.Register("US1", us1)

	addr := newTestHTTPGateway(t, map[string]HTTPGatewayRoute{
		"us.example.com": {Server: "US1", Target: "1.1.1.1:5201"},
	}, reg)

	client := dialWithRetry(t, addr)
	defer client.Close()
	if _, err := client.Write(httpRequest("GET / HTTP/1.1", "Host: us.example.com")); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		calls, conn, target, report := us1.snapshot()
		if calls > 0 {
			if target != "1.1.1.1:5201" {
				t.Fatalf("expected target 1.1.1.1:5201, got %q", target)
			}
			if report != 5201 {
				t.Fatalf("expected reportPort 5201, got %d", report)
			}
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("US1 was not dispatched a connection")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestHTTPGateway_UnknownHostRejected(t *testing.T) {
	tr1 := &mockTarget{ready: true, enqueueOK: true}
	reg := NewRegistry()
	reg.Register("TR1", tr1)

	addr := newTestHTTPGateway(t, map[string]HTTPGatewayRoute{
		"tr.example.com": {Server: "TR1", Target: "443"},
	}, reg)

	client := dialWithRetry(t, addr)
	defer client.Close()
	if _, err := client.Write(httpRequest("GET / HTTP/1.1", "Host: unknown.example")); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	time.Sleep(300 * time.Millisecond)
	if calls, _, _, _ := tr1.snapshot(); calls != 0 {
		t.Fatalf("enqueue should not be called for unknown Host, got %d", calls)
	}

	client.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	if _, err := client.Read(buf); err == nil {
		t.Fatal("expected connection to be closed for unknown Host")
	}
}

func TestHTTPGateway_MalformedRequestRejected(t *testing.T) {
	tr1 := &mockTarget{ready: true, enqueueOK: true}
	reg := NewRegistry()
	reg.Register("TR1", tr1)

	addr := newTestHTTPGateway(t, map[string]HTTPGatewayRoute{
		"tr.example.com": {Server: "TR1", Target: "443"},
	}, reg)

	client := dialWithRetry(t, addr)
	defer client.Close()
	// TLS ClientHello bytes are not valid cleartext HTTP.
	if _, err := client.Write([]byte("\x16\x03\x01\x00\x10not-http\r\n\r\n")); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	time.Sleep(300 * time.Millisecond)
	if calls, _, _, _ := tr1.snapshot(); calls != 0 {
		t.Fatalf("enqueue should not be called for malformed request, got %d", calls)
	}
}

func TestHTTPGateway_NotReadyRejected(t *testing.T) {
	tr1 := &mockTarget{ready: false, enqueueOK: true}
	reg := NewRegistry()
	reg.Register("TR1", tr1)

	addr := newTestHTTPGateway(t, map[string]HTTPGatewayRoute{
		"tr.example.com": {Server: "TR1", Target: "443"},
	}, reg)

	client := dialWithRetry(t, addr)
	defer client.Close()
	if _, err := client.Write(httpRequest("GET / HTTP/1.1", "Host: tr.example.com")); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	time.Sleep(300 * time.Millisecond)
	if calls, _, _, _ := tr1.snapshot(); calls != 0 {
		t.Fatalf("enqueue should not be called when target not ready, got %d", calls)
	}
}

func TestHTTPGateway_EnqueueFailClosesConn(t *testing.T) {
	tr1 := &mockTarget{ready: true, enqueueOK: false}
	reg := NewRegistry()
	reg.Register("TR1", tr1)

	addr := newTestHTTPGateway(t, map[string]HTTPGatewayRoute{
		"tr.example.com": {Server: "TR1", Target: "443"},
	}, reg)

	client := dialWithRetry(t, addr)
	defer client.Close()
	if _, err := client.Write(httpRequest("GET / HTTP/1.1", "Host: tr.example.com")); err != nil {
		t.Fatalf("failed to write request: %v", err)
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
