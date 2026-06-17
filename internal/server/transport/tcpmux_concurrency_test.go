package transport

import (
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sirupsen/logrus"
)

func quietLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(io.Discard)
	l.SetLevel(logrus.PanicLevel)
	return l
}

// decrementPositiveInt32 must never drive the counter below zero, even when many
// goroutines race on it.
func TestDecrementPositiveInt32_NeverNegative(t *testing.T) {
	for _, start := range []int32{1, 50, 100} {
		t.Run("", func(t *testing.T) {
			var v int32 = start

			const goroutines = 100
			var wg sync.WaitGroup
			wg.Add(goroutines)
			for i := 0; i < goroutines; i++ {
				go func() {
					defer wg.Done()
					decrementPositiveInt32(&v)
				}()
			}
			wg.Wait()

			got := atomic.LoadInt32(&v)
			if got < 0 {
				t.Fatalf("counter went negative: %d", got)
			}
			// With more goroutines than the starting value, the counter must
			// bottom out at exactly zero; otherwise it drops by one per call.
			want := start - goroutines
			if want < 0 {
				want = 0
			}
			if got != want {
				t.Fatalf("start=%d goroutines=%d: got %d, want %d", start, goroutines, got, want)
			}
		})
	}
}

func newTestMuxServer(t *testing.T, cfg *TcpMuxConfig) *TcpMuxTransport {
	t.Helper()
	return &TcpMuxTransport{
		config:         cfg,
		logger:         quietLogger(),
		reqNewConnChan: make(chan struct{}, 8192),
	}
}

// Concurrent callers must not collectively queue more requests than the deficit
// allows, and must respect max_mux_sessions as a hard cap.
func TestMaybeRequestMuxSessions_ConcurrentBounded(t *testing.T) {
	s := newTestMuxServer(t, &TcpMuxConfig{
		MuxCon:           8,
		MaxMuxSessions:   100,
		MuxSpareSessions: 0,
	})
	atomic.StoreInt32(&s.streamCounter, 15000) // needs 1875 sessions, capped at 100
	atomic.StoreInt32(&s.sessionCounter, 0)
	atomic.StoreInt32(&s.pendingSessionRequests, 0)

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			s.maybeRequestMuxSessions()
		}()
	}
	wg.Wait()

	if pending := atomic.LoadInt32(&s.pendingSessionRequests); pending != 100 {
		t.Fatalf("pendingSessionRequests = %d, want exactly 100 (the cap)", pending)
	}
	if queued := len(s.reqNewConnChan); queued != 100 {
		t.Fatalf("queued requests = %d, want exactly 100 (the cap)", queued)
	}
}

// When sessions already cover part of the cap, only the remaining room is
// requested.
func TestMaybeRequestMuxSessions_RemainingRoom(t *testing.T) {
	s := newTestMuxServer(t, &TcpMuxConfig{
		MuxCon:           8,
		MaxMuxSessions:   100,
		MuxSpareSessions: 0,
	})
	atomic.StoreInt32(&s.streamCounter, 15000)
	atomic.StoreInt32(&s.sessionCounter, 80)
	atomic.StoreInt32(&s.pendingSessionRequests, 0)

	const goroutines = 64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			s.maybeRequestMuxSessions()
		}()
	}
	wg.Wait()

	if pending := atomic.LoadInt32(&s.pendingSessionRequests); pending != 20 {
		t.Fatalf("pendingSessionRequests = %d, want 20 (100 cap - 80 active)", pending)
	}
	if queued := len(s.reqNewConnChan); queued != 20 {
		t.Fatalf("queued requests = %d, want 20", queued)
	}
}

// Already at the cap (active + pending) -> nothing new is queued.
func TestMaybeRequestMuxSessions_AlreadyAtCap(t *testing.T) {
	s := newTestMuxServer(t, &TcpMuxConfig{
		MuxCon:         8,
		MaxMuxSessions: 100,
	})
	atomic.StoreInt32(&s.streamCounter, 15000)
	atomic.StoreInt32(&s.sessionCounter, 60)
	atomic.StoreInt32(&s.pendingSessionRequests, 40)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			s.maybeRequestMuxSessions()
		}()
	}
	wg.Wait()

	if pending := atomic.LoadInt32(&s.pendingSessionRequests); pending != 40 {
		t.Fatalf("pendingSessionRequests = %d, want unchanged 40", pending)
	}
	if queued := len(s.reqNewConnChan); queued != 0 {
		t.Fatalf("queued requests = %d, want 0", queued)
	}
}

// With max_mux_sessions = 0 (unlimited) pending coalescing still prevents
// redundant requests: concurrent callers queue only the real deficit.
func TestMaybeRequestMuxSessions_UnlimitedCoalesces(t *testing.T) {
	s := newTestMuxServer(t, &TcpMuxConfig{
		MuxCon:         8,
		MaxMuxSessions: 0,
	})
	atomic.StoreInt32(&s.streamCounter, 80) // needs exactly 10 sessions
	atomic.StoreInt32(&s.sessionCounter, 0)
	atomic.StoreInt32(&s.pendingSessionRequests, 0)

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			s.maybeRequestMuxSessions()
		}()
	}
	wg.Wait()

	if pending := atomic.LoadInt32(&s.pendingSessionRequests); pending != 10 {
		t.Fatalf("pendingSessionRequests = %d, want 10 (coalesced deficit)", pending)
	}
	if queued := len(s.reqNewConnChan); queued != 10 {
		t.Fatalf("queued requests = %d, want 10", queued)
	}
}
