package cmd

import (
	"strings"
	"testing"
)

func TestConfig_HTTPGatewayParsed(t *testing.T) {
	content := baseServer + `
[[http_gateway]]
name = "PUBLIC-XHTTP-443"
listen_addr = "0.0.0.0:443"
routes = [ { host = "tr.example.com", server = "TR1", target = "443" } ]
`
	cfg, err := loadConfig(writeTempConfig(t, content))
	if err != nil {
		t.Fatalf("loadConfig error: %v", err)
	}
	applyDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig error: %v", err)
	}
	if len(cfg.HTTPGateways) != 1 {
		t.Fatalf("expected 1 http gateway, got %d", len(cfg.HTTPGateways))
	}
	g := cfg.HTTPGateways[0]
	if g.ListenAddr != "0.0.0.0:443" {
		t.Fatalf("listen_addr not parsed: %+v", g)
	}
	if g.InspectTimeout != 1 {
		t.Fatalf("expected default inspect_timeout 1, got %d", g.InspectTimeout)
	}
	if g.MaxHeaderBytes != 32768 {
		t.Fatalf("expected default max_header_bytes 32768, got %d", g.MaxHeaderBytes)
	}
	if g.DefaultAction != "reject" {
		t.Fatalf("expected default default_action reject, got %q", g.DefaultAction)
	}
	r, ok := g.RouteMap["tr.example.com"]
	if !ok || r.Server != "TR1" || r.Target != "443" {
		t.Fatalf("route map not built correctly: %+v", g.RouteMap)
	}
}

func TestConfig_HTTPGatewayWithoutListenAddrRejected(t *testing.T) {
	content := baseServer + `
ports = ["10001"]

[[http_gateway]]
name = "G"
routes = [ { host = "a.com", server = "TR1", target = "443" } ]
`
	if err := loadAndValidate(t, content); err == nil {
		t.Fatal("expected error for http_gateway without listen_addr")
	}
}

func TestConfig_HTTPGatewayUnsupportedDefaultActionRejected(t *testing.T) {
	content := baseServer + `
[[http_gateway]]
name = "G"
listen_addr = "0.0.0.0:443"
default_action = "accept"
routes = [ { host = "a.com", server = "TR1", target = "443" } ]
`
	if err := loadAndValidate(t, content); err == nil {
		t.Fatal("expected error for unsupported default_action")
	}
}

func TestConfig_HTTPGatewayRouteMissingFieldsRejected(t *testing.T) {
	cases := map[string]string{
		"missing host":   `{ server = "TR1", target = "443" }`,
		"missing server": `{ host = "a.com", target = "443" }`,
		"missing target": `{ host = "a.com", server = "TR1" }`,
	}
	for name, route := range cases {
		content := baseServer + `
[[http_gateway]]
name = "G"
listen_addr = "0.0.0.0:443"
routes = [ ` + route + ` ]
`
		if err := loadAndValidate(t, content); err == nil {
			t.Fatalf("expected error for %s", name)
		}
	}
}

func TestConfig_HTTPGatewayDuplicateHostRejected(t *testing.T) {
	content := baseServer + `
[[http_gateway]]
name = "G"
listen_addr = "0.0.0.0:443"
routes = [
  { host = "Example.COM.", server = "TR1", target = "443" },
  { host = "example.com", server = "TR1", target = "8443" }
]
`
	if err := loadAndValidate(t, content); err == nil {
		t.Fatal("expected error for duplicate host after normalization")
	}
}

func TestConfig_HTTPGatewayHostNormalized(t *testing.T) {
	content := baseServer + `
[[http_gateway]]
name = "G"
listen_addr = "0.0.0.0:443"
routes = [ { host = "  Example.COM:443  ", server = "TR1", target = "443" } ]
`
	cfg, err := loadConfig(writeTempConfig(t, content))
	if err != nil {
		t.Fatalf("loadConfig error: %v", err)
	}
	applyDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig error: %v", err)
	}
	if _, ok := cfg.HTTPGateways[0].RouteMap["example.com"]; !ok {
		t.Fatalf("expected normalized key 'example.com', got %+v", cfg.HTTPGateways[0].RouteMap)
	}
}

func TestConfig_HTTPGatewayUnknownServerRejected(t *testing.T) {
	content := baseServer + `
[[http_gateway]]
name = "G"
listen_addr = "0.0.0.0:443"
routes = [ { host = "a.com", server = "TR2", target = "443" } ]
`
	err := loadAndValidate(t, content)
	if err == nil || !strings.Contains(err.Error(), "unknown server") {
		t.Fatalf("expected unknown server error, got: %v", err)
	}
}

func TestConfig_HTTPGatewayRouteToUDPServerRejected(t *testing.T) {
	content := `
[[server]]
name = "U1"
bind_addr = "0.0.0.0:30000"
transport = "udp"
token = "x"

[[http_gateway]]
name = "G"
listen_addr = "0.0.0.0:443"
routes = [ { host = "a.com", server = "U1", target = "443" } ]
`
	if err := loadAndValidate(t, content); err == nil {
		t.Fatal("expected error for route to udp server")
	}
}

func TestConfig_HTTPGatewayOnlyServerAllowed(t *testing.T) {
	content := `
[[server]]
name = "TR1"
bind_addr = "0.0.0.0:20001"
transport = "tcpmux"
token = "x"

[[http_gateway]]
name = "G"
listen_addr = "0.0.0.0:443"
routes = [ { host = "a.com", server = "TR1", target = "443" } ]
`
	if err := loadAndValidate(t, content); err != nil {
		t.Fatalf("expected http-only server to be valid, got: %v", err)
	}
}

func TestConfig_HTTPGatewayNoInboundRejected(t *testing.T) {
	// A server referenced by no gateway and with no ports must be rejected.
	content := `
[[server]]
name = "TR1"
bind_addr = "0.0.0.0:20001"
transport = "tcpmux"
token = "x"

[[server]]
name = "US1"
bind_addr = "0.0.0.0:20002"
transport = "tcpmux"
token = "x"

[[http_gateway]]
name = "G"
listen_addr = "0.0.0.0:443"
routes = [ { host = "a.com", server = "TR1", target = "443" } ]
`
	err := loadAndValidate(t, content)
	if err == nil || !strings.Contains(err.Error(), "no inbound configured for server \"US1\"") {
		t.Fatalf("expected no-inbound error for US1, got: %v", err)
	}
}

func TestConfig_HTTPGatewayMaxHeaderBytesBounds(t *testing.T) {
	for _, v := range []string{"64", "2000000"} {
		content := baseServer + `
[[http_gateway]]
name = "G"
listen_addr = "0.0.0.0:443"
max_header_bytes = ` + v + `
routes = [ { host = "a.com", server = "TR1", target = "443" } ]
`
		if err := loadAndValidate(t, content); err == nil {
			t.Fatalf("expected error for out-of-range max_header_bytes %s", v)
		}
	}
}

func TestConfig_HTTPGatewayListenAddrConflicts(t *testing.T) {
	// conflict with server bind_addr
	bindConflict := `
[[server]]
name = "TR1"
bind_addr = "0.0.0.0:443"
transport = "tcpmux"
ports = ["64335"]

[[http_gateway]]
name = "G"
listen_addr = "127.0.0.1:443"
routes = [ { host = "a.com", server = "TR1", target = "443" } ]
`
	if err := loadAndValidate(t, bindConflict); err == nil || !strings.Contains(err.Error(), "bind_addr of server") {
		t.Fatalf("expected bind_addr conflict, got: %v", err)
	}

	// conflict with server ports
	portConflict := baseServer + `
ports = ["443"]

[[http_gateway]]
name = "G"
listen_addr = "0.0.0.0:443"
routes = [ { host = "a.com", server = "TR1", target = "443" } ]
`
	if err := loadAndValidate(t, portConflict); err == nil || !strings.Contains(err.Error(), "ports listener") {
		t.Fatalf("expected ports conflict, got: %v", err)
	}

	// conflict with sni_gateway
	sniConflict := baseServer + `
[[sni_gateway]]
name = "S"
listen_addr = "0.0.0.0:443"
routes = [ { sni = "tls.example.com", server = "TR1", target = "443" } ]

[[http_gateway]]
name = "H"
listen_addr = "0.0.0.0:443"
routes = [ { host = "http.example.com", server = "TR1", target = "443" } ]
`
	if err := loadAndValidate(t, sniConflict); err == nil || !strings.Contains(err.Error(), "sni_gateway") {
		t.Fatalf("expected sni/http gateway conflict, got: %v", err)
	}

	// conflict between two http gateways
	httpConflict := baseServer + `
[[http_gateway]]
name = "H1"
listen_addr = "0.0.0.0:443"
routes = [ { host = "a.example.com", server = "TR1", target = "443" } ]

[[http_gateway]]
name = "H2"
listen_addr = "0.0.0.0:443"
routes = [ { host = "b.example.com", server = "TR1", target = "443" } ]
`
	if err := loadAndValidate(t, httpConflict); err == nil || !strings.Contains(err.Error(), "duplicate http_gateway listen_addr") {
		t.Fatalf("expected duplicate http_gateway conflict, got: %v", err)
	}
}

func TestConfig_SNIAndHTTPGatewayDifferentPortsAllowed(t *testing.T) {
	content := baseServer + `
[[sni_gateway]]
name = "S"
listen_addr = "0.0.0.0:443"
routes = [ { sni = "tls.example.com", server = "TR1", target = "443" } ]

[[http_gateway]]
name = "H"
listen_addr = "0.0.0.0:8443"
routes = [ { host = "http.example.com", server = "TR1", target = "8443" } ]
`
	if err := loadAndValidate(t, content); err != nil {
		t.Fatalf("expected sni+http on different ports to be valid, got: %v", err)
	}
}

func TestConfig_SameHostDifferentHTTPGatewaysAllowed(t *testing.T) {
	content := baseServer + `
[[http_gateway]]
name = "H1"
listen_addr = "0.0.0.0:443"
routes = [ { host = "a.example.com", server = "TR1", target = "443" } ]

[[http_gateway]]
name = "H2"
listen_addr = "0.0.0.0:8443"
routes = [ { host = "a.example.com", server = "TR1", target = "8443" } ]
`
	if err := loadAndValidate(t, content); err != nil {
		t.Fatalf("expected same host across gateways on different ports to be valid, got: %v", err)
	}
}

func TestConfig_MultiServerHTTPGateway(t *testing.T) {
	content := `
[[server]]
name = "TR1"
bind_addr = "0.0.0.0:20001"
transport = "tcpmux"
token = "example-token"

[[server]]
name = "US1"
bind_addr = "0.0.0.0:20002"
transport = "tcpmux"
token = "example-token"

[[http_gateway]]
name = "PUBLIC-XHTTP-443"
listen_addr = "0.0.0.0:443"
inspect_timeout = 1
max_header_bytes = 32768
default_action = "reject"
routes = [
  { host = "tr.example.com", server = "TR1", target = "443" },
  { host = "us.example.com", server = "US1", target = "443" }
]
`
	if err := loadAndValidate(t, content); err != nil {
		t.Fatalf("expected multi-server http gateway config to be valid, got: %v", err)
	}
}
