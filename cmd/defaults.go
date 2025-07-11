package cmd

import (
	"github.com/musix/backhaul/internal/config"

	"github.com/sirupsen/logrus"
)

const ( // Default values
	defaultToken          = "musix"
	defaultChannelSize    = 2048
	defaultRetryInterval  = 3 // only for client
	defaultConnectionPool = 8
	defaultLogLevel       = "info"
	defaultMuxSession     = 1
	defaultKeepAlive      = 75
	deafultHeartbeat      = 40 // 40 seconds
	defaultDialTimeout    = 10 // 10 seconds
	// related to smux
	defaultMuxVersion       = 1
	defaultMaxFrameSize     = 32768   // 32KB
	defaultMaxReceiveBuffer = 4194304 // 4MB
	defaultMaxStreamBuffer  = 65536   // 256KB
	defaultSnifferLog       = "backhaul.json"
	defaultMuxCon           = 8
)

func applyDefaults(cfg *config.Config) {
	// Token
	for i := range cfg.Servers {
		if cfg.Servers[i].Token == "" {
			cfg.Servers[i].Token = defaultToken
		}
		if cfg.Servers[i].ChannelSize <= 0 {
			cfg.Servers[i].ChannelSize = defaultChannelSize
		}
		if _, err := logrus.ParseLevel(cfg.Servers[i].LogLevel); err != nil {
			cfg.Servers[i].LogLevel = defaultLogLevel
		}
		if cfg.Servers[i].MuxSession <= 0 {
			cfg.Servers[i].MuxSession = defaultMuxSession
		}
		if cfg.Servers[i].Keepalive <= 0 {
			cfg.Servers[i].Keepalive = defaultKeepAlive
		}
		if cfg.Servers[i].MuxVersion <= 0 || cfg.Servers[i].MuxVersion > 2 {
			cfg.Servers[i].MuxVersion = defaultMuxVersion
		}
		if cfg.Servers[i].MaxFrameSize <= 0 {
			cfg.Servers[i].MaxFrameSize = defaultMaxFrameSize
		}
		if cfg.Servers[i].MaxReceiveBuffer <= 0 {
			cfg.Servers[i].MaxReceiveBuffer = defaultMaxReceiveBuffer
		}
		if cfg.Servers[i].MaxStreamBuffer <= 0 {
			cfg.Servers[i].MaxStreamBuffer = defaultMaxStreamBuffer
		}
		if cfg.Servers[i].SnifferLog == "" {
			cfg.Servers[i].SnifferLog = defaultSnifferLog
		}
		if cfg.Servers[i].Heartbeat < 1 {
			cfg.Servers[i].Heartbeat = deafultHeartbeat
		}
		if cfg.Servers[i].MuxCon < 1 {
			cfg.Servers[i].MuxCon = defaultMuxCon
		}
	}

	// Client
	if cfg.Client.Token == "" {
		cfg.Client.Token = defaultToken
	}
	if _, err := logrus.ParseLevel(cfg.Client.LogLevel); err != nil {
		cfg.Client.LogLevel = defaultLogLevel
	}
	if cfg.Client.RetryInterval <= 0 {
		cfg.Client.RetryInterval = defaultRetryInterval
	}
	if cfg.Client.ConnectionPool <= 0 {
		cfg.Client.ConnectionPool = defaultConnectionPool
	}
	if cfg.Client.MuxSession <= 0 {
		cfg.Client.MuxSession = defaultMuxSession
	}
	if cfg.Client.Keepalive <= 0 {
		cfg.Client.Keepalive = defaultKeepAlive
	}
	if cfg.Client.MuxVersion <= 0 || cfg.Client.MuxVersion > 2 {
		cfg.Client.MuxVersion = defaultMuxVersion
	}
	if cfg.Client.MaxFrameSize <= 0 {
		cfg.Client.MaxFrameSize = defaultMaxFrameSize
	}
	if cfg.Client.MaxReceiveBuffer <= 0 {
		cfg.Client.MaxReceiveBuffer = defaultMaxReceiveBuffer
	}
	if cfg.Client.MaxStreamBuffer <= 0 {
		cfg.Client.MaxStreamBuffer = defaultMaxStreamBuffer
	}
	if cfg.Client.SnifferLog == "" {
		cfg.Client.SnifferLog = defaultSnifferLog
	}
	if cfg.Client.DialTimeout < 1 {
		cfg.Client.DialTimeout = defaultDialTimeout
	}
}