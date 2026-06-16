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
name = "TR1"
bind_addr = "0.0.0.0:30000"
transport = "tcpmux"
token = "QTRfs754a7"
`

// gatewayBlock is a canonical [[sni_gateway]] referencing the base server.
const gatewayBlock = `
[[sni_gateway]]
name = "PUBLIC-443"
listen_addr = "0.0.0.0:443"
inspect_timeout = 1
default_action = "reject"

routes = [
  { sni = "myket.ir", server = "TR1", target = "10001" },
  { sni = "cafebazaar.ir", server = "TR1", target = "10002" }
]
`

func TestConfig_PortsParsed(t *testing.T) {
	content := baseServer + `
ports = ["20000-20100"]
`
	cfg, err := loadConfig(writeTempConfig(t, content))
	if err != nil {
		t.Fatalf("loadConfig error: %v", err)
	}
	applyDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig error: %v", err)
	}
	if len(cfg.Servers[0].Ports) != 1 || cfg.Servers[0].Ports[0] != "20000-20100" {
		t.Fatalf("ports not parsed: %+v", cfg.Servers[0].Ports)
	}
}

func TestConfig_RawPortsRejected(t *testing.T) {
	content := baseServer + `
raw_ports = ["443-600"]
`
	_, err := loadConfig(writeTempConfig(t, content))
	if err == nil {
		t.Fatal("expected error for removed 'raw_ports' field")
	}
	if !strings.Contains(err.Error(), "ports") {
		t.Fatalf("expected error to mention ports, got: %v", err)
	}
}

func TestConfig_LegacyPerServerSNIRejected(t *testing.T) {
	cases := []string{
		`sni_router = true`,
		`sni_listen_addr = "0.0.0.0:443"`,
		`sni_inspect_timeout = 1`,
		`sni_default_action = "reject"`,
		"sni_routes = [ { sni = \"a.com\", target = \"10001\" } ]",
	}
	for _, legacy := range cases {
		content := baseServer + "ports = [\"10001\"]\n" + legacy + "\n"
		_, err := loadConfig(writeTempConfig(t, content))
		if err == nil {
			t.Fatalf("expected error for legacy field %q", legacy)
		}
		if !strings.Contains(err.Error(), "sni_gateway") {
			t.Fatalf("expected migration message for %q, got: %v", legacy, err)
		}
	}
}

func TestConfig_SNIGatewayParsed(t *testing.T) {
	content := baseServer + gatewayBlock
	cfg, err := loadConfig(writeTempConfig(t, content))
	if err != nil {
		t.Fatalf("loadConfig error: %v", err)
	}
	applyDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig error: %v", err)
	}
	if len(cfg.SNIGateways) != 1 {
		t.Fatalf("expected 1 gateway, got %d", len(cfg.SNIGateways))
	}
	g := cfg.SNIGateways[0]
	if g.ListenAddr != "0.0.0.0:443" || g.InspectTimeout != 1 || g.DefaultAction != "reject" {
		t.Fatalf("gateway fields not parsed: %+v", g)
	}
	r, ok := g.RouteMap["myket.ir"]
	if !ok || r.Server != "TR1" || r.Target != "10001" {
		t.Fatalf("route map not built correctly: %+v", g.RouteMap)
	}
}

func TestConfig_SNIGatewayDefaults(t *testing.T) {
	content := baseServer + `
[[sni_gateway]]
name = "PUBLIC-443"
listen_addr = "0.0.0.0:443"
routes = [ { sni = "a.com", server = "TR1", target = "10001" } ]
`
	cfg, err := loadConfig(writeTempConfig(t, content))
	if err != nil {
		t.Fatalf("loadConfig error: %v", err)
	}
	applyDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig error: %v", err)
	}
	if cfg.SNIGateways[0].InspectTimeout != 1 {
		t.Fatalf("expected default inspect_timeout 1, got %d", cfg.SNIGateways[0].InspectTimeout)
	}
	if cfg.SNIGateways[0].DefaultAction != "reject" {
		t.Fatalf("expected default default_action reject, got %q", cfg.SNIGateways[0].DefaultAction)
	}
}

func TestConfig_RouteUnknownServerRejected(t *testing.T) {
	content := baseServer + `
[[sni_gateway]]
name = "PUBLIC-443"
listen_addr = "0.0.0.0:443"
routes = [ { sni = "a.com", server = "TR2", target = "10001" } ]
`
	err := loadAndValidate(t, content)
	if err == nil {
		t.Fatal("expected error for route referencing unknown server")
	}
	if !strings.Contains(err.Error(), "unknown server") {
		t.Fatalf("expected unknown server message, got: %v", err)
	}
}

func TestConfig_DuplicateServerNameRejected(t *testing.T) {
	content := `
[[server]]
name = "TR1"
bind_addr = "0.0.0.0:30000"
transport = "tcpmux"
ports = ["10001"]

[[server]]
name = "TR1"
bind_addr = "0.0.0.0:30001"
transport = "tcpmux"
ports = ["10002"]
`
	err := loadAndValidate(t, content)
	if err == nil || !strings.Contains(err.Error(), "duplicate server name") {
		t.Fatalf("expected duplicate server name error, got: %v", err)
	}
}

func TestConfig_EmptyServerNameRejected(t *testing.T) {
	content := `
[[server]]
bind_addr = "0.0.0.0:30000"
transport = "tcpmux"
ports = ["10001"]
`
	if err := loadAndValidate(t, content); err == nil {
		t.Fatal("expected error for empty server name")
	}
}

func TestConfig_DuplicateSNIWithinGatewayRejected(t *testing.T) {
	content := baseServer + `
[[sni_gateway]]
name = "PUBLIC-443"
listen_addr = "0.0.0.0:443"
routes = [
  { sni = "Myket.ir", server = "TR1", target = "10001" },
  { sni = "myket.ir.", server = "TR1", target = "10002" }
]
`
	if err := loadAndValidate(t, content); err == nil {
		t.Fatal("expected error for duplicate SNI after normalization")
	}
}

func TestConfig_SNINormalized(t *testing.T) {
	content := baseServer + `
[[sni_gateway]]
name = "PUBLIC-443"
listen_addr = "0.0.0.0:443"
routes = [ { sni = "  Example.COM.  ", server = "TR1", target = "10001" } ]
`
	cfg, err := loadConfig(writeTempConfig(t, content))
	if err != nil {
		t.Fatalf("loadConfig error: %v", err)
	}
	applyDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig error: %v", err)
	}
	if _, ok := cfg.SNIGateways[0].RouteMap["example.com"]; !ok {
		t.Fatalf("expected normalized key 'example.com', got %+v", cfg.SNIGateways[0].RouteMap)
	}
}

func TestConfig_RouteMissingFieldsRejected(t *testing.T) {
	cases := map[string]string{
		"missing sni":    `{ server = "TR1", target = "10001" }`,
		"missing server": `{ sni = "a.com", target = "10001" }`,
		"missing target": `{ sni = "a.com", server = "TR1" }`,
	}
	for name, route := range cases {
		content := baseServer + `
[[sni_gateway]]
name = "PUBLIC-443"
listen_addr = "0.0.0.0:443"
routes = [ ` + route + ` ]
`
		if err := loadAndValidate(t, content); err == nil {
			t.Fatalf("expected error for %s", name)
		}
	}
}

func TestConfig_GatewayWithoutListenAddrRejected(t *testing.T) {
	content := baseServer + `
ports = ["10001"]

[[sni_gateway]]
name = "PUBLIC-443"
routes = [ { sni = "a.com", server = "TR1", target = "10001" } ]
`
	if err := loadAndValidate(t, content); err == nil {
		t.Fatal("expected error for gateway without listen_addr")
	}
}

func TestConfig_UnsupportedDefaultActionRejected(t *testing.T) {
	content := baseServer + `
[[sni_gateway]]
name = "PUBLIC-443"
listen_addr = "0.0.0.0:443"
default_action = "accept"
routes = [ { sni = "a.com", server = "TR1", target = "10001" } ]
`
	if err := loadAndValidate(t, content); err == nil {
		t.Fatal("expected error for unsupported default_action")
	}
}

func TestConfig_DuplicateGatewayListenAddrRejected(t *testing.T) {
	content := baseServer + `
[[sni_gateway]]
name = "G1"
listen_addr = "0.0.0.0:443"
routes = [ { sni = "a.com", server = "TR1", target = "10001" } ]

[[sni_gateway]]
name = "G2"
listen_addr = "0.0.0.0:443"
routes = [ { sni = "b.com", server = "TR1", target = "10002" } ]
`
	err := loadAndValidate(t, content)
	if err == nil || !strings.Contains(err.Error(), "duplicate sni_gateway listen_addr") {
		t.Fatalf("expected duplicate gateway listen_addr error, got: %v", err)
	}
}

func TestConfig_GatewayPortConflict(t *testing.T) {
	for _, ports := range []string{`["443"]`, `["0.0.0.0:443"]`, `["127.0.0.1:443"]`} {
		content := baseServer + `
ports = ` + ports + `

[[sni_gateway]]
name = "PUBLIC-443"
listen_addr = "0.0.0.0:443"
routes = [ { sni = "a.com", server = "TR1", target = "443" } ]
`
		err := loadAndValidate(t, content)
		if err == nil || !strings.Contains(err.Error(), "conflicts") {
			t.Fatalf("expected conflict error for ports %s, got: %v", ports, err)
		}
	}
}

func TestConfig_DuplicatePortsBetweenServers(t *testing.T) {
	content := `
[[server]]
name = "TR1"
bind_addr = "0.0.0.0:30000"
transport = "tcpmux"
ports = ["64335"]

[[server]]
name = "US1"
bind_addr = "0.0.0.0:30001"
transport = "tcpmux"
ports = ["64335"]
`
	err := loadAndValidate(t, content)
	if err == nil || !strings.Contains(err.Error(), "duplicate port listener") {
		t.Fatalf("expected duplicate port listener error, got: %v", err)
	}
}

func TestConfig_SNIOnlyServerAllowed(t *testing.T) {
	content := `
[[server]]
name = "TR1"
bind_addr = "0.0.0.0:20001"
transport = "tcpmux"
token = "x"

[[sni_gateway]]
name = "PUBLIC-443"
listen_addr = "0.0.0.0:443"
routes = [ { sni = "a.com", server = "TR1", target = "443" } ]
`
	if err := loadAndValidate(t, content); err != nil {
		t.Fatalf("expected SNI-only server to be valid, got: %v", err)
	}
}

func TestConfig_NoInboundRejected(t *testing.T) {
	if err := loadAndValidate(t, baseServer); err == nil {
		t.Fatal("expected error when server has neither ports nor a gateway reference")
	}
}

func TestConfig_RouteToUDPServerRejected(t *testing.T) {
	content := `
[[server]]
name = "U1"
bind_addr = "0.0.0.0:30000"
transport = "udp"
token = "x"

[[sni_gateway]]
name = "PUBLIC-443"
listen_addr = "0.0.0.0:443"
routes = [ { sni = "a.com", server = "U1", target = "443" } ]
`
	if err := loadAndValidate(t, content); err == nil {
		t.Fatal("expected error for route to udp server")
	}
}

func TestConfig_MultiServerSNIGateway(t *testing.T) {
	content := `
[[server]]
name = "TR1"
bind_addr = "0.0.0.0:20001"
transport = "tcpmux"
ports = ["64335=64335"]

[[server]]
name = "US1"
bind_addr = "0.0.0.0:20002"
transport = "tcpmux"
ports = ["64336=64335"]

[[sni_gateway]]
name = "PUBLIC-443"
listen_addr = "0.0.0.0:443"
routes = [
  { sni = "tr.example.com", server = "TR1", target = "443" },
  { sni = "us.example.com", server = "US1", target = "443" }
]
`
	if err := loadAndValidate(t, content); err != nil {
		t.Fatalf("expected multi-server SNI gateway config to be valid, got: %v", err)
	}
}

func TestConfig_GatewayBindAddrConflict(t *testing.T) {
	for _, bind := range []string{"0.0.0.0:443", "127.0.0.1:443"} {
		content := `
[[server]]
name = "TR1"
bind_addr = "` + bind + `"
transport = "tcpmux"
ports = ["64335"]

[[sni_gateway]]
name = "PUBLIC-443"
listen_addr = "0.0.0.0:443"
routes = [ { sni = "a.com", server = "TR1", target = "443" } ]
`
		err := loadAndValidate(t, content)
		if err == nil || !strings.Contains(err.Error(), "bind_addr of server") {
			t.Fatalf("expected gateway/bind_addr conflict for bind %s, got: %v", bind, err)
		}
	}
}

func TestConfig_DuplicateBindAddrBetweenServers(t *testing.T) {
	content := `
[[server]]
name = "TR1"
bind_addr = "0.0.0.0:20001"
transport = "tcpmux"
ports = ["64335"]

[[server]]
name = "US1"
bind_addr = "0.0.0.0:20001"
transport = "tcpmux"
ports = ["64336"]
`
	err := loadAndValidate(t, content)
	if err == nil || !strings.Contains(err.Error(), "duplicate server bind_addr") {
		t.Fatalf("expected duplicate bind_addr error, got: %v", err)
	}
}

func TestConfig_InvalidPortFormatRejected(t *testing.T) {
	content := baseServer + `
ports = ["not-a-port"]
`
	if err := loadAndValidate(t, content); err == nil {
		t.Fatal("expected error for invalid port format")
	}
}
