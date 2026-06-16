package transport

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/sirupsen/logrus"
)

// HTTPGatewayRoute is a single resolved HTTP route: which server runtime to
// dispatch to and the virtual target advertised to the external client.
type HTTPGatewayRoute struct {
	Server string
	Target string
}

// HTTPGatewayConfig is the runtime configuration of a single HTTP gateway.
type HTTPGatewayConfig struct {
	Name           string
	ListenAddr     string
	InspectTimeout time.Duration
	MaxHeaderBytes int
	DefaultAction  string
	// Routes maps a normalized Host (trimmed, lowercased, port and trailing dot
	// removed) to its route.
	Routes map[string]HTTPGatewayRoute
}

// HTTPGateway is a standalone, transport-agnostic HTTP Host router. It opens a
// single public TCP listener, reads each connection's cleartext HTTP/1.x request
// header (without terminating TLS), extracts the Host header and dispatches the
// connection - with the inspected bytes preserved - into the runtime of the
// routed server.
type HTTPGateway struct {
	config   HTTPGatewayConfig
	registry *Registry
	logger   *logrus.Logger
}

// NewHTTPGateway builds an HTTP gateway from its runtime configuration.
func NewHTTPGateway(cfg HTTPGatewayConfig, registry *Registry, logger *logrus.Logger) *HTTPGateway {
	if cfg.InspectTimeout <= 0 {
		cfg.InspectTimeout = time.Second
	}
	if cfg.MaxHeaderBytes <= 0 {
		cfg.MaxHeaderBytes = defaultMaxHTTPHeaderBytes
	}
	if cfg.DefaultAction == "" {
		cfg.DefaultAction = HTTPDefaultActionReject
	}
	return &HTTPGateway{config: cfg, registry: registry, logger: logger}
}

// Start binds the public listener synchronously and, on success, serves
// connections in a background goroutine until ctx is cancelled. It returns an
// error if the listener cannot be bound so the caller can fail fast instead of
// leaving the public HTTP entrypoint silently down. The listener is bound to ctx
// so it is released on shutdown/hot reload, letting a fresh gateway re-bind the
// same port without "address already in use".
func (g *HTTPGateway) Start(ctx context.Context) error {
	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", g.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("http_gateway %q failed to listen on %s: %w", g.config.Name, g.config.ListenAddr, err)
	}

	g.logger.Infof("http_gateway %q listening on %s with %d routes", g.config.Name, g.config.ListenAddr, len(g.config.Routes))

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	go g.acceptLoop(ctx, listener)

	return nil
}

// acceptLoop accepts connections until ctx is cancelled. Accept errors that are
// not caused by shutdown are logged at debug level and the loop continues.
func (g *HTTPGateway) acceptLoop(ctx context.Context, listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				g.logger.Debugf("http_gateway %q accept error: %v", g.config.Name, err)
				continue
			}
		}
		go g.handleConn(conn)
	}
}

func (g *HTTPGateway) handleConn(conn net.Conn) {
	host, firstBytes, err := ReadHTTPHost(conn, g.config.InspectTimeout, g.config.MaxHeaderBytes)
	if err != nil {
		// Keep this at debug level: malformed/timeout requests are common under
		// scanning load and must not produce a log storm.
		g.logger.Debugf("http_gateway %q: failed to read HTTP request from %s: %v", g.config.Name, conn.RemoteAddr(), err)
		conn.Close()
		return
	}

	route, ok := g.config.Routes[host]
	if !ok {
		// Only "reject" is currently supported; unknown Hosts are closed.
		g.logger.Debugf("http_gateway %q: no route for Host %q from %s (default action: %s)", g.config.Name, host, conn.RemoteAddr(), g.config.DefaultAction)
		conn.Close()
		return
	}

	target, ok := g.registry.Lookup(route.Server)
	if !ok || target == nil || !target.IsReady() {
		g.logger.Debugf("http_gateway %q: server %q not ready for Host %q, dropping connection", g.config.Name, route.Server, host)
		conn.Close()
		return
	}

	// Replay the inspected request bytes to the destination so the HTTP/XHTTP
	// request is preserved end-to-end.
	wrapped := NewPrefixedConn(conn, firstBytes)
	reportPort := PortFromTarget(route.Target)

	if !target.EnqueueInbound(wrapped, route.Target, reportPort) {
		g.logger.Debugf("http_gateway %q: server %q could not accept connection for Host %q (channel full/not ready)", g.config.Name, route.Server, host)
		wrapped.Close()
		return
	}

	g.logger.Debugf("http_gateway %q matched Host %q -> server %q target %q", g.config.Name, host, route.Server, route.Target)
}
