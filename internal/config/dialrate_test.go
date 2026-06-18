package config

import "testing"

func TestParseDialRateLimit_Disabled(t *testing.T) {
	for _, raw := range []string{"", "0", "0/s", "  ", "0.0/s"} {
		got, err := ParseDialRateLimit(raw)
		if err != nil {
			t.Errorf("ParseDialRateLimit(%q) unexpected error: %v", raw, err)
			continue
		}
		if got.Enabled {
			t.Errorf("ParseDialRateLimit(%q).Enabled = true, want false", raw)
		}
	}
}

func TestParseDialRateLimit_Valid(t *testing.T) {
	cases := []struct {
		raw  string
		rate float64
	}{
		{"2/s", 2},
		{"5/s", 5},
		{"10/s", 10},
		{"0.5/s", 0.5},
	}
	for _, tc := range cases {
		got, err := ParseDialRateLimit(tc.raw)
		if err != nil {
			t.Errorf("ParseDialRateLimit(%q) unexpected error: %v", tc.raw, err)
			continue
		}
		if !got.Enabled {
			t.Errorf("ParseDialRateLimit(%q).Enabled = false, want true", tc.raw)
		}
		if got.PerSecond != tc.rate {
			t.Errorf("ParseDialRateLimit(%q).PerSecond = %v, want %v", tc.raw, got.PerSecond, tc.rate)
		}
	}
}

func TestParseDialRateLimit_Invalid(t *testing.T) {
	for _, raw := range []string{"2", "2/sec", "2/m", "-1/s", "abc", "/s", "s"} {
		if _, err := ParseDialRateLimit(raw); err == nil {
			t.Errorf("ParseDialRateLimit(%q) expected error, got nil", raw)
		}
	}
}
