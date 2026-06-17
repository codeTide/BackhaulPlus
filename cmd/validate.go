package cmd

import (
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"

	"github.com/codeTide/BackhaulPlus/internal/config"
	"github.com/codeTide/BackhaulPlus/internal/server/transport"
)

// validateConfig validates servers and SNI gateways, normalizes SNI route keys,
// and detects listener conflicts before any runtime listener is started. It must
// run after applyDefaults so the gateway defaults are already in place.
func validateConfig(cfg *config.Config) error {
	// 1. Server names must be non-empty and unique.
	serverByName := make(map[string]*config.ServerConfig, len(cfg.Servers))
	for i := range cfg.Servers {
		s := &cfg.Servers[i]
		if strings.TrimSpace(s.Name) == "" {
			return fmt.Errorf("server name must not be empty (bind_addr %q)", s.BindAddr)
		}
		if _, ok := serverByName[s.Name]; ok {
			return fmt.Errorf("duplicate server name %q", s.Name)
		}
		serverByName[s.Name] = s
	}

	// 2. Validate gateways and build the set of servers referenced by routes.
	referenced := make(map[string]bool)
	if err := validateGateways(cfg, serverByName, referenced); err != nil {
		return err
	}
	if err := validateHTTPGateways(cfg, serverByName, referenced); err != nil {
		return err
	}

	// 3. Detect listener conflicts (server bind_addr/ports and gateway
	//    listen_addr). This also validates the port mapping format.
	if err := validateListeners(cfg); err != nil {
		return err
	}

	// 4. Every server must expose some user-facing inbound: either its own
	//    ports, or a reference from at least one gateway route.
	for i := range cfg.Servers {
		s := &cfg.Servers[i]
		if len(s.Ports) == 0 && !referenced[s.Name] {
			return fmt.Errorf("no inbound configured for server %q: set ports or reference it from [[sni_gateway]] or [[http_gateway]].routes", s.Name)
		}
	}

	// 5. Parse client-side tunnel TCP buffer sizes into their runtime value.
	for i := range cfg.Clients {
		c := &cfg.Clients[i]
		bytes, err := parseTunnelTCPBuffer(c.TunnelTCPBuffer)
		if err != nil {
			return fmt.Errorf("client %q: invalid tunnel_tcp_buffer: %v", c.Name, err)
		}
		c.TunnelTCPBufferBytes = bytes
	}

	return nil
}

// parseTunnelTCPBuffer parses the client-side tunnel_tcp_buffer option into a
// byte count for use at runtime. The returned value is 0 for "auto" (leave the
// TCP socket buffers to OS/kernel autotuning) or a positive number of bytes for
// a fixed buffer applied equally as the read and write socket buffers.
//
// Accepted forms (trimmed, case-insensitive):
//   - "auto"                  -> 0
//   - "<n>kb"                 -> n * 1024   (positive integer n)
//   - "<n>mb"                 -> n * 1048576 (positive integer n)
//   - "<n>"                   -> n           (raw positive bytes)
//
// Zero is invalid unless the value is exactly "auto". Negative numbers,
// decimals, gb (and other units), and values that overflow int are rejected.
func parseTunnelTCPBuffer(value string) (int, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return 0, fmt.Errorf("value must not be empty")
	}
	if v == "auto" {
		return 0, nil
	}

	multiplier := 1
	numStr := v
	switch {
	case strings.HasSuffix(v, "kb"):
		multiplier = 1024
		numStr = strings.TrimSuffix(v, "kb")
	case strings.HasSuffix(v, "mb"):
		multiplier = 1024 * 1024
		numStr = strings.TrimSuffix(v, "mb")
	}

	// Only plain positive integers are accepted (no sign, no decimals, no
	// other units). This rejects "-1", "1.5mb", "10gb", "kb", "mb", "abc".
	if numStr == "" || !isAllDigits(numStr) {
		return 0, fmt.Errorf("invalid value %q (use \"auto\", a positive size like \"512kb\"/\"1mb\"/\"2mb\", or raw bytes)", value)
	}

	n, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("invalid value %q: %v", value, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("invalid value %q: size must be positive (use \"auto\" for OS autotuning)", value)
	}

	// Reject overflow when applying the unit multiplier.
	if n > math.MaxInt/multiplier {
		return 0, fmt.Errorf("invalid value %q: size is too large", value)
	}
	return n * multiplier, nil
}

// isAllDigits reports whether s is a non-empty string of ASCII digits only.
func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// validateGateways validates each [[sni_gateway]], normalizes its routes into a
// lookup map and records every referenced server name.
func validateGateways(cfg *config.Config, serverByName map[string]*config.ServerConfig, referenced map[string]bool) error {
	for i := range cfg.SNIGateways {
		g := &cfg.SNIGateways[i]

		if strings.TrimSpace(g.ListenAddr) == "" {
			return fmt.Errorf("sni_gateway %q: listen_addr must not be empty", g.Name)
		}
		if g.DefaultAction != "reject" {
			return fmt.Errorf("sni_gateway %q: unsupported default_action %q (only \"reject\" is currently supported)", g.Name, g.DefaultAction)
		}

		routeMap := make(map[string]config.SNIGatewayRoute, len(g.Routes))
		for _, route := range g.Routes {
			sni := normalizeSNIHost(route.SNI)
			if sni == "" {
				return fmt.Errorf("sni_gateway %q: route is missing %q", g.Name, "sni")
			}
			srv := strings.TrimSpace(route.Server)
			if srv == "" {
				return fmt.Errorf("sni_gateway %q: route for %q is missing %q", g.Name, sni, "server")
			}
			target := strings.TrimSpace(route.Target)
			if target == "" {
				return fmt.Errorf("sni_gateway %q: route for %q is missing %q", g.Name, sni, "target")
			}
			if _, ok := routeMap[sni]; ok {
				return fmt.Errorf("sni_gateway %q: duplicate sni route after normalization: %q", g.Name, sni)
			}

			ref, ok := serverByName[srv]
			if !ok {
				return fmt.Errorf("sni_gateway %q route for %q references unknown server %q", g.Name, sni, srv)
			}
			if ref.Transport == config.UDP {
				return fmt.Errorf("sni_gateway %q route for %q references server %q which uses the udp transport (SNI routing requires a TCP/TLS stream)", g.Name, sni, srv)
			}

			routeMap[sni] = config.SNIGatewayRoute{SNI: sni, Server: srv, Target: target}
			referenced[srv] = true
		}
		g.RouteMap = routeMap
	}
	return nil
}

// HTTP gateway max_header_bytes bounds: too small cannot hold a realistic
// request line + Host, too large invites memory abuse.
const (
	minHTTPMaxHeaderBytes = 128
	maxHTTPMaxHeaderBytes = 1 << 20 // 1 MiB
)

// validateHTTPGateways validates each [[http_gateway]], normalizes its routes
// into a lookup map and records every referenced server name.
func validateHTTPGateways(cfg *config.Config, serverByName map[string]*config.ServerConfig, referenced map[string]bool) error {
	for i := range cfg.HTTPGateways {
		g := &cfg.HTTPGateways[i]

		if strings.TrimSpace(g.ListenAddr) == "" {
			return fmt.Errorf("http_gateway %q: listen_addr must not be empty", g.Name)
		}
		if g.DefaultAction != "reject" {
			return fmt.Errorf("http_gateway %q: unsupported default_action %q (only \"reject\" is currently supported)", g.Name, g.DefaultAction)
		}
		if g.MaxHeaderBytes < minHTTPMaxHeaderBytes || g.MaxHeaderBytes > maxHTTPMaxHeaderBytes {
			return fmt.Errorf("http_gateway %q: max_header_bytes %d out of range (must be between %d and %d)", g.Name, g.MaxHeaderBytes, minHTTPMaxHeaderBytes, maxHTTPMaxHeaderBytes)
		}

		routeMap := make(map[string]config.HTTPGatewayRoute, len(g.Routes))
		for _, route := range g.Routes {
			host := transport.NormalizeHTTPHost(route.Host)
			if host == "" {
				return fmt.Errorf("http_gateway %q: route is missing %q", g.Name, "host")
			}
			srv := strings.TrimSpace(route.Server)
			if srv == "" {
				return fmt.Errorf("http_gateway %q: route for %q is missing %q", g.Name, host, "server")
			}
			target := strings.TrimSpace(route.Target)
			if target == "" {
				return fmt.Errorf("http_gateway %q: route for %q is missing %q", g.Name, host, "target")
			}
			if _, ok := routeMap[host]; ok {
				return fmt.Errorf("http_gateway %q: duplicate host route after normalization: %q", g.Name, host)
			}

			ref, ok := serverByName[srv]
			if !ok {
				return fmt.Errorf("http_gateway %q route for %q references unknown server %q", g.Name, host, srv)
			}
			if ref.Transport == config.UDP {
				return fmt.Errorf("http_gateway %q route for %q references server %q which uses the udp transport (HTTP routing requires a TCP stream)", g.Name, host, srv)
			}

			routeMap[host] = config.HTTPGatewayRoute{Host: host, Server: srv, Target: target}
			referenced[srv] = true
		}
		g.RouteMap = routeMap
	}
	return nil
}

// ownerKind classifies what a listen address belongs to, for conflict messages.
type ownerKind int

const (
	ownerBind        ownerKind = iota // a server's bind_addr (tunnel listener)
	ownerPort                         // a server's ports listener
	ownerSNIGateway                   // a sni_gateway listen_addr
	ownerHTTPGateway                  // a http_gateway listen_addr
)

// listenOwner identifies who owns a listen address for conflict reporting.
type listenOwner struct {
	kind ownerKind
	name string
}

// validateListeners parses every concrete listen address opened by the config -
// server bind_addr, server ports, and gateway listen_addr - and ensures no two
// of them collide, including wildcard/specific-IP overlaps. Invalid ports
// formats are rejected here too.
func validateListeners(cfg *config.Config) error {
	// portWildcard[port] -> owner that binds the wildcard address on that port.
	portWildcard := make(map[int]listenOwner)
	// portHosts[port][host] -> owner that binds a specific host on that port.
	portHosts := make(map[int]map[string]listenOwner)

	register := func(host string, port int, owner listenOwner) (listenOwner, bool) {
		wildcard := isWildcardHost(host)
		if existing, ok := portWildcard[port]; ok {
			return existing, false
		}
		if wildcard {
			if hosts := portHosts[port]; len(hosts) > 0 {
				for _, existing := range hosts {
					return existing, false
				}
			}
			portWildcard[port] = owner
			return listenOwner{}, true
		}
		if hosts := portHosts[port]; hosts != nil {
			if existing, ok := hosts[host]; ok {
				return existing, false
			}
		} else {
			portHosts[port] = make(map[string]listenOwner)
		}
		portHosts[port][host] = owner
		return listenOwner{}, true
	}

	// Server bind_addr and ports first.
	for i := range cfg.Servers {
		s := &cfg.Servers[i]

		// bind_addr (the tunnel listener). Skip when empty/unparseable; the
		// runtime surfaces a malformed bind_addr on its own.
		if host, port, err := parseHostPort(s.BindAddr); err == nil {
			existing, ok := register(host, port, listenOwner{kind: ownerBind, name: s.Name})
			if !ok && existing.name != s.Name {
				addr := formatListenAddr(host, port)
				return fmt.Errorf("duplicate server bind_addr %q used by both server %q and server %q", addr, existing.name, s.Name)
			}
		}

		owner := listenOwner{kind: ownerPort, name: s.Name}
		for _, pm := range s.Ports {
			addrs, err := portMappingListenAddrs(pm)
			if err != nil {
				return fmt.Errorf("server %q: %v", s.Name, err)
			}
			for _, a := range addrs {
				existing, ok := register(a.host, a.port, owner)
				if ok {
					continue
				}
				// Overlaps within the same server (e.g. illustrative mappings
				// that reuse a port, or ports that coincide with bind_addr) are
				// left to the runtime to surface as a real bind error; only
				// cross-server collisions are rejected here.
				if existing.name == s.Name {
					continue
				}
				addr := formatListenAddr(a.host, a.port)
				return fmt.Errorf("duplicate port listener %q used by both server %q and server %q", addr, existing.name, s.Name)
			}
		}
	}

	// SNI gateway listen addresses.
	for i := range cfg.SNIGateways {
		g := &cfg.SNIGateways[i]
		host, port, err := parseHostPort(g.ListenAddr)
		if err != nil {
			return fmt.Errorf("sni_gateway %q: invalid listen_addr %q: %v", g.Name, g.ListenAddr, err)
		}
		existing, ok := register(host, port, listenOwner{kind: ownerSNIGateway, name: g.Name})
		if !ok {
			return gatewayConflictError("sni_gateway", g.Name, formatListenAddr(host, port), existing)
		}
	}

	// HTTP gateway listen addresses.
	for i := range cfg.HTTPGateways {
		g := &cfg.HTTPGateways[i]
		host, port, err := parseHostPort(g.ListenAddr)
		if err != nil {
			return fmt.Errorf("http_gateway %q: invalid listen_addr %q: %v", g.Name, g.ListenAddr, err)
		}
		existing, ok := register(host, port, listenOwner{kind: ownerHTTPGateway, name: g.Name})
		if !ok {
			return gatewayConflictError("http_gateway", g.Name, formatListenAddr(host, port), existing)
		}
	}

	return nil
}

// gatewayConflictError builds a clear conflict message for a gateway listen
// address that collides with an already-registered listener.
func gatewayConflictError(kindLabel, name, addr string, existing listenOwner) error {
	switch existing.kind {
	case ownerSNIGateway:
		if kindLabel == "sni_gateway" {
			return fmt.Errorf("duplicate sni_gateway listen_addr %q used by both sni_gateway %q and sni_gateway %q", addr, existing.name, name)
		}
		return fmt.Errorf("%s %q listen_addr %q conflicts with sni_gateway %q", kindLabel, name, addr, existing.name)
	case ownerHTTPGateway:
		if kindLabel == "http_gateway" {
			return fmt.Errorf("duplicate http_gateway listen_addr %q used by both http_gateway %q and http_gateway %q", addr, existing.name, name)
		}
		return fmt.Errorf("%s %q listen_addr %q conflicts with http_gateway %q", kindLabel, name, addr, existing.name)
	case ownerBind:
		return fmt.Errorf("%s %q listen_addr %q conflicts with bind_addr of server %q", kindLabel, name, addr, existing.name)
	default: // ownerPort
		return fmt.Errorf("%s %q listen_addr %q conflicts with a ports listener of server %q", kindLabel, name, addr, existing.name)
	}
}

// listenAddr is a parsed (host, port) listen address.
type listenAddr struct {
	host string
	port int
}

// portMappingListenAddrs parses a single port-mapping entry (the same syntax
// accepted at runtime) and returns the concrete TCP listen addresses it opens.
// It returns an error for any malformed mapping so configuration fails fast.
func portMappingListenAddrs(mapping string) ([]listenAddr, error) {
	parts := strings.SplitN(mapping, "=", 2)
	local := strings.TrimSpace(parts[0])
	if local == "" {
		return nil, fmt.Errorf("invalid port mapping format: %q", mapping)
	}

	// Range form: start-end (numeric only, optional remote after "=").
	if strings.Contains(local, "-") {
		rangeParts := strings.Split(local, "-")
		if len(rangeParts) != 2 {
			return nil, fmt.Errorf("invalid port range format: %q", local)
		}
		startPort, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
		if err != nil || startPort < 1 || startPort > 65535 {
			return nil, fmt.Errorf("invalid start port in range: %q", rangeParts[0])
		}
		endPort, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
		if err != nil || endPort < 1 || endPort > 65535 || endPort < startPort {
			return nil, fmt.Errorf("invalid end port in range: %q", rangeParts[1])
		}
		addrs := make([]listenAddr, 0, endPort-startPort+1)
		for p := startPort; p <= endPort; p++ {
			addrs = append(addrs, listenAddr{host: "", port: p})
		}
		return addrs, nil
	}

	// ip:port form.
	if strings.Contains(local, ":") {
		host, port, err := parseHostPort(local)
		if err != nil {
			return nil, fmt.Errorf("invalid port mapping format: %q", mapping)
		}
		return []listenAddr{{host: host, port: port}}, nil
	}

	// Bare port form.
	port, err := strconv.Atoi(local)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid port format: %q", local)
	}
	return []listenAddr{{host: "", port: port}}, nil
}

// parseHostPort splits a host:port listen address and validates the port.
func parseHostPort(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("invalid port %q", portStr)
	}
	return host, port, nil
}

// isWildcardHost reports whether host binds every interface for its port.
func isWildcardHost(host string) bool {
	switch strings.TrimSpace(host) {
	case "", "0.0.0.0", "::", "[::]", "*":
		return true
	}
	return false
}

// formatListenAddr renders a listen address for error messages, normalizing the
// wildcard host to 0.0.0.0.
func formatListenAddr(host string, port int) string {
	if isWildcardHost(host) {
		host = "0.0.0.0"
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// normalizeSNIHost trims spaces, lowercases and strips a single trailing dot.
func normalizeSNIHost(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, ".")
	return s
}
