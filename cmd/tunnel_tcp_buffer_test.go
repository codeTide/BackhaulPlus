package cmd

import (
	"strings"
	"testing"

	"github.com/codeTide/BackhaulPlus/internal/config"
)

func TestParseTunnelTCPBuffer_Valid(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"auto", 0},
		{"AUTO", 0},
		{"Auto", 0},
		{" auto ", 0},
		{"256kb", 262144},
		{"512kb", 524288},
		{"1mb", 1048576},
		{"2mb", 2097152},
		{"524288", 524288},
		{"1048576", 1048576},
		{" 1MB ", 1048576},
	}
	for _, tc := range cases {
		got, err := parseTunnelTCPBuffer(tc.in)
		if err != nil {
			t.Errorf("parseTunnelTCPBuffer(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseTunnelTCPBuffer(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseTunnelTCPBuffer_Invalid(t *testing.T) {
	cases := []string{
		"",
		"0",
		"0kb",
		"0mb",
		"-1",
		"abc",
		"10gb",
		"1.5mb",
		"kb",
		"mb",
		" ",
	}
	for _, in := range cases {
		if _, err := parseTunnelTCPBuffer(in); err == nil {
			t.Errorf("parseTunnelTCPBuffer(%q) expected error, got nil", in)
		}
	}
}

// baseTCPMuxClient is a minimal client config using the tcpmux transport.
const baseTCPMuxClient = `
[[client]]
name = "C1"
remote_addr = "0.0.0.0:3080"
transport = "tcpmux"
token = "tok"
`

// applyAndValidate runs the same pipeline as Run for a TOML body and returns
// the resulting config so tests can inspect parsed runtime values.
func applyAndValidate(t *testing.T, content string) *config.Config {
	t.Helper()
	cfg, err := loadConfig(writeTempConfig(t, content))
	if err != nil {
		t.Fatalf("loadConfig error: %v", err)
	}
	applyDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig error: %v", err)
	}
	return cfg
}

func TestTunnelTCPBuffer_DefaultWhenMissing(t *testing.T) {
	cfg := applyAndValidate(t, baseTCPMuxClient)
	if got := cfg.Clients[0].TunnelTCPBuffer; got != "2mb" {
		t.Fatalf("default TunnelTCPBuffer = %q, want %q", got, "2mb")
	}
	if got := cfg.Clients[0].TunnelTCPBufferBytes; got != 2097152 {
		t.Fatalf("default TunnelTCPBufferBytes = %d, want %d", got, 2097152)
	}
}

func TestTunnelTCPBuffer_ConfiguredValues(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"auto", 0},
		{"512kb", 524288},
		{"1mb", 1048576},
		{"2mb", 2097152},
		{"524288", 524288},
	}
	for _, tc := range cases {
		content := baseTCPMuxClient + "tunnel_tcp_buffer = \"" + tc.raw + "\"\n"
		cfg := applyAndValidate(t, content)
		if got := cfg.Clients[0].TunnelTCPBufferBytes; got != tc.want {
			t.Errorf("tunnel_tcp_buffer=%q -> bytes %d, want %d", tc.raw, got, tc.want)
		}
	}
}

func TestTunnelTCPBuffer_InvalidRejected(t *testing.T) {
	content := baseTCPMuxClient + "tunnel_tcp_buffer = \"10gb\"\n"
	cfg, err := loadConfig(writeTempConfig(t, content))
	if err != nil {
		t.Fatalf("loadConfig error: %v", err)
	}
	applyDefaults(cfg)
	err = validateConfig(cfg)
	if err == nil {
		t.Fatal("expected validateConfig to reject invalid tunnel_tcp_buffer")
	}
	if !strings.Contains(err.Error(), "tunnel_tcp_buffer") {
		t.Fatalf("error %q does not mention tunnel_tcp_buffer", err)
	}
}
