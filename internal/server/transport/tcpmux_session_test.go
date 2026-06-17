package transport

import "testing"

func TestCeilDiv(t *testing.T) {
	cases := []struct {
		a, b, want int
	}{
		{0, 8, 0},
		{1, 8, 1},
		{8, 8, 1},
		{9, 8, 2},
		{15000, 8, 1875},
		{15000, 32, 469},
		{5, 0, 0},  // non-positive divisor
		{-3, 8, 0}, // non-positive numerator
	}
	for _, c := range cases {
		if got := ceilDiv(c.a, c.b); got != c.want {
			t.Errorf("ceilDiv(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSessionRequestDeficit_NeededSessions(t *testing.T) {
	// With no existing/pending sessions and no spare, the deficit equals the
	// number of sessions needed to cover the active streams.
	cases := []struct {
		name                  string
		activeStreams, muxCon int
		spare, want           int
	}{
		{"zero streams no spare", 0, 8, 0, 0},
		{"one stream", 1, 8, 0, 1},
		{"exactly one session", 8, 8, 0, 1},
		{"just over one session", 9, 8, 0, 2},
		{"large mux8", 15000, 8, 0, 1875},
		{"large mux32", 15000, 32, 0, 469},
		{"zero streams with spare", 0, 8, 4, 4},
		{"streams plus spare", 9, 8, 2, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sessionRequestDeficit(c.activeStreams, 0, 0, c.muxCon, c.spare, 0)
			if got != c.want {
				t.Errorf("sessionRequestDeficit = %d, want %d", got, c.want)
			}
		})
	}
}

func TestSessionRequestDeficit_PendingCoalescing(t *testing.T) {
	// 9 streams over mux_con 8 needs 2 sessions. If 1 is active and 1 is
	// pending, no further request should be issued.
	if got := sessionRequestDeficit(9, 1, 1, 8, 0, 0); got != 0 {
		t.Fatalf("expected no deficit when active+pending covers need, got %d", got)
	}
	// One active, none pending -> still need one more.
	if got := sessionRequestDeficit(9, 1, 0, 8, 0, 0); got != 1 {
		t.Fatalf("expected deficit of 1, got %d", got)
	}
	// More active+pending than needed -> never negative.
	if got := sessionRequestDeficit(8, 5, 5, 8, 0, 0); got != 0 {
		t.Fatalf("expected non-negative deficit, got %d", got)
	}
}

func TestSessionRequestDeficit_MaxMuxSessions(t *testing.T) {
	// Needed would be 1875 sessions, but the hard cap of 512 leaves room for
	// only 512 (none active/pending yet).
	if got := sessionRequestDeficit(15000, 0, 0, 8, 0, 512); got != 512 {
		t.Fatalf("expected deficit capped at 512, got %d", got)
	}
	// Already at the cap -> no room, no request.
	if got := sessionRequestDeficit(15000, 300, 212, 8, 0, 512); got != 0 {
		t.Fatalf("expected no room at cap, got %d", got)
	}
	// Cap above need is a no-op constraint.
	if got := sessionRequestDeficit(9, 0, 0, 8, 0, 1024); got != 2 {
		t.Fatalf("expected deficit of 2 with generous cap, got %d", got)
	}
}

func TestSessionRequestDeficit_MuxConGuard(t *testing.T) {
	if got := sessionRequestDeficit(100, 0, 0, 0, 0, 0); got != 0 {
		t.Fatalf("expected 0 deficit when muxCon <= 0, got %d", got)
	}
}
