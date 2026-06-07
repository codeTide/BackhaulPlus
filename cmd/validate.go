package cmd

import (
	"fmt"
	"strings"

	"github.com/codeTide/BackhaulPlus/internal/config"
)

// validateConfig validates every server entry and normalizes SNI route keys.
// It must run after applyDefaults so the SNI defaults are already in place.
func validateConfig(cfg *config.Config) error {
	for i := range cfg.Servers {
		if err := validateServer(&cfg.Servers[i]); err != nil {
			return err
		}
	}
	return nil
}

func validateServer(s *config.ServerConfig) error {
	name := s.Name
	if name == "" {
		name = s.BindAddr
	}

	// Normalize the sni_routes slice into a lookup map. SNI keys are trimmed,
	// lowercased, and have a trailing dot removed. Any duplicate SNI after
	// normalization is rejected.
	if len(s.SNIRoutes) > 0 {
		normalized := make(map[string]string, len(s.SNIRoutes))
		for _, route := range s.SNIRoutes {
			key := normalizeSNIHost(route.SNI)
			if key == "" {
				return fmt.Errorf("server %q: sni route is missing %q", name, "sni")
			}
			val := strings.TrimSpace(route.Target)
			if val == "" {
				return fmt.Errorf("server %q: sni route %q is missing %q", name, route.SNI, "target")
			}
			if _, ok := normalized[key]; ok {
				return fmt.Errorf("server %q: duplicate sni route after normalization: %q", name, key)
			}
			normalized[key] = val
		}
		s.SNIRouteMap = normalized
	}

	// Every server must expose some user-facing inbound.
	if len(s.RawPorts) == 0 && !s.SNIRouter {
		return fmt.Errorf("server %q: no inbound configured: set raw_ports or enable sni_router", name)
	}

	if s.SNIRouter {
		if s.Transport == config.UDP {
			return fmt.Errorf("server %q: sni_router is not supported with the udp transport (SNI routing requires a TCP/TLS inbound)", name)
		}
		if s.SNIListenAddr == "" {
			return fmt.Errorf("server %q: sni_router is enabled but sni_listen_addr is empty", name)
		}
		if len(s.SNIRoutes) == 0 {
			return fmt.Errorf("server %q: sni_router is enabled but sni_routes is empty", name)
		}
		if s.SNIDefaultAction != "reject" {
			return fmt.Errorf("server %q: unsupported sni_default_action %q (only \"reject\" is currently supported)", name, s.SNIDefaultAction)
		}
	}

	return nil
}

// normalizeSNIHost trims spaces, lowercases and strips a single trailing dot.
func normalizeSNIHost(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, ".")
	return s
}
