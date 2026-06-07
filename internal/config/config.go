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
	RawPorts         []string      `toml:"raw_ports"`
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

	// SNI-based internal TCP routing. When enabled, the server listens on
	// SNIListenAddr, reads the TLS ClientHello (without terminating TLS) and
	// routes the connection into the tunnel based on the SNI value. The route
	// targets are virtual tunnel targets and do not need a matching raw_ports
	// listener.
	SNIRouter         bool             `toml:"sni_router"`
	SNIListenAddr     string           `toml:"sni_listen_addr"`
	SNIInspectTimeout int              `toml:"sni_inspect_timeout"`
	SNIDefaultAction  string           `toml:"sni_default_action"`
	SNIRoutes         []SNIRouteConfig `toml:"sni_routes"`
	// SNIRouteMap is the normalized (lowercase, trimmed) SNI -> target lookup
	// table derived from SNIRoutes during validation. It is not read from TOML.
	SNIRouteMap map[string]string `toml:"-"`
}

// SNIRouteConfig is a single SNI routing rule, expressed in TOML as an inline
// table inside the sni_routes array:
//
//	sni_routes = [
//	  { sni = "myket.ir", target = "10001" },
//	]
type SNIRouteConfig struct {
	SNI    string `toml:"sni"`
	Target string `toml:"target"`
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
}

// Config represents the complete configuration, including both server and client settings.
type Config struct {
	Servers []ServerConfig `toml:"server"`
	Clients []ClientConfig `toml:"client"`
}
