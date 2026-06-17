package config

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

	// MaxMuxSessions is the hard cap on active + pending mux tunnel sessions a
	// tcpmux server will request from the client. 0 means unlimited (the legacy
	// behaviour).
	MaxMuxSessions int `toml:"max_mux_sessions"`
	// MuxSpareSessions is the number of mux sessions kept above the strictly
	// required capacity so traffic bursts can be served without waiting for a
	// fresh dial. 0 (the default) preserves the legacy behaviour.
	MuxSpareSessions int `toml:"mux_spare_sessions"`
	// NewConnRequestTimeout is the number of seconds after which an unanswered
	// "new session" request is considered stale and reclaimed so the server can
	// retry. 0 lets applyDefaults pick a safe value.
	NewConnRequestTimeout int `toml:"new_conn_request_timeout"`
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
	Name             string        `toml:"name"`
	RemoteAddr       string        `toml:"remote_addr"`
	Transport        TransportType `toml:"transport"`
	Token            string        `toml:"token"`
	ConnectionPool   int           `toml:"connection_pool"`
	RetryInterval    int           `toml:"retry_interval"`
	Nodelay          bool          `toml:"nodelay"`
	Keepalive        int           `toml:"keepalive_period"`
	LogLevel         string        `toml:"log_level"`
	PPROF            bool          `toml:"pprof"`
	MuxSession       int           `toml:"mux_session"`
	MuxVersion       int           `toml:"mux_version"`
	MaxFrameSize     int           `toml:"mux_framesize"`
	MaxReceiveBuffer int           `toml:"mux_recievebuffer"`
	MaxStreamBuffer  int           `toml:"mux_streambuffer"`
	Sniffer          bool          `toml:"sniffer"`
	WebPort          int           `toml:"web_port"`
	SnifferLog       string        `toml:"sniffer_log"`
	DialTimeout      int           `toml:"dial_timeout"`
	AggressivePool   bool          `toml:"aggressive_pool"`
	EdgeIP           string        `toml:"edge_ip"`

	// MaxConnectionPool is the hard cap on active + dialing tunnel sessions a
	// tcpmux client keeps open. 0 means unlimited (the legacy behaviour). When
	// set it must be greater than or equal to connection_pool.
	MaxConnectionPool int `toml:"max_connection_pool"`
}

// Config represents the complete configuration, including server, client and
// gateway settings.
type Config struct {
	Servers      []ServerConfig      `toml:"server"`
	Clients      []ClientConfig      `toml:"client"`
	SNIGateways  []SNIGatewayConfig  `toml:"sni_gateway"`
	HTTPGateways []HTTPGatewayConfig `toml:"http_gateway"`
}
