package utils

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// DialRateLimiter paces new remote dial attempts so a client does not exceed a
// configured number of connects per second. A single limiter is shared by all
// dial goroutines of one client, so concurrent control-channel, pool and load
// dials all pass through the same gate.
//
// It is implemented as a simple, concurrency-safe, context-aware spacer: each
// Wait reserves the next allowed slot, spaced "interval" apart. A nil limiter
// means "disabled" and Wait returns immediately (only honouring context
// cancellation), so callers can hold a possibly-nil *DialRateLimiter and call
// Wait unconditionally.
type DialRateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
	logger   *logrus.Logger
}

// NewDialRateLimiter returns a limiter that allows at most perSecond dials per
// second. A non-positive rate returns nil (disabled).
func NewDialRateLimiter(perSecond float64, logger *logrus.Logger) *DialRateLimiter {
	if perSecond <= 0 {
		return nil
	}
	return &DialRateLimiter{
		interval: time.Duration(float64(time.Second) / perSecond),
		logger:   logger,
	}
}

// Wait blocks until the next dial slot is available or ctx is cancelled. It
// returns ctx.Err() if the context is cancelled while waiting. A nil receiver
// (disabled limiter) returns immediately unless ctx is already cancelled.
func (l *DialRateLimiter) Wait(ctx context.Context) error {
	if l == nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	l.mu.Lock()
	now := time.Now()
	if l.next.Before(now) {
		l.next = now
	}
	wait := l.next.Sub(now)
	l.next = l.next.Add(l.interval)
	l.mu.Unlock()

	if wait <= 0 {
		return nil
	}

	if l.logger != nil {
		l.logger.Debugf("remote dial delayed by rate limit; wait=%s", wait.Round(time.Millisecond))
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
