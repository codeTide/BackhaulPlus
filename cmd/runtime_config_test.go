package cmd

import (
	"strings"
	"testing"
	"time"
)

func TestRuntimeConfigMemoryReleaseInterval(t *testing.T) {
	cases := []struct {
		name    string
		runtime string
		want    time.Duration
		wantErr bool
	}{
		{name: "missing runtime", runtime: "", want: 0},
		{name: "missing option", runtime: "[runtime]\n", want: 0},
		{name: "empty", runtime: "[runtime]\nmemory_release_interval = \"\"\n", want: 0},
		{name: "zero", runtime: "[runtime]\nmemory_release_interval = \"0\"\n", want: 0},
		{name: "zero seconds", runtime: "[runtime]\nmemory_release_interval = \"0s\"\n", want: 0},
		{name: "one second", runtime: "[runtime]\nmemory_release_interval = \"1s\"\n", want: time.Second},
		{name: "ten minutes", runtime: "[runtime]\nmemory_release_interval = \"10m\"\n", want: 10 * time.Minute},
		{name: "one hour", runtime: "[runtime]\nmemory_release_interval = \"1h\"\n", want: time.Hour},
		{name: "negative", runtime: "[runtime]\nmemory_release_interval = \"-1s\"\n", wantErr: true},
		{name: "invalid", runtime: "[runtime]\nmemory_release_interval = \"abc\"\n", wantErr: true},
		{name: "below minimum", runtime: "[runtime]\nmemory_release_interval = \"500ms\"\n", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadConfig(writeTempConfig(t, tc.runtime+baseTCPServer+"ports = [\"8080\"]\n"))
			if err != nil {
				t.Fatalf("loadConfig error: %v", err)
			}
			applyDefaults(cfg)
			err = validateConfig(cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected validation error")
				}
				if !strings.Contains(err.Error(), "memory_release_interval") {
					t.Fatalf("error %q does not mention memory_release_interval", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateConfig error: %v", err)
			}
			if got := cfg.Runtime.MemoryReleaseIntervalDuration; got != tc.want {
				t.Fatalf("MemoryReleaseIntervalDuration = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestRuntimeConfigAutoRestartInterval(t *testing.T) {
	cases := []struct {
		name    string
		runtime string
		want    time.Duration
		wantErr bool
	}{
		{name: "missing runtime", runtime: "", want: 0},
		{name: "missing option", runtime: "[runtime]\n", want: 0},
		{name: "empty", runtime: "[runtime]\nauto_restart_interval = \"\"\n", want: 0},
		{name: "zero", runtime: "[runtime]\nauto_restart_interval = \"0\"\n", want: 0},
		{name: "zero seconds", runtime: "[runtime]\nauto_restart_interval = \"0s\"\n", want: 0},
		{name: "one minute", runtime: "[runtime]\nauto_restart_interval = \"1m\"\n", want: time.Minute},
		{name: "six hours", runtime: "[runtime]\nauto_restart_interval = \"6h\"\n", want: 6 * time.Hour},
		{name: "twenty four hours", runtime: "[runtime]\nauto_restart_interval = \"24h\"\n", want: 24 * time.Hour},
		{name: "below minimum", runtime: "[runtime]\nauto_restart_interval = \"10s\"\n", wantErr: true},
		{name: "negative", runtime: "[runtime]\nauto_restart_interval = \"-1h\"\n", wantErr: true},
		{name: "invalid", runtime: "[runtime]\nauto_restart_interval = \"abc\"\n", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadConfig(writeTempConfig(t, tc.runtime+baseTCPServer+"ports = [\"8080\"]\n"))
			if err != nil {
				t.Fatalf("loadConfig error: %v", err)
			}
			applyDefaults(cfg)
			err = validateConfig(cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected validation error")
				}
				if !strings.Contains(err.Error(), "auto_restart_interval") {
					t.Fatalf("error %q does not mention auto_restart_interval", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateConfig error: %v", err)
			}
			if got := cfg.Runtime.AutoRestartIntervalDuration; got != tc.want {
				t.Fatalf("AutoRestartIntervalDuration = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestRuntimeConfigCombined(t *testing.T) {
	cfg := applyAndValidate(t, "[runtime]\n"+baseTCPServer+"ports = [\"8080\"]\n")
	if cfg.Runtime.MemoryReleaseIntervalDuration != 0 || cfg.Runtime.AutoRestartIntervalDuration != 0 {
		t.Fatalf("runtime durations = %s/%s, want both disabled", cfg.Runtime.MemoryReleaseIntervalDuration, cfg.Runtime.AutoRestartIntervalDuration)
	}

	cfg = applyAndValidate(t, "[runtime]\nmemory_release_interval = \"10m\"\nauto_restart_interval = \"6h\"\n"+baseTCPServer+"ports = [\"8080\"]\n")
	if cfg.Runtime.MemoryReleaseIntervalDuration != 10*time.Minute || cfg.Runtime.AutoRestartIntervalDuration != 6*time.Hour {
		t.Fatalf("runtime durations = %s/%s, want 10m/6h", cfg.Runtime.MemoryReleaseIntervalDuration, cfg.Runtime.AutoRestartIntervalDuration)
	}

	err := loadAndValidate(t, "[runtime]\nmemory_release_interval = \"abc\"\nauto_restart_interval = \"6h\"\n"+baseTCPServer+"ports = [\"8080\"]\n")
	if err == nil || !strings.Contains(err.Error(), "memory_release_interval") {
		t.Fatalf("expected memory_release_interval validation error, got: %v", err)
	}
}
