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

// At the cap, admission is refused and neither counter moves (no pending here).
func TestAdmitMuxSession_RejectsAtCap(t *testing.T) {
	s := newTestMuxServer(t, &TcpMuxConfig{MaxMuxSessions: 2})
	atomic.StoreInt32(&s.sessionCounter, 2)
	atomic.StoreInt32(&s.pendingSessionRequests, 0)

	if s.admitMuxSession() {
		t.Fatal("expected admission to be rejected at cap")
	}
	if got := atomic.LoadInt32(&s.sessionCounter); got != 2 {
		t.Fatalf("sessionCounter = %d, want unchanged 2", got)
	}
	if got := atomic.LoadInt32(&s.pendingSessionRequests); got != 0 {
		t.Fatalf("pendingSessionRequests = %d, want 0", got)
	}
}

// A rejected session still answered a pending request, so pending is released
// even though the session is not admitted.
func TestAdmitMuxSession_DecrementsPendingEvenWhenRejected(t *testing.T) {
	s := newTestMuxServer(t, &TcpMuxConfig{MaxMuxSessions: 2})
	atomic.StoreInt32(&s.sessionCounter, 2)
	atomic.StoreInt32(&s.pendingSessionRequests, 1)

	if s.admitMuxSession() {
		t.Fatal("expected admission to be rejected at cap")
	}
	if got := atomic.LoadInt32(&s.sessionCounter); got != 2 {
		t.Fatalf("sessionCounter = %d, want unchanged 2", got)
	}
	if got := atomic.LoadInt32(&s.pendingSessionRequests); got != 0 {
		t.Fatalf("pendingSessionRequests = %d, want 0 (request was answered)", got)
	}
}

// Below the cap, the session is admitted and pending is released.
func TestAdmitMuxSession_AllowsBelowCap(t *testing.T) {
	s := newTestMuxServer(t, &TcpMuxConfig{MaxMuxSessions: 2})
	atomic.StoreInt32(&s.sessionCounter, 1)
	atomic.StoreInt32(&s.pendingSessionRequests, 1)

	if !s.admitMuxSession() {
		t.Fatal("expected admission below cap")
	}
	if got := atomic.LoadInt32(&s.sessionCounter); got != 2 {
		t.Fatalf("sessionCounter = %d, want 2", got)
	}
	if got := atomic.LoadInt32(&s.pendingSessionRequests); got != 0 {
		t.Fatalf("pendingSessionRequests = %d, want 0", got)
	}
}

// max_mux_sessions = 0 means unlimited: always admit, regardless of count.
func TestAdmitMuxSession_Unlimited(t *testing.T) {
	s := newTestMuxServer(t, &TcpMuxConfig{MaxMuxSessions: 0})
	atomic.StoreInt32(&s.sessionCounter, 10000)
	atomic.StoreInt32(&s.pendingSessionRequests, 1)

	if !s.admitMuxSession() {
		t.Fatal("expected unlimited admission")
	}
	if got := atomic.LoadInt32(&s.sessionCounter); got != 10001 {
		t.Fatalf("sessionCounter = %d, want 10001", got)
	}
	if got := atomic.LoadInt32(&s.pendingSessionRequests); got != 0 {
		t.Fatalf("pendingSessionRequests = %d, want 0", got)
	}
}

// Under a burst of arrivals, admissions are capped exactly at MaxMuxSessions and
// the counter never exceeds it.
func TestAdmitMuxSession_ConcurrentHardCap(t *testing.T) {
	s := newTestMuxServer(t, &TcpMuxConfig{MaxMuxSessions: 100})
	atomic.StoreInt32(&s.sessionCounter, 0)
	atomic.StoreInt32(&s.pendingSessionRequests, 0)

	const goroutines = 1000
	var admitted int32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if s.admitMuxSession() {
				atomic.AddInt32(&admitted, 1)
			}
		}()
	}
	wg.Wait()

	if admitted != 100 {
		t.Fatalf("admitted = %d, want exactly 100 (the cap)", admitted)
	}
	if got := atomic.LoadInt32(&s.sessionCounter); got != 100 {
		t.Fatalf("sessionCounter = %d, want 100", got)
	}
}

// releaseMuxSession must never drive sessionCounter negative, even when more
// releases race than there are sessions. reqNewConnChan is sized so the
// replacement requests it may queue never block.
func TestReleaseMuxSession_NeverNegative(t *testing.T) {
	s := newTestMuxServer(t, &TcpMuxConfig{MuxCon: 8, MaxMuxSessions: 0})
	atomic.StoreInt32(&s.sessionCounter, 1)

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			s.releaseMuxSession()
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&s.sessionCounter); got < 0 {
		t.Fatalf("sessionCounter went negative: %d", got)
	}
	if got := atomic.LoadInt32(&s.sessionCounter); got != 0 {
		t.Fatalf("sessionCounter = %d, want 0", got)
	}
}

// After a session is released while streams are still waiting, the next request
// evaluation queues a replacement.
func TestMaybeRequestMuxSessions_AfterSessionRelease(t *testing.T) {
	s := newTestMuxServer(t, &TcpMuxConfig{MuxCon: 8, MaxMuxSessions: 0})
	// 16 streams need 2 sessions; one session is active and about to be released.
	atomic.StoreInt32(&s.streamCounter, 16)
	atomic.StoreInt32(&s.sessionCounter, 1)
	atomic.StoreInt32(&s.pendingSessionRequests, 0)

	s.releaseMuxSession() // drops to 0 active, then re-evaluates

	// 16 streams / mux_con 8 = 2 needed, 0 active, 0 pending -> queue 2.
	if got := atomic.LoadInt32(&s.pendingSessionRequests); got != 2 {
		t.Fatalf("pendingSessionRequests = %d, want 2 after release", got)
	}
	if queued := len(s.reqNewConnChan); queued != 2 {
		t.Fatalf("queued requests = %d, want 2", queued)
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
