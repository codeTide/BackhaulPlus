package config

import (
	"fmt"
	"strings"
	"time"
)

// RetryIntervalConfig describes how long a client waits between failed attempts
// to (re)establish its remote control channel. It is parsed from the
// retry_interval option of a [[client]] block and supports three forms:
//
//	retry_interval = 5         # legacy: a bare TOML number means 5 seconds
//	retry_interval = "5s"      # fixed duration (Go time.ParseDuration units)
//	retry_interval = "5s-60s"  # adaptive backoff range: start at 5s, back off
//	                           # up to 60s after repeated failures
//
// For the fixed forms Min == Max and Adaptive is false. For the range form
// Min is the starting delay, Max is the cap and Adaptive is true.
type RetryIntervalConfig struct {
	// Raw is the original value as written in the config, kept for logging.
	Raw string
	// Min is the first/only retry delay. It is always > 0 once parsed.
	Min time.Duration
	// Max is the maximum retry delay. For fixed intervals Max == Min.
	Max time.Duration
	// Adaptive is true only for the range form ("min-max").
	Adaptive bool
}

// IsZero reports whether the value was never set (no retry_interval in TOML).
// It is used by the defaults pass to decide whether to fill in the default.
func (r RetryIntervalConfig) IsZero() bool {
	return r.Min == 0 && r.Max == 0 && r.Raw == ""
}

// String renders the policy the way it is logged, e.g. "5s" or "5s-60s".
func (r RetryIntervalConfig) String() string {
	if r.Adaptive {
		return fmt.Sprintf("%s-%s", r.Min, r.Max)
	}
	return r.Min.String()
}

// UnmarshalTOML lets retry_interval accept either a bare TOML integer (legacy
// seconds) or a string ("5s", "500ms", "5s-60s"). This keeps old numeric
// configs working while enabling the new fixed/adaptive string forms.
func (r *RetryIntervalConfig) UnmarshalTOML(data interface{}) error {
	switch v := data.(type) {
	case int64:
		return r.fromSeconds(v)
	case int:
		return r.fromSeconds(int64(v))
	case string:
		parsed, err := ParseRetryInterval(v)
		if err != nil {
			return err
		}
		*r = parsed
		return nil
	default:
		return fmt.Errorf("retry_interval must be a positive number of seconds or a duration string like \"5s\" or \"5s-60s\", got %T", data)
	}
}

// fromSeconds applies the legacy numeric form (seconds).
func (r *RetryIntervalConfig) fromSeconds(seconds int64) error {
	if seconds <= 0 {
		return fmt.Errorf("retry_interval must be greater than 0 (got %d)", seconds)
	}
	d := time.Duration(seconds) * time.Second
	*r = RetryIntervalConfig{
		Raw:      fmt.Sprintf("%d", seconds),
		Min:      d,
		Max:      d,
		Adaptive: false,
	}
	return nil
}

// ParseRetryInterval parses the string form of retry_interval. It accepts a
// single Go duration ("5s", "500ms", "2m30s") for a fixed interval or a
// "min-max" range ("5s-60s") for adaptive backoff. Whitespace around the value
// and around the range separator is ignored. The bounds must be positive and,
// for a range, min must be strictly less than max.
func ParseRetryInterval(raw string) (RetryIntervalConfig, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return RetryIntervalConfig{}, fmt.Errorf("retry_interval must not be empty")
	}

	parts := strings.Split(s, "-")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	// No separator: a single fixed duration.
	if len(parts) == 1 {
		d, err := parsePositiveDuration(parts[0])
		if err != nil {
			return RetryIntervalConfig{}, err
		}
		return RetryIntervalConfig{Raw: raw, Min: d, Max: d, Adaptive: false}, nil
	}

	// Exactly two non-empty parts: an adaptive range. Anything else (empty
	// part, leading/trailing "-", "5s--60s", "-5s-60s") is malformed.
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return RetryIntervalConfig{}, fmt.Errorf("invalid retry_interval %q: malformed range, use \"min-max\" like \"5s-60s\"", raw)
	}

	min, err := parsePositiveDuration(parts[0])
	if err != nil {
		return RetryIntervalConfig{}, err
	}
	max, err := parsePositiveDuration(parts[1])
	if err != nil {
		return RetryIntervalConfig{}, err
	}
	if min == max {
		return RetryIntervalConfig{}, fmt.Errorf("invalid retry_interval %q: range bounds are equal; for a fixed interval use a single duration like %q", raw, parts[0])
	}
	if min > max {
		return RetryIntervalConfig{}, fmt.Errorf("invalid retry_interval %q: range start must be smaller than the maximum", raw)
	}

	return RetryIntervalConfig{Raw: raw, Min: min, Max: max, Adaptive: true}, nil
}

// parsePositiveDuration parses a Go duration string and requires it to be
// strictly positive. A bare number without a unit (e.g. "5" or "5-60") is
// rejected because time.ParseDuration requires a unit.
func parsePositiveDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid retry_interval duration %q: %v (use units like \"500ms\", \"5s\", \"1m\")", s, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid retry_interval duration %q: must be greater than 0", s)
	}
	return d, nil
}

// RetryState tracks the current backoff delay for a single retry loop. It is
// not safe for concurrent use; each control-channel dial loop owns its own
// state.
type RetryState struct {
	policy  RetryIntervalConfig
	current time.Duration
}

// NewRetryState returns a RetryState starting at the policy's minimum delay.
func NewRetryState(policy RetryIntervalConfig) *RetryState {
	return &RetryState{policy: policy, current: policy.Min}
}

// NextDelay returns the delay to wait before the next attempt. For a fixed
// policy it always returns Min. For an adaptive policy it returns the current
// delay and then doubles it for next time, capped at Max.
func (r *RetryState) NextDelay() time.Duration {
	delay := r.current
	if r.policy.Adaptive {
		next := r.current * 2
		if next > r.policy.Max || next <= 0 {
			next = r.policy.Max
		}
		r.current = next
	}
	return delay
}

// Reset returns the backoff to its starting (minimum) delay. It should be
// called after a successful connection.
func (r *RetryState) Reset() {
	r.current = r.policy.Min
}
