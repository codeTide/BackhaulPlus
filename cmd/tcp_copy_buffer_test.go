package cmd

import (
	"strings"
	"testing"
)

func TestParseTCPCopyBuffer_Valid(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"1kb", 1024},
		{"2kb", 2048},
		{"4kb", 4096},
		{"8kb", 8192},
		{"16kb", 16384},
		{"32kb", 32768},
		{"64kb", 65536},
		{"1mb", 1048576},
		{"4096", 4096},
		{"16384", 16384},
		{" 4KB ", 4096},
	}
	for _, tc := range cases {
		got, err := parseTCPCopyBuffer(tc.in)
		if err != nil {
			t.Errorf("parseTCPCopyBuffer(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseTCPCopyBuffer(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseTCPCopyBuffer_Invalid(t *testing.T) {
	cases := []string{
		"",
		"0",
		"0kb",
		"-1",
		"auto",
		"1.5kb",
		"1.5mb",
		"512",  // below the 1KB minimum
		"512b", // unsupported "b" unit
		"10gb", // gb is not supported
		"abc",
		"kb",
		"mb",
		"2mb", // above the 1MB maximum
	}
	for _, in := range cases {
		if _, err := parseTCPCopyBuffer(in); err == nil {
			t.Errorf("parseTCPCopyBuffer(%q) expected error, got nil", in)
		}
	}
}

// baseTCPServer is a minimal server config using the tcp transport.
const baseTCPServer = `
[[server]]
name = "S1"
bind_addr = "0.0.0.0:3080"
transport = "tcp"
token = "tok"
ports = ["8080"]
`

func TestTCPCopyBuffer_ServerDefaultWhenMissing(t *testing.T) {
	cfg := applyAndValidate(t, baseTCPServer)
	if got := cfg.Servers[0].TCPCopyBuffer; got != "16kb" {
		t.Fatalf("default server TCPCopyBuffer = %q, want %q", got, "16kb")
	}
	if got := cfg.Servers[0].TCPCopyBufferBytes; got != 16384 {
		t.Fatalf("default server TCPCopyBufferBytes = %d, want %d", got, 16384)
	}
}

func TestTCPCopyBuffer_ClientDefaultWhenMissing(t *testing.T) {
	cfg := applyAndValidate(t, baseTCPMuxClient)
	if got := cfg.Clients[0].TCPCopyBuffer; got != "16kb" {
		t.Fatalf("default client TCPCopyBuffer = %q, want %q", got, "16kb")
	}
	if got := cfg.Clients[0].TCPCopyBufferBytes; got != 16384 {
		t.Fatalf("default client TCPCopyBufferBytes = %d, want %d", got, 16384)
	}
}

func TestTCPCopyBuffer_ServerConfigured(t *testing.T) {
	cfg := applyAndValidate(t, baseTCPServer+"tcp_copy_buffer = \"4kb\"\n")
	if got := cfg.Servers[0].TCPCopyBufferBytes; got != 4096 {
		t.Fatalf("server tcp_copy_buffer=\"4kb\" -> bytes %d, want %d", got, 4096)
	}
}

func TestTCPCopyBuffer_ClientConfigured(t *testing.T) {
	cfg := applyAndValidate(t, baseTCPMuxClient+"tcp_copy_buffer = \"4kb\"\n")
	if got := cfg.Clients[0].TCPCopyBufferBytes; got != 4096 {
		t.Fatalf("client tcp_copy_buffer=\"4kb\" -> bytes %d, want %d", got, 4096)
	}
}

func TestTCPCopyBuffer_InvalidRejected(t *testing.T) {
	content := baseTCPMuxClient + "tcp_copy_buffer = \"2mb\"\n"
	cfg, err := loadConfig(writeTempConfig(t, content))
	if err != nil {
		t.Fatalf("loadConfig error: %v", err)
	}
	applyDefaults(cfg)
	err = validateConfig(cfg)
	if err == nil {
		t.Fatal("expected validateConfig to reject out-of-range tcp_copy_buffer")
	}
	if !strings.Contains(err.Error(), "tcp_copy_buffer") {
		t.Fatalf("error %q does not mention tcp_copy_buffer", err)
	}
}

// TestTCPCopyBuffer_DoesNotAffectTunnelBuffer is a regression guard ensuring the
// new option leaves tunnel_tcp_buffer parsing and defaults untouched.
func TestTCPCopyBuffer_DoesNotAffectTunnelBuffer(t *testing.T) {
	// "auto" stays valid for tunnel_tcp_buffer and resolves to 0.
	if got, err := parseTunnelTCPBuffer("auto"); err != nil || got != 0 {
		t.Fatalf("parseTunnelTCPBuffer(\"auto\") = %d, %v; want 0, nil", got, err)
	}
	// "2mb" stays valid for tunnel_tcp_buffer (no 1MB cap there).
	if got, err := parseTunnelTCPBuffer("2mb"); err != nil || got != 2097152 {
		t.Fatalf("parseTunnelTCPBuffer(\"2mb\") = %d, %v; want 2097152, nil", got, err)
	}
	// A client that sets only tcp_copy_buffer must still get the 2mb tunnel default.
	cfg := applyAndValidate(t, baseTCPMuxClient+"tcp_copy_buffer = \"4kb\"\n")
	if got := cfg.Clients[0].TunnelTCPBuffer; got != "2mb" {
		t.Fatalf("TunnelTCPBuffer = %q, want %q", got, "2mb")
	}
	if got := cfg.Clients[0].TunnelTCPBufferBytes; got != 2097152 {
		t.Fatalf("TunnelTCPBufferBytes = %d, want %d", got, 2097152)
	}
}
