package transport

import (
	"sync/atomic"
	"testing"
)

func TestCanDialMore(t *testing.T) {
	cases := []struct {
		name                 string
		active, dialing, max int
		want                 bool
	}{
		{"unlimited zero", 100, 100, 0, true},
		{"unlimited negative", 100, 100, -1, true},
		{"below cap", 50, 10, 100, true},
		{"at cap via active", 100, 0, 100, false},
		{"at cap via dialing", 0, 100, 100, false},
		{"at cap split", 60, 40, 100, false},
		{"one below cap", 60, 39, 100, true},
		{"over cap", 80, 40, 100, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := canDialMore(c.active, c.dialing, c.max); got != c.want {
				t.Errorf("canDialMore(%d, %d, %d) = %v, want %v", c.active, c.dialing, c.max, got, c.want)
			}
		})
	}
}

func TestCanStartTunnelDialer_HardCap(t *testing.T) {
	c := &TcpMuxTransport{config: &TcpMuxConfig{MaxConnPoolSize: 100}}

	// 100 active + 0 dialing -> at cap, cannot start.
	atomic.StoreInt32(&c.poolConnections, 100)
	atomic.StoreInt32(&c.dialingConnections, 0)
	if c.canStartTunnelDialer() {
		t.Fatal("expected canStartTunnelDialer to be false at cap (active)")
	}

	// Split between active and dialing still counts against the cap.
	atomic.StoreInt32(&c.poolConnections, 60)
	atomic.StoreInt32(&c.dialingConnections, 40)
	if c.canStartTunnelDialer() {
		t.Fatal("expected canStartTunnelDialer to be false at cap (active+dialing)")
	}

	// One slot free.
	atomic.StoreInt32(&c.poolConnections, 60)
	atomic.StoreInt32(&c.dialingConnections, 39)
	if !c.canStartTunnelDialer() {
		t.Fatal("expected canStartTunnelDialer to be true with a free slot")
	}
}

func TestCanStartTunnelDialer_Unlimited(t *testing.T) {
	c := &TcpMuxTransport{config: &TcpMuxConfig{MaxConnPoolSize: 0}}
	atomic.StoreInt32(&c.poolConnections, 10000)
	atomic.StoreInt32(&c.dialingConnections, 10000)
	if !c.canStartTunnelDialer() {
		t.Fatal("expected unlimited pool to always allow new dialers")
	}
}
