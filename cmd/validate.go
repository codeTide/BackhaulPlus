package cmd

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/codeTide/BackhaulPlus/internal/config"
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

	// 2. Validate SNI gateways and build the set of servers referenced by routes.
	referenced := make(map[string]bool)
	if err := validateGateways(cfg, serverByName, referenced); err != nil {
		return err
	}

	// 3. Detect listener conflicts (server ports vs. server ports, and gateway
	//    listen_addr vs. anything). This also validates the port mapping format.
	if err := validateListeners(cfg); err != nil {
		return err
	}

	// 4. Every server must expose some user-facing inbound: either its own
	//    ports, or a reference from at least one sni_gateway route.
	for i := range cfg.Servers {
		s := &cfg.Servers[i]
		if len(s.Ports) == 0 && !referenced[s.Name] {
			return fmt.Errorf("no inbound configured for server %q: set ports or reference it from [[sni_gateway]].routes", s.Name)
		}
	}

	return nil
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

// listenOwner identifies who owns a listen address for conflict reporting.
type listenOwner struct {
	gateway bool
	name    string
}

// validateListeners parses every server's ports into concrete listen addresses,
// rejecting invalid formats, then ensures no two listeners (server ports or
// gateway listen_addr) collide - including wildcard/specific-IP overlaps.
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

	// Server ports first.
	for i := range cfg.Servers {
		s := &cfg.Servers[i]
		owner := listenOwner{name: s.Name}
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
				// Overlap within the same server (e.g. illustrative mappings that
				// reuse a port) is left to the runtime to surface as a real bind
				// error; only cross-server collisions are rejected here.
				if !existing.gateway && existing.name == s.Name {
					continue
				}
				addr := formatListenAddr(a.host, a.port)
				return fmt.Errorf("duplicate port listener %q used by both server %q and server %q", addr, existing.name, s.Name)
			}
		}
	}

	// Gateway listen addresses.
	for i := range cfg.SNIGateways {
		g := &cfg.SNIGateways[i]
		host, port, err := parseHostPort(g.ListenAddr)
		if err != nil {
			return fmt.Errorf("sni_gateway %q: invalid listen_addr %q: %v", g.Name, g.ListenAddr, err)
		}
		owner := listenOwner{gateway: true, name: g.Name}
		existing, ok := register(host, port, owner)
		if ok {
			continue
		}
		addr := formatListenAddr(host, port)
		if existing.gateway {
			return fmt.Errorf("duplicate sni_gateway listen_addr %q used by both sni_gateway %q and sni_gateway %q", addr, existing.name, g.Name)
		}
		return fmt.Errorf("sni_gateway %q listen_addr %q conflicts with a ports listener of server %q", g.Name, addr, existing.name)
	}

	return nil
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
