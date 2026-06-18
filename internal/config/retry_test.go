package config

import (
	"testing"
	"time"
)

func TestParseRetryInterval_ValidStrings(t *testing.T) {
	cases := []struct {
		raw      string
		min      time.Duration
		max      time.Duration
		adaptive bool
	}{
		{"5s", 5 * time.Second, 5 * time.Second, false},
		{"500ms", 500 * time.Millisecond, 500 * time.Millisecond, false},
		{"1m", time.Minute, time.Minute, false},
		{"2m30s", 2*time.Minute + 30*time.Second, 2*time.Minute + 30*time.Second, false},
		{"5s-60s", 5 * time.Second, 60 * time.Second, true},
		{"1m-10m", time.Minute, 10 * time.Minute, true},
		{"500ms-5s", 500 * time.Millisecond, 5 * time.Second, true},
		{"2s-2m30s", 2 * time.Second, 2*time.Minute + 30*time.Second, true},
		// Spaces around the range separator are allowed.
		{"5s - 60s", 5 * time.Second, 60 * time.Second, true},
	}

	for _, tc := range cases {
		got, err := ParseRetryInterval(tc.raw)
		if err != nil {
			t.Errorf("ParseRetryInterval(%q) unexpected error: %v", tc.raw, err)
			continue
		}
		if got.Min != tc.min {
			t.Errorf("ParseRetryInterval(%q).Min = %s, want %s", tc.raw, got.Min, tc.min)
		}
		if got.Max != tc.max {
			t.Errorf("ParseRetryInterval(%q).Max = %s, want %s", tc.raw, got.Max, tc.max)
		}
		if got.Adaptive != tc.adaptive {
			t.Errorf("ParseRetryInterval(%q).Adaptive = %v, want %v", tc.raw, got.Adaptive, tc.adaptive)
		}
	}
}

func TestParseRetryInterval_Invalid(t *testing.T) {
	invalid := []string{
		"5m-10s",  // start greater than max
		"60s-5s",  // reversed range
		"0s-60s",  // start must be > 0
		"5s-0s",   // max must be > 0
		"-5s-60s", // negative / malformed
		"5s--60s", // malformed range
		"5-60",    // no units
		"5",       // no unit
		"abc",     // not a duration
		"5s-",     // trailing separator
		"-60s",    // leading separator / negative
		"",        // empty
		"5s-5s",   // equal bounds (should suggest fixed form)
	}

	for _, raw := range invalid {
		if _, err := ParseRetryInterval(raw); err == nil {
			t.Errorf("ParseRetryInterval(%q) expected error, got nil", raw)
		}
	}
}

func TestRetryIntervalUnmarshalTOML_LegacyNumber(t *testing.T) {
	// A bare TOML integer decodes via int64 and means seconds.
	var r RetryIntervalConfig
	if err := r.UnmarshalTOML(int64(5)); err != nil {
		t.Fatalf("UnmarshalTOML(int64(5)) error: %v", err)
	}
	if r.Min != 5*time.Second || r.Max != 5*time.Second || r.Adaptive {
		t.Fatalf("legacy number got %+v, want fixed 5s", r)
	}

	// Non-positive numbers are rejected.
	if err := (&RetryIntervalConfig{}).UnmarshalTOML(int64(0)); err == nil {
		t.Errorf("UnmarshalTOML(int64(0)) expected error, got nil")
	}
	if err := (&RetryIntervalConfig{}).UnmarshalTOML(int64(-3)); err == nil {
		t.Errorf("UnmarshalTOML(int64(-3)) expected error, got nil")
	}
}

func TestRetryIntervalUnmarshalTOML_String(t *testing.T) {
	var r RetryIntervalConfig
	if err := r.UnmarshalTOML("5s-60s"); err != nil {
		t.Fatalf("UnmarshalTOML(\"5s-60s\") error: %v", err)
	}
	if !r.Adaptive || r.Min != 5*time.Second || r.Max != 60*time.Second {
		t.Fatalf("got %+v, want adaptive 5s-60s", r)
	}
}

func TestRetryIntervalString_Display(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"5s-60s", "5s-60s"},
		{"5s - 60s", "5s-60s"}, // spaces collapsed
		{"500ms", "500ms"},
		{"2m30s", "2m30s"},
	}
	for _, tc := range cases {
		ri, err := ParseRetryInterval(tc.raw)
		if err != nil {
			t.Fatalf("ParseRetryInterval(%q) error: %v", tc.raw, err)
		}
		if got := ri.String(); got != tc.want {
			t.Errorf("ParseRetryInterval(%q).String() = %q, want %q", tc.raw, got, tc.want)
		}
	}

	// Legacy numeric form logs as a normalized duration, not a bare number.
	var legacy RetryIntervalConfig
	if err := legacy.UnmarshalTOML(int64(5)); err != nil {
		t.Fatalf("UnmarshalTOML(5) error: %v", err)
	}
	if got := legacy.String(); got != "5s" {
		t.Errorf("legacy retry_interval = 5 String() = %q, want %q", got, "5s")
	}
}

func TestRetryState_Fixed(t *testing.T) {
	policy, err := ParseRetryInterval("5s")
	if err != nil {
		t.Fatalf("ParseRetryInterval error: %v", err)
	}
	rs := NewRetryState(policy)
	for i := 0; i < 4; i++ {
		if d := rs.NextDelay(); d != 5*time.Second {
			t.Fatalf("fixed NextDelay() #%d = %s, want 5s", i, d)
		}
	}
	rs.Reset()
	if d := rs.NextDelay(); d != 5*time.Second {
		t.Fatalf("fixed NextDelay() after reset = %s, want 5s", d)
	}
}

func TestRetryState_AdaptiveBackoff(t *testing.T) {
	policy, err := ParseRetryInterval("5s-60s")
	if err != nil {
		t.Fatalf("ParseRetryInterval error: %v", err)
	}
	rs := NewRetryState(policy)

	want := []time.Duration{
		5 * time.Second,
		10 * time.Second,
		20 * time.Second,
		40 * time.Second,
		60 * time.Second,
		60 * time.Second, // capped
	}
	for i, w := range want {
		if got := rs.NextDelay(); got != w {
			t.Fatalf("adaptive NextDelay() #%d = %s, want %s", i, got, w)
		}
	}

	rs.Reset()
	if got := rs.NextDelay(); got != 5*time.Second {
		t.Fatalf("adaptive NextDelay() after reset = %s, want 5s", got)
	}
}
