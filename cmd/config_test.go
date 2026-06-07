package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return path
}

// loadAndValidate mirrors the pipeline in Run: load -> defaults -> validate.
func loadAndValidate(t *testing.T, content string) error {
	t.Helper()
	cfg, err := loadConfig(writeTempConfig(t, content))
	if err != nil {
		return err
	}
	applyDefaults(cfg)
	return validateConfig(cfg)
}

const baseServer = `
[[server]]
name = "TU3"
bind_addr = "0.0.0.0:30000"
transport = "tcpmux"
token = "QTRfs754a7"
`

func TestConfig_SNIOnly_Valid(t *testing.T) {
	content := baseServer + `
sni_router = true
sni_listen_addr = "0.0.0.0:443"
sni_inspect_timeout = 2
sni_default_action = "reject"

sni_routes = { "myket.ir" = "10001", "cafebazaar.ir" = "10002" }
`
	cfg, err := loadConfig(writeTempConfig(t, content))
	if err != nil {
		t.Fatalf("loadConfig error: %v", err)
	}
	applyDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig error: %v", err)
	}

	s := cfg.Servers[0]
	if !s.SNIRouter || s.SNIListenAddr != "0.0.0.0:443" {
		t.Fatalf("sni router fields not parsed: %+v", s)
	}
	if s.SNIInspectTimeout != 2 {
		t.Fatalf("expected sni_inspect_timeout 2, got %d", s.SNIInspectTimeout)
	}
	if s.SNIRoutes["myket.ir"] != "10001" || s.SNIRoutes["cafebazaar.ir"] != "10002" {
		t.Fatalf("sni_routes not parsed correctly: %+v", s.SNIRoutes)
	}
}

func TestConfig_RawPortsParsed(t *testing.T) {
	content := baseServer + `
raw_ports = ["20000-20100"]
`
	cfg, err := loadConfig(writeTempConfig(t, content))
	if err != nil {
		t.Fatalf("loadConfig error: %v", err)
	}
	applyDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig error: %v", err)
	}
	if len(cfg.Servers[0].RawPorts) != 1 || cfg.Servers[0].RawPorts[0] != "20000-20100" {
		t.Fatalf("raw_ports not parsed: %+v", cfg.Servers[0].RawPorts)
	}
}

func TestConfig_LegacyPortsRejected(t *testing.T) {
	content := baseServer + `
ports = ["443-600"]
`
	_, err := loadConfig(writeTempConfig(t, content))
	if err == nil {
		t.Fatal("expected error for legacy 'ports' field")
	}
	if !strings.Contains(err.Error(), "raw_ports") {
		t.Fatalf("expected error to mention raw_ports, got: %v", err)
	}
}

func TestConfig_NoInboundRejected(t *testing.T) {
	err := loadAndValidate(t, baseServer)
	if err == nil {
		t.Fatal("expected error when neither raw_ports nor sni_router is set")
	}
}

func TestConfig_SNIRouterWithoutRoutes(t *testing.T) {
	content := baseServer + `
sni_router = true
sni_listen_addr = "0.0.0.0:443"
`
	if err := loadAndValidate(t, content); err == nil {
		t.Fatal("expected error for sni_router without sni_routes")
	}
}

func TestConfig_SNIRouterWithoutListenAddr(t *testing.T) {
	content := baseServer + `
sni_router = true
sni_routes = { "myket.ir" = "10001" }
`
	if err := loadAndValidate(t, content); err == nil {
		t.Fatal("expected error for sni_router without sni_listen_addr")
	}
}

func TestConfig_SNIInspectTimeoutDefault(t *testing.T) {
	content := baseServer + `
sni_router = true
sni_listen_addr = "0.0.0.0:443"
sni_routes = { "myket.ir" = "10001" }
`
	cfg, err := loadConfig(writeTempConfig(t, content))
	if err != nil {
		t.Fatalf("loadConfig error: %v", err)
	}
	applyDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig error: %v", err)
	}
	if cfg.Servers[0].SNIInspectTimeout != 1 {
		t.Fatalf("expected default sni_inspect_timeout 1, got %d", cfg.Servers[0].SNIInspectTimeout)
	}
	if cfg.Servers[0].SNIDefaultAction != "reject" {
		t.Fatalf("expected default sni_default_action reject, got %q", cfg.Servers[0].SNIDefaultAction)
	}
}

func TestConfig_SNIRouteKeysNormalized(t *testing.T) {
	content := baseServer + `
sni_router = true
sni_listen_addr = "0.0.0.0:443"
sni_routes = { "  MyKeT.IR.  " = "10001" }
`
	cfg, err := loadConfig(writeTempConfig(t, content))
	if err != nil {
		t.Fatalf("loadConfig error: %v", err)
	}
	applyDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig error: %v", err)
	}
	if _, ok := cfg.Servers[0].SNIRoutes["myket.ir"]; !ok {
		t.Fatalf("expected normalized key 'myket.ir', got %+v", cfg.Servers[0].SNIRoutes)
	}
}

func TestConfig_UnsupportedDefaultAction(t *testing.T) {
	content := baseServer + `
sni_router = true
sni_listen_addr = "0.0.0.0:443"
sni_default_action = "accept"
sni_routes = { "myket.ir" = "10001" }
`
	if err := loadAndValidate(t, content); err == nil {
		t.Fatal("expected error for unsupported sni_default_action")
	}
}

func TestConfig_UDPWithSNIRejected(t *testing.T) {
	content := `
[[server]]
name = "U"
bind_addr = "0.0.0.0:30000"
transport = "udp"
token = "x"
sni_router = true
sni_listen_addr = "0.0.0.0:443"
sni_routes = { "myket.ir" = "10001" }
`
	if err := loadAndValidate(t, content); err == nil {
		t.Fatal("expected error for udp transport with sni_router")
	}
}

func TestConfig_MixedRawAndSNI(t *testing.T) {
	content := baseServer + `
raw_ports = ["20000-20100"]
sni_router = true
sni_listen_addr = "0.0.0.0:443"
sni_routes = { "myket.ir" = "10001" }
`
	if err := loadAndValidate(t, content); err != nil {
		t.Fatalf("expected mixed raw_ports + sni_router to be valid, got: %v", err)
	}
}
