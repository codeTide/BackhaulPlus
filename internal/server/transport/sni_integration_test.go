package transport

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// TestTCPTransport_SNIInboundReachesLocalChannel verifies that the SNI router,
// when wired to the TCP transport's enqueue, delivers a routed connection into
// localChannel with the correct virtual target and report port, and that the
// inspected ClientHello bytes are replayed intact.
func TestTCPTransport_SNIInboundReachesLocalChannel(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := freeAddr(t)
	cfg := &TcpConfig{
		ChannelSize:       16,
		SNIRouter:         true,
		SNIListenAddr:     addr,
		SNIInspectTimeout: time.Second,
		SNIDefaultAction:  "reject",
		SNIRoutes:         map[string]string{"myket.ir": "10001"},
	}
	s := NewTCPServer(ctx, cfg, logger)

	go s.startSNIRouter()

	client := dialWithRetry(t, addr)
	defer client.Close()

	hello := buildClientHello("myket.ir")
	if _, err := client.Write(hello); err != nil {
		t.Fatalf("failed to write ClientHello: %v", err)
	}

	select {
	case lc := <-s.localChannel:
		if lc.remoteAddr != "10001" {
			t.Fatalf("expected target 10001, got %q", lc.remoteAddr)
		}
		if lc.usagePort() != 10001 {
			t.Fatalf("expected usagePort 10001, got %d", lc.usagePort())
		}
		got := make([]byte, len(hello))
		lc.conn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := readFull(lc.conn, got); err != nil {
			t.Fatalf("failed to read replayed ClientHello: %v", err)
		}
		if !bytes.Equal(got, hello) {
			t.Fatalf("replayed ClientHello mismatch")
		}
		lc.conn.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("SNI-routed connection did not reach localChannel")
	}
}

// TestTCPMuxTransport_SNIInboundReachesLocalChannel is the tcpmux equivalent of
// the TCP smoke test above.
func TestTCPMuxTransport_SNIInboundReachesLocalChannel(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := freeAddr(t)
	cfg := &TcpMuxConfig{
		ChannelSize:       16,
		MuxCon:            8,
		SNIRouter:         true,
		SNIListenAddr:     addr,
		SNIInspectTimeout: time.Second,
		SNIDefaultAction:  "reject",
		SNIRoutes:         map[string]string{"cafebazaar.ir": "10002"},
	}
	s := NewTcpMuxServer(ctx, cfg, logger)

	go s.startSNIRouter()

	client := dialWithRetry(t, addr)
	defer client.Close()

	hello := buildClientHello("cafebazaar.ir")
	if _, err := client.Write(hello); err != nil {
		t.Fatalf("failed to write ClientHello: %v", err)
	}

	select {
	case lc := <-s.localChannel:
		if lc.remoteAddr != "10002" {
			t.Fatalf("expected target 10002, got %q", lc.remoteAddr)
		}
		if lc.usagePort() != 10002 {
			t.Fatalf("expected usagePort 10002, got %d", lc.usagePort())
		}
		lc.conn.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("SNI-routed connection did not reach localChannel")
	}
}
