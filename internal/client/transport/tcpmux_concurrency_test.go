package transport

import (
	"sync"
	"sync/atomic"
	"testing"
)

// Under a burst, only the slots up to the hard cap may be reserved; active +
// dialing must never exceed max_connection_pool.
func TestTryReserveDialSlot_ConcurrentHardCap(t *testing.T) {
	c := &TcpMuxTransport{config: &TcpMuxConfig{MaxConnPoolSize: 10}}
	atomic.StoreInt32(&c.poolConnections, 9) // one slot left
	atomic.StoreInt32(&c.dialingConnections, 0)

	const goroutines = 100
	var granted int32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if c.tryReserveDialSlot() {
				atomic.AddInt32(&granted, 1)
			}
		}()
	}
	wg.Wait()

	if granted != 1 {
		t.Fatalf("granted = %d, want exactly 1 (single free slot)", granted)
	}
	total := atomic.LoadInt32(&c.poolConnections) + atomic.LoadInt32(&c.dialingConnections)
	if total > 10 {
		t.Fatalf("active+dialing = %d, exceeded cap of 10", total)
	}
}

// From an empty pool with a generous cap, exactly cap slots are granted.
func TestTryReserveDialSlot_ExactlyCapGranted(t *testing.T) {
	c := &TcpMuxTransport{config: &TcpMuxConfig{MaxConnPoolSize: 100}}
	atomic.StoreInt32(&c.poolConnections, 0)
	atomic.StoreInt32(&c.dialingConnections, 0)

	const goroutines = 1000
	var granted int32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if c.tryReserveDialSlot() {
				atomic.AddInt32(&granted, 1)
			}
		}()
	}
	wg.Wait()

	if granted != 100 {
		t.Fatalf("granted = %d, want exactly 100 (the cap)", granted)
	}
	if dialing := atomic.LoadInt32(&c.dialingConnections); dialing != 100 {
		t.Fatalf("dialingConnections = %d, want 100", dialing)
	}
}

// max_connection_pool = 0 keeps the legacy unlimited behaviour: every caller is
// granted and dialingConnections tracks the count.
func TestTryReserveDialSlot_Unlimited(t *testing.T) {
	c := &TcpMuxTransport{config: &TcpMuxConfig{MaxConnPoolSize: 0}}
	atomic.StoreInt32(&c.poolConnections, 5000)
	atomic.StoreInt32(&c.dialingConnections, 0)

	const goroutines = 1000
	var granted int32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if c.tryReserveDialSlot() {
				atomic.AddInt32(&granted, 1)
			}
		}()
	}
	wg.Wait()

	if granted != goroutines {
		t.Fatalf("granted = %d, want %d (unlimited)", granted, goroutines)
	}
	if dialing := atomic.LoadInt32(&c.dialingConnections); dialing != goroutines {
		t.Fatalf("dialingConnections = %d, want %d", dialing, goroutines)
	}
}
