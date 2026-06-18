package transport

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestSleepWithContext(t *testing.T) {
	// The full delay elapses when the context stays alive.
	if !sleepWithContext(context.Background(), 10*time.Millisecond) {
		t.Fatal("sleepWithContext returned false for an uncancelled context")
	}

	// A cancelled context returns false promptly instead of blocking.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if sleepWithContext(ctx, time.Hour) {
		t.Fatal("sleepWithContext returned true for a cancelled context")
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("sleepWithContext did not return promptly on cancel: %s", elapsed)
	}
}

// TestTcpDialer_ContextCancelDuringBackoff verifies that the internal retry
// backoff inside TcpDialer is context-aware: when the context is cancelled
// while it is waiting between attempts, it returns promptly rather than
// blocking for the full (1s+) backoff.
func TestTcpDialer_ContextCancelDuringBackoff(t *testing.T) {
	// Reserve a local address that nothing listens on so connects are refused
	// quickly (not a slow timeout). Closing the listener frees the port.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve a port: %v", err)
	}
	addr := l.Addr().String()
	l.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	// retry=3 means the first failed attempt is followed by a ~1s backoff; with
	// a context-aware sleep the cancel above must short-circuit it.
	_, err = TcpDialer(ctx, addr, 500*time.Millisecond, 0, true, 3, 0, 0)
	if err == nil {
		t.Fatal("TcpDialer unexpectedly succeeded dialing a closed port")
	}
	if elapsed := time.Since(start); elapsed > 800*time.Millisecond {
		t.Fatalf("TcpDialer did not honor context cancel during backoff: %s", elapsed)
	}
}
