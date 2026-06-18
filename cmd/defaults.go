package cmd

import (
	"strings"

	"github.com/codeTide/BackhaulPlus/internal/config"

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
	// tcpmux tunnel TCP socket buffer (client only). "2mb" preserves the
	// historical hardcoded behavior for configs without this option.
	defaultTunnelTCPBuffer = "2mb"
	// userspace copy buffer used by TCPConnectionHandler on TCP/TCPMUX paths
	// (server and client). "16kb" preserves the historical hardcoded behavior
	// for configs without this option.
	defaultTCPCopyBuffer = "16kb"
	// SNI gateway
	defaultSNIInspectTimeout = 1        // seconds
	defaultSNIDefaultAction  = "reject" // only "reject" is currently supported
	// HTTP gateway
	defaultHTTPInspectTimeout = 1        // seconds
	defaultHTTPMaxHeaderBytes = 32768    // 32 KB
	defaultHTTPDefaultAction  = "reject" // only "reject" is currently supported
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
		// Preserve the historical 16KB userspace copy buffer for configs that
		// do not set tcp_copy_buffer (backward compatibility).
		if strings.TrimSpace(cfg.Servers[i].TCPCopyBuffer) == "" {
			cfg.Servers[i].TCPCopyBuffer = defaultTCPCopyBuffer
		}
	}

	// SNI gateway defaults
	for i := range cfg.SNIGateways {
		if cfg.SNIGateways[i].InspectTimeout <= 0 {
			cfg.SNIGateways[i].InspectTimeout = defaultSNIInspectTimeout
		}
		if cfg.SNIGateways[i].DefaultAction == "" {
			cfg.SNIGateways[i].DefaultAction = defaultSNIDefaultAction
		}
	}

	// HTTP gateway defaults
	for i := range cfg.HTTPGateways {
		if cfg.HTTPGateways[i].InspectTimeout <= 0 {
			cfg.HTTPGateways[i].InspectTimeout = defaultHTTPInspectTimeout
		}
		if cfg.HTTPGateways[i].MaxHeaderBytes <= 0 {
			cfg.HTTPGateways[i].MaxHeaderBytes = defaultHTTPMaxHeaderBytes
		}
		if cfg.HTTPGateways[i].DefaultAction == "" {
			cfg.HTTPGateways[i].DefaultAction = defaultHTTPDefaultAction
		}
	}

	// Client(s)
	for i := range cfg.Clients {
		if cfg.Clients[i].Token == "" {
			cfg.Clients[i].Token = defaultToken
		}
		if _, err := logrus.ParseLevel(cfg.Clients[i].LogLevel); err != nil {
			cfg.Clients[i].LogLevel = defaultLogLevel
		}
		if cfg.Clients[i].RetryInterval <= 0 {
			cfg.Clients[i].RetryInterval = defaultRetryInterval
		}
		if cfg.Clients[i].ConnectionPool <= 0 {
			cfg.Clients[i].ConnectionPool = defaultConnectionPool
		}
		if cfg.Clients[i].MuxSession <= 0 {
			cfg.Clients[i].MuxSession = defaultMuxSession
		}
		if cfg.Clients[i].Keepalive <= 0 {
			cfg.Clients[i].Keepalive = defaultKeepAlive
		}
		if cfg.Clients[i].MuxVersion <= 0 || cfg.Clients[i].MuxVersion > 2 {
			cfg.Clients[i].MuxVersion = defaultMuxVersion
		}
		if cfg.Clients[i].MaxFrameSize <= 0 {
			cfg.Clients[i].MaxFrameSize = defaultMaxFrameSize
		}
		if cfg.Clients[i].MaxReceiveBuffer <= 0 {
			cfg.Clients[i].MaxReceiveBuffer = defaultMaxReceiveBuffer
		}
		if cfg.Clients[i].MaxStreamBuffer <= 0 {
			cfg.Clients[i].MaxStreamBuffer = defaultMaxStreamBuffer
		}
		if cfg.Clients[i].SnifferLog == "" {
			cfg.Clients[i].SnifferLog = defaultSnifferLog
		}
		if cfg.Clients[i].DialTimeout < 1 {
			cfg.Clients[i].DialTimeout = defaultDialTimeout
		}
		// Preserve the historical 2MB tunnel TCP buffer for configs that do
		// not set tunnel_tcp_buffer (backward compatibility).
		if strings.TrimSpace(cfg.Clients[i].TunnelTCPBuffer) == "" {
			cfg.Clients[i].TunnelTCPBuffer = defaultTunnelTCPBuffer
		}
		// Preserve the historical 16KB userspace copy buffer for configs that
		// do not set tcp_copy_buffer (backward compatibility).
		if strings.TrimSpace(cfg.Clients[i].TCPCopyBuffer) == "" {
			cfg.Clients[i].TCPCopyBuffer = defaultTCPCopyBuffer
		}
	}
}
