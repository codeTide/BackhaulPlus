package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// DialRateLimitConfig describes an optional cap on how many new remote Dial /
// connect attempts a single [[client]] may make per second towards its
// remote_addr. It is parsed from the dial_rate_limit option.
//
// The option exists to reduce outbound SYN bursts on providers (e.g. OVH) that
// flag a VPS sending many TCP SYNs to one destination as abuse. It throttles
// BackhaulPlus's own connect attempts; it does not touch kernel packets
// directly and is never applied to local Xray/localhost dials.
//
// Accepted forms (trimmed):
//
//	dial_rate_limit = ""      # disabled (also the default when omitted)
//	dial_rate_limit = "0"     # disabled
//	dial_rate_limit = "0/s"   # disabled
//	dial_rate_limit = "2/s"   # at most 2 remote dials per second
//	dial_rate_limit = "0.5/s" # fractional rates are allowed
type DialRateLimitConfig struct {
	// Raw is the original value as written in the config.
	Raw string
	// Enabled is false when the option is empty or zero.
	Enabled bool
	// PerSecond is the maximum number of remote dials per second. It is only
	// meaningful when Enabled is true and is always > 0 in that case.
	PerSecond float64
}

// ParseDialRateLimit parses the dial_rate_limit option. Empty, "0" and "0/s"
// mean disabled. Otherwise the value must be a positive number followed by the
// "/s" suffix (e.g. "2/s", "0.5/s"). Anything else is rejected.
func ParseDialRateLimit(raw string) (DialRateLimitConfig, error) {
	s := strings.TrimSpace(raw)

	// Disabled forms.
	if s == "" || s == "0" || s == "0/s" {
		return DialRateLimitConfig{Raw: raw, Enabled: false}, nil
	}

	if !strings.HasSuffix(s, "/s") {
		return DialRateLimitConfig{}, fmt.Errorf("invalid dial_rate_limit %q: expected a rate like \"2/s\"", raw)
	}

	numStr := strings.TrimSpace(strings.TrimSuffix(s, "/s"))
	if numStr == "" {
		return DialRateLimitConfig{}, fmt.Errorf("invalid dial_rate_limit %q: missing rate before \"/s\"", raw)
	}

	rate, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return DialRateLimitConfig{}, fmt.Errorf("invalid dial_rate_limit %q: %v", raw, err)
	}
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return DialRateLimitConfig{}, fmt.Errorf("invalid dial_rate_limit %q: rate must be a finite number", raw)
	}
	if rate == 0 {
		// e.g. "0.0/s" - treat as disabled rather than an error.
		return DialRateLimitConfig{Raw: raw, Enabled: false}, nil
	}
	if rate < 0 {
		return DialRateLimitConfig{}, fmt.Errorf("invalid dial_rate_limit %q: rate must be positive", raw)
	}

	return DialRateLimitConfig{Raw: raw, Enabled: true, PerSecond: rate}, nil
}
