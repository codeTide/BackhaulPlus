package utils

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestDialRateLimiter_Disabled(t *testing.T) {
	// A non-positive rate yields a nil limiter (disabled).
	if l := NewDialRateLimiter(0, nil); l != nil {
		t.Fatalf("NewDialRateLimiter(0) = %v, want nil", l)
	}

	// Calling Wait on a nil limiter returns immediately.
	var l *DialRateLimiter
	if err := l.Wait(context.Background()); err != nil {
		t.Fatalf("nil limiter Wait returned error: %v", err)
	}

	// A nil limiter still honours a cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.Wait(ctx); err == nil {
		t.Fatalf("nil limiter Wait with cancelled ctx returned nil, want error")
	}
}

func TestDialRateLimiter_PacesSharedAcrossGoroutines(t *testing.T) {
	// 50 dials/s => 20ms spacing. A single shared limiter must pace all
	// concurrent goroutines through one gate, so N requests take at least
	// (N-1) intervals.
	const perSecond = 50.0
	interval := time.Duration(float64(time.Second) / perSecond)

	l := NewDialRateLimiter(perSecond, nil)

	const n = 5
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := l.Wait(context.Background()); err != nil {
				t.Errorf("Wait error: %v", err)
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	// Allow some slack but ensure pacing actually happened: at least 3
	// intervals for 5 requests (conservative lower bound to avoid flakiness).
	if min := 3 * interval; elapsed < min {
		t.Fatalf("shared limiter did not pace: elapsed %s < %s", elapsed, min)
	}
}

func TestDialRateLimiter_ContextCancel(t *testing.T) {
	// 1 dial/s => 1s spacing. The first Wait passes immediately; the second
	// must block and then return when the context is cancelled.
	l := NewDialRateLimiter(1, nil)

	if err := l.Wait(context.Background()); err != nil {
		t.Fatalf("first Wait error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := l.Wait(ctx)
	if err == nil {
		t.Fatalf("second Wait returned nil, want context error")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("Wait did not return promptly on cancel: elapsed %s", elapsed)
	}
}
