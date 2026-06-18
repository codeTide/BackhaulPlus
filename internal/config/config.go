package config

import "time"

// TransportType defines the type of transport.
type TransportType string

const (
	TCP    TransportType = "tcp"
	TCPMUX TransportType = "tcpmux"
	WS     TransportType = "ws"
	WSS    TransportType = "wss"
	WSMUX  TransportType = "wsmux"
	WSSMUX TransportType = "wssmux"
	QUIC   TransportType = "quic"
	UDP    TransportType = "udp"
)

// RuntimeConfig contains process-wide runtime maintenance settings. These
// options are top-level because they affect the whole Go process, regardless of
// whether BackhaulPlus is running server or client entries.
type RuntimeConfig struct {
	MemoryReleaseInterval string `toml:"memory_release_interval"`
	AutoRestartInterval   string `toml:"auto_restart_interval"`

	MemoryReleaseIntervalDuration time.Duration `toml:"-"`
	AutoRestartIntervalDuration   time.Duration `toml:"-"`
}

// ServerConfig represents the configuration for the server.
type ServerConfig struct {
	Name             string        `toml:"name"`
	BindAddr         string        `toml:"bind_addr"`
	Transport        TransportType `toml:"transport"`
	Token            string        `toml:"token"`
	AllowMultiIP     bool          `toml:"allow_multi_ip"`
	Nodelay          bool          `toml:"nodelay"`
	Keepalive        int           `toml:"keepalive_period"`
	ChannelSize      int           `toml:"channel_size"`
	LogLevel         string        `toml:"log_level"`
	Ports            []string      `toml:"ports"`
	PPROF            bool          `toml:"pprof"`
	MuxSession       int           `toml:"mux_session"`
	MuxVersion       int           `toml:"mux_version"`
	MaxFrameSize     int           `toml:"mux_framesize"`
	MaxReceiveBuffer int           `toml:"mux_recievebuffer"`
	MaxStreamBuffer  int           `toml:"mux_streambuffer"`
	Sniffer          bool          `toml:"sniffer"`
	WebPort          int           `toml:"web_port"`
	SnifferLog       string        `toml:"sniffer_log"`
	TLSCertFile      string        `toml:"tls_cert"`
	TLSKeyFile       string        `toml:"tls_key"`
	Heartbeat        int           `toml:"heartbeat"`
	MuxCon           int           `toml:"mux_con"`
	AcceptUDP        bool          `toml:"accept_udp"`

	// TCPCopyBuffer controls the userspace buffer used by TCPConnectionHandler.
	// It is the raw string read from TOML, for example "2kb", "4kb", "16kb", or
	// "4096". This is the in-process read/write copy buffer used while relaying
	// data between tunnel/stream and local TCP connections; it is NOT a kernel
	// TCP socket buffer and is unrelated to tunnel_tcp_buffer. Default: "16kb".
	TCPCopyBuffer string `toml:"tcp_copy_buffer"`
	// TCPCopyBufferBytes is the parsed runtime value of TCPCopyBuffer in bytes.
	// It is not read from TOML.
	TCPCopyBufferBytes int `toml:"-"`
}

// SNIGatewayConfig describes a standalone, transport-agnostic SNI gateway. A
// gateway opens a single public TCP listener (e.g. 0.0.0.0:443), reads the TLS
// ClientHello of every connection (without terminating TLS), extracts the SNI
// and dispatches the connection - with the inspected bytes preserved - into the
// runtime of the target [[server]] selected by the matching route.
//
// SNI routing is intentionally decoupled from [[server]] blocks so that several
// servers can share a single public entrypoint (e.g. :443) without each one
// trying to bind the same port.
type SNIGatewayConfig struct {
	Name           string                  `toml:"name"`
	ListenAddr     string                  `toml:"listen_addr"`
	InspectTimeout int                     `toml:"inspect_timeout"`
	DefaultAction  string                  `toml:"default_action"`
	Routes         []SNIGatewayRouteConfig `toml:"routes"`

	// RouteMap is the normalized (trimmed, lowercased, trailing dot removed)
	// SNI -> route lookup table built during validation. It is not read from
	// TOML.
	RouteMap map[string]SNIGatewayRoute `toml:"-"`
}

// SNIGatewayRouteConfig is a single SNI routing rule, expressed in TOML as an
// inline table inside the routes array:
//
//	routes = [
//	  { sni = "www.example.com", server = "TR1", target = "443" },
//	]
type SNIGatewayRouteConfig struct {
	SNI    string `toml:"sni"`
	Server string `toml:"server"`
	Target string `toml:"target"`
}

// SNIGatewayRoute is the normalized, validated form of a routing rule.
type SNIGatewayRoute struct {
	SNI    string
	Server string
	Target string
}

// HTTPGatewayConfig describes a standalone, transport-agnostic HTTP gateway. It
// opens a single public TCP listener (e.g. 0.0.0.0:443), reads the cleartext
// HTTP/1.x request header of every connection (without terminating TLS),
// extracts the Host header and dispatches the connection - with the inspected
// bytes preserved - into the runtime of the target [[server]].
//
// It complements [[sni_gateway]]: SNI gateways route encrypted TLS by their
// ClientHello SNI, while HTTP gateways route cleartext HTTP/XHTTP by their Host
// header. An HTTP gateway cannot route TLS/REALITY traffic because the Host is
// encrypted there.
type HTTPGatewayConfig struct {
	Name           string                   `toml:"name"`
	ListenAddr     string                   `toml:"listen_addr"`
	InspectTimeout int                      `toml:"inspect_timeout"`
	MaxHeaderBytes int                      `toml:"max_header_bytes"`
	DefaultAction  string                   `toml:"default_action"`
	Routes         []HTTPGatewayRouteConfig `toml:"routes"`

	// RouteMap is the normalized (trimmed, lowercased, port and trailing dot
	// removed) Host -> route lookup table built during validation. It is not
	// read from TOML.
	RouteMap map[string]HTTPGatewayRoute `toml:"-"`
}

// HTTPGatewayRouteConfig is a single HTTP Host routing rule, expressed in TOML
// as an inline table inside the routes array:
//
//	routes = [
//	  { host = "tr.example.com", server = "TR1", target = "443" },
//	]
type HTTPGatewayRouteConfig struct {
	Host   string `toml:"host"`
	Server string `toml:"server"`
	Target string `toml:"target"`
}

// HTTPGatewayRoute is the normalized, validated form of an HTTP routing rule.
type HTTPGatewayRoute struct {
	Host   string
	Server string
	Target string
}

// ClientConfig represents the configuration for the client.
type ClientConfig struct {
	Name             string              `toml:"name"`
	RemoteAddr       string              `toml:"remote_addr"`
	Transport        TransportType       `toml:"transport"`
	Token            string              `toml:"token"`
	ConnectionPool   int                 `toml:"connection_pool"`
	RetryInterval    RetryIntervalConfig `toml:"retry_interval"`
	Nodelay          bool                `toml:"nodelay"`
	Keepalive        int                 `toml:"keepalive_period"`
	LogLevel         string              `toml:"log_level"`
	PPROF            bool                `toml:"pprof"`
	MuxSession       int                 `toml:"mux_session"`
	MuxVersion       int                 `toml:"mux_version"`
	MaxFrameSize     int                 `toml:"mux_framesize"`
	MaxReceiveBuffer int                 `toml:"mux_recievebuffer"`
	MaxStreamBuffer  int                 `toml:"mux_streambuffer"`
	Sniffer          bool                `toml:"sniffer"`
	WebPort          int                 `toml:"web_port"`
	SnifferLog       string              `toml:"sniffer_log"`
	DialTimeout      int                 `toml:"dial_timeout"`
	AggressivePool   bool                `toml:"aggressive_pool"`
	EdgeIP           string              `toml:"edge_ip"`

	// TunnelTCPBuffer controls the TCP socket receive/send buffer used for
	// tcpmux tunnel connections. It is the raw string read from TOML
	// (e.g. "auto", "512kb", "1mb", "2mb", "524288"). Default: "2mb".
	TunnelTCPBuffer string `toml:"tunnel_tcp_buffer"`
	// TunnelTCPBufferBytes is the parsed, runtime-only value of
	// TunnelTCPBuffer in bytes. 0 means leave TCP socket buffers to
	// OS/kernel autotuning; a positive value is applied equally as the read
	// and write socket buffers. It is not read from TOML.
	TunnelTCPBufferBytes int `toml:"-"`

	// TCPCopyBuffer controls the userspace buffer used by TCPConnectionHandler.
	// It is the raw string read from TOML, for example "2kb", "4kb", "16kb", or
	// "4096". This is the in-process read/write copy buffer used while relaying
	// data between tunnel/stream and local TCP connections; it is NOT a kernel
	// TCP socket buffer and is unrelated to tunnel_tcp_buffer. Default: "16kb".
	TCPCopyBuffer string `toml:"tcp_copy_buffer"`
	// TCPCopyBufferBytes is the parsed runtime value of TCPCopyBuffer in bytes.
	// It is not read from TOML.
	TCPCopyBufferBytes int `toml:"-"`

	// DialRateLimit optionally caps the number of new remote Dial/connect
	// attempts this client makes to remote_addr per second (e.g. "2/s"). Empty,
	// "0" and "0/s" disable it. It only throttles remote dials, never local
	// Xray/localhost dials. It is the raw string read from TOML.
	DialRateLimit string `toml:"dial_rate_limit"`
	// DialRateLimitConfig is the parsed, runtime-only form of DialRateLimit.
	// It is not read from TOML.
	DialRateLimitConfig DialRateLimitConfig `toml:"-"`
}

// Config represents the complete configuration, including server, client and
// gateway settings.
type Config struct {
	Runtime      RuntimeConfig       `toml:"runtime"`
	Servers      []ServerConfig      `toml:"server"`
	Clients      []ClientConfig      `toml:"client"`
	SNIGateways  []SNIGatewayConfig  `toml:"sni_gateway"`
	HTTPGateways []HTTPGatewayConfig `toml:"http_gateway"`
}
