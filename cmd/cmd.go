package cmd

import (
	"context"

	"github.com/codeTide/BackhaulPlus/internal/client"
	"github.com/codeTide/BackhaulPlus/internal/config"
	"github.com/codeTide/BackhaulPlus/internal/server"
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

	// Determine whether to run as a server or client
	switch {
	case len(cfg.Servers) > 0:
		// Run multiple servers
		for _, srvConfig := range cfg.Servers {
			srv := server.NewServer(&srvConfig, ctx)
			go srv.Start()
		}
	
		// Wait for shutdown signal
		<-ctx.Done()
		logger.Println("shutting down servers...")
	
	case cfg.Client.RemoteAddr != "":
		clnt := client.NewClient(&cfg.Client, ctx) // client
		go clnt.Start()
	
		<-ctx.Done()
		clnt.Stop()
		logger.Println("shutting down client...")
	
	default:
		logger.Fatalf("neither server nor client configuration is properly set.")
	}	
}

// loadConfig loads and parses the TOML configuration file.
func loadConfig(configPath string) (*config.Config, error) {
	var cfg config.Config
	if _, err := toml.DecodeFile(configPath, &cfg); err != nil {
		return &cfg, err
	}
	return &cfg, nil
}
