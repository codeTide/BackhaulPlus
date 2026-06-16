package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/codeTide/BackhaulPlus/internal/client"
	"github.com/codeTide/BackhaulPlus/internal/config"
	"github.com/codeTide/BackhaulPlus/internal/server"
	"github.com/codeTide/BackhaulPlus/internal/server/transport"
	"github.com/codeTide/BackhaulPlus/internal/utils"

	"github.com/BurntSushi/toml"
)

var (
	logger = utils.NewLogger("info")
)

func Run(configPath string, ctx context.Context) {
	// Load and parse the configuration file
	cfg, err := loadConfig(configPath)
	if err != nil {
		logger.Fatalf("failed to load configuration: %v", err)
	}

	// Apply default values to the configuration
	applyDefaults(cfg)

	// Validate the configuration (also normalizes SNI routes)
	if err := validateConfig(cfg); err != nil {
		logger.Fatalf("invalid configuration: %v", err)
	}

	// Determine whether to run as a server or client
	switch {
	case len(cfg.Servers) > 0:
		// A shared registry maps each server name to its inbound runtime so SNI
		// gateways can dispatch connections into the correct server pipeline.
		registry := transport.NewRegistry()

		// Run multiple servers and register their runtimes.
		for i := range cfg.Servers {
			srv := server.NewServer(&cfg.Servers[i], ctx)
			if rt := srv.Runtime(); rt != nil {
				registry.Register(cfg.Servers[i].Name, rt)
			}
			go srv.Start()
		}

		// Start the SNI gateways once the runtimes are registered. Lookups and
		// readiness checks happen per-connection, so gateways may start before
		// the external clients connect.
		for i := range cfg.SNIGateways {
			gw := transport.NewGateway(gatewayRuntimeConfig(&cfg.SNIGateways[i]), registry, logger)
			go gw.Start(ctx)
		}

		// Wait for shutdown signal
		<-ctx.Done()
		logger.Println("shutting down servers...")

	case len(cfg.Clients) > 0:
		clients := make([]*client.Client, 0, len(cfg.Clients))
		for i := range cfg.Clients {
			if cfg.Clients[i].RemoteAddr == "" {
				logger.Warnf("skipping client %q due to empty remote_addr", cfg.Clients[i].Name)
				continue
			}
			clnt := client.NewClient(&cfg.Clients[i], ctx)
			clients = append(clients, clnt)
			go clnt.Start()
		}

		if len(clients) == 0 {
			logger.Fatalf("no valid client entries found. each [[client]] needs remote_addr")
		}

		<-ctx.Done()
		for _, clnt := range clients {
			clnt.Stop()
		}
		logger.Println("shutting down clients...")

	default:
		logger.Fatalf("neither server nor client configuration is properly set.")
	}
}

// legacyServerSNIFields are the per-server SNI fields that have been removed in
// favour of the standalone [[sni_gateway]] section.
var legacyServerSNIFields = map[string]bool{
	"sni_router":          true,
	"sni_listen_addr":     true,
	"sni_inspect_timeout": true,
	"sni_default_action":  true,
	"sni_routes":          true,
}

// loadConfig loads and parses the TOML configuration file.
func loadConfig(configPath string) (*config.Config, error) {
	var cfg config.Config
	md, err := toml.DecodeFile(configPath, &cfg)
	if err != nil {
		return &cfg, err
	}

	// Detect removed fields so old configs fail loudly instead of being
	// silently ignored. The BurntSushi decoder leaves unknown keys undecoded.
	for _, key := range md.Undecoded() {
		if len(key) == 0 {
			continue
		}
		switch last := key[len(key)-1]; {
		case last == "raw_ports":
			return &cfg, fmt.Errorf("field \"raw_ports\" has been removed; use \"ports\" instead")
		case legacyServerSNIFields[last]:
			return &cfg, fmt.Errorf("per-server sni_router has been removed; use [[sni_gateway]] instead")
		}
	}

	return &cfg, nil
}

// gatewayRuntimeConfig translates a validated config.SNIGatewayConfig into the
// runtime gateway configuration consumed by the transport package.
func gatewayRuntimeConfig(g *config.SNIGatewayConfig) transport.GatewayConfig {
	routes := make(map[string]transport.GatewayRoute, len(g.RouteMap))
	for sni, r := range g.RouteMap {
		routes[sni] = transport.GatewayRoute{Server: r.Server, Target: r.Target}
	}
	return transport.GatewayConfig{
		Name:           g.Name,
		ListenAddr:     g.ListenAddr,
		InspectTimeout: time.Duration(g.InspectTimeout) * time.Second,
		DefaultAction:  g.DefaultAction,
		Routes:         routes,
	}
}
