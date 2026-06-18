package cmd

import (
	"testing"
	"time"

	"github.com/codeTide/BackhaulPlus/internal/config"
)

// loadConfigPipeline runs the real load -> defaults -> validate pipeline and
// returns the resulting config so individual parsed fields can be asserted.
func loadConfigPipeline(t *testing.T, content string) (*config.Config, error) {
	t.Helper()
	cfg, err := loadConfig(writeTempConfig(t, content))
	if err != nil {
		return nil, err
	}
	applyDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// clientBlock builds a minimal valid [[client]] config with the given extra
// lines appended, so individual options can be exercised through the real
// load -> defaults -> validate pipeline.
func clientBlock(extra string) string {
	return `
[[client]]
name = "IR1"
remote_addr = "127.0.0.1:20017"
transport = "tcpmux"
token = "CHANGE_ME"
` + extra + "\n"
}

func TestRetryInterval_LegacyNumber(t *testing.T) {
	cfg, err := loadConfigPipeline(t, clientBlock(`retry_interval = 5`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ri := cfg.Clients[0].RetryInterval
	if ri.Min != 5*time.Second || ri.Max != 5*time.Second || ri.Adaptive {
		t.Fatalf("retry_interval = 5 got %+v, want fixed 5s", ri)
	}
}

func TestRetryInterval_DefaultWhenAbsent(t *testing.T) {
	cfg, err := loadConfigPipeline(t, clientBlock(``))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ri := cfg.Clients[0].RetryInterval
	if ri.Min != 3*time.Second || ri.Adaptive {
		t.Fatalf("absent retry_interval got %+v, want default fixed 3s", ri)
	}
}

func TestRetryInterval_FixedString(t *testing.T) {
	cfg, err := loadConfigPipeline(t, clientBlock(`retry_interval = "5s"`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ri := cfg.Clients[0].RetryInterval
	if ri.Min != 5*time.Second || ri.Max != 5*time.Second || ri.Adaptive {
		t.Fatalf("retry_interval = \"5s\" got %+v, want fixed 5s", ri)
	}
}

func TestRetryInterval_AdaptiveRange(t *testing.T) {
	cfg, err := loadConfigPipeline(t, clientBlock(`retry_interval = "5s-60s"`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ri := cfg.Clients[0].RetryInterval
	if !ri.Adaptive || ri.Min != 5*time.Second || ri.Max != 60*time.Second {
		t.Fatalf("retry_interval = \"5s-60s\" got %+v, want adaptive 5s-60s", ri)
	}
}

func TestRetryInterval_InvalidRejected(t *testing.T) {
	for _, v := range []string{`"5m-10s"`, `"60s-5s"`, `"5"`, `"5-60"`, `"abc"`, `"5s-"`} {
		if _, err := loadConfigPipeline(t, clientBlock(`retry_interval = `+v)); err == nil {
			t.Errorf("retry_interval = %s expected error, got nil", v)
		}
	}
}

func TestDialRateLimit_Valid(t *testing.T) {
	cfg, err := loadConfigPipeline(t, clientBlock(`dial_rate_limit = "2/s"`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	drl := cfg.Clients[0].DialRateLimitConfig
	if !drl.Enabled || drl.PerSecond != 2 {
		t.Fatalf("dial_rate_limit = \"2/s\" got %+v, want enabled 2/s", drl)
	}
}

func TestDialRateLimit_DisabledWhenAbsent(t *testing.T) {
	cfg, err := loadConfigPipeline(t, clientBlock(``))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Clients[0].DialRateLimitConfig.Enabled {
		t.Fatalf("absent dial_rate_limit should be disabled")
	}
}

func TestDialRateLimit_InvalidRejected(t *testing.T) {
	for _, v := range []string{`"2"`, `"2/sec"`, `"2/m"`, `"-1/s"`, `"abc"`} {
		if _, err := loadConfigPipeline(t, clientBlock(`dial_rate_limit = `+v)); err == nil {
			t.Errorf("dial_rate_limit = %s expected error, got nil", v)
		}
	}
}
