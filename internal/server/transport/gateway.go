package transport

import (
	"context"
	"net"
	"time"

	"github.com/sirupsen/logrus"
)

// GatewayRoute is a single resolved SNI route: which server runtime to dispatch
// to and the virtual target advertised to the external client.
type GatewayRoute struct {
	Server string
	Target string
}

// GatewayConfig is the runtime configuration of a single SNI gateway.
type GatewayConfig struct {
	Name           string
	ListenAddr     string
	InspectTimeout time.Duration
	DefaultAction  string
	// Routes maps a normalized (trimmed, lowercased, trailing dot removed) SNI
	// to its route.
	Routes map[string]GatewayRoute
}

// Gateway is a standalone, transport-agnostic SNI router. It opens a single
// public TCP listener, reads each connection's TLS ClientHello (without
// terminating TLS), extracts the SNI and dispatches the connection - with the
// inspected bytes preserved - into the runtime of the routed server.
type Gateway struct {
	config   GatewayConfig
	registry *Registry
	logger   *logrus.Logger
}

// NewGateway builds a gateway from its runtime configuration.
func NewGateway(cfg GatewayConfig, registry *Registry, logger *logrus.Logger) *Gateway {
	if cfg.InspectTimeout <= 0 {
		cfg.InspectTimeout = time.Second
	}
	if cfg.DefaultAction == "" {
		cfg.DefaultAction = SNIDefaultActionReject
	}
	return &Gateway{config: cfg, registry: registry, logger: logger}
}

// Start opens the public listener and serves connections until ctx is cancelled.
// The listener is bound to ctx so it is released on shutdown/hot reload, letting
// a fresh gateway re-bind the same port without "address already in use".
func (g *Gateway) Start(ctx context.Context) {
	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", g.config.ListenAddr)
	if err != nil {
		g.logger.Errorf("sni_gateway %q failed to listen on %s: %v", g.config.Name, g.config.ListenAddr, err)
		return
	}

	g.logger.Infof("sni_gateway %q listening on %s with %d routes", g.config.Name, g.config.ListenAddr, len(g.config.Routes))

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				g.logger.Debugf("sni_gateway %q accept error: %v", g.config.Name, err)
				continue
			}
		}
		go g.handleConn(conn)
	}
}

func (g *Gateway) handleConn(conn net.Conn) {
	sni, firstBytes, err := ReadTLSClientHelloSNI(conn, g.config.InspectTimeout)
	if err != nil {
		// Keep this at debug level: malformed/timeout ClientHellos are common
		// under scanning load and must not produce a log storm.
		g.logger.Debugf("sni_gateway %q: failed to read ClientHello from %s: %v", g.config.Name, conn.RemoteAddr(), err)
		conn.Close()
		return
	}

	route, ok := g.config.Routes[sni]
	if !ok {
		// Only "reject" is currently supported; unknown SNIs are closed.
		g.logger.Debugf("sni_gateway %q: no route for SNI %q from %s (default action: %s)", g.config.Name, sni, conn.RemoteAddr(), g.config.DefaultAction)
		conn.Close()
		return
	}

	target, ok := g.registry.Lookup(route.Server)
	if !ok || target == nil || !target.IsReady() {
		g.logger.Debugf("sni_gateway %q: server %q not ready for SNI %q, dropping connection", g.config.Name, route.Server, sni)
		conn.Close()
		return
	}

	// Replay the inspected ClientHello bytes to the destination so the
	// TLS/REALITY/XHTTP handshake is preserved end-to-end.
	wrapped := NewPrefixedConn(conn, firstBytes)
	reportPort := PortFromTarget(route.Target)

	if !target.EnqueueInbound(wrapped, route.Target, reportPort) {
		g.logger.Debugf("sni_gateway %q: server %q could not accept connection for SNI %q (channel full/not ready)", g.config.Name, route.Server, sni)
		wrapped.Close()
		return
	}

	g.logger.Debugf("sni_gateway %q matched SNI %q -> server %q target %q", g.config.Name, sni, route.Server, route.Target)
}
