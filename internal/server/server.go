package server

import (
	"context"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/codeTide/BackhaulPlus/internal/config"
	"github.com/codeTide/BackhaulPlus/internal/server/transport"
	"github.com/codeTide/BackhaulPlus/internal/utils"

	"github.com/sirupsen/logrus"
)

type Server struct {
	config  *config.ServerConfig
	ctx     context.Context
	cancel  context.CancelFunc
	logger  *logrus.Logger
	runtime transport.InboundTarget
	start   func()
}

func NewServer(cfg *config.ServerConfig, parentCtx context.Context) *Server {
	ctx, cancel := context.WithCancel(parentCtx)
	s := &Server{
		config: cfg,
		ctx:    ctx,
		cancel: cancel,
		logger: utils.NewLogger(cfg.LogLevel, cfg.Name),
	}
	s.build()
	return s
}

// Runtime returns the inbound runtime for this server so it can be registered
// for SNI gateway dispatch. It is nil for transports that cannot accept a
// gateway-routed TCP/TLS stream (e.g. udp).
func (s *Server) Runtime() transport.InboundTarget {
	return s.runtime
}

// build constructs the transport runtime for the configured transport and wires
// up its start function. The runtime is created synchronously so it can be
// registered before the SNI gateways begin dispatching.
func (s *Server) build() {
	switch s.config.Transport {
	case config.TCP:
		rt := transport.NewTCPServer(s.ctx, &transport.TcpConfig{
			BindAddr:     s.config.BindAddr,
			Nodelay:      s.config.Nodelay,
			KeepAlive:    time.Duration(s.config.Keepalive) * time.Second,
			Heartbeat:    time.Duration(s.config.Heartbeat) * time.Second,
			Token:        s.config.Token,
			ChannelSize:  s.config.ChannelSize,
			Ports:        s.config.Ports,
			Sniffer:      s.config.Sniffer,
			WebPort:      s.config.WebPort,
			SnifferLog:   s.config.SnifferLog,
			AcceptUDP:    s.config.AcceptUDP,
			AllowMultiIP: s.config.AllowMultiIP,
		}, s.logger)
		s.runtime = rt
		s.start = rt.Start

	case config.TCPMUX:
		rt := transport.NewTcpMuxServer(s.ctx, &transport.TcpMuxConfig{
			BindAddr:         s.config.BindAddr,
			Nodelay:          s.config.Nodelay,
			KeepAlive:        time.Duration(s.config.Keepalive) * time.Second,
			Heartbeat:        time.Duration(s.config.Heartbeat) * time.Second,
			Token:            s.config.Token,
			ChannelSize:      s.config.ChannelSize,
			Ports:            s.config.Ports,
			MuxCon:           s.config.MuxCon,
			MuxVersion:       s.config.MuxVersion,
			MaxFrameSize:     s.config.MaxFrameSize,
			MaxReceiveBuffer: s.config.MaxReceiveBuffer,
			MaxStreamBuffer:  s.config.MaxStreamBuffer,
			Sniffer:          s.config.Sniffer,
			WebPort:          s.config.WebPort,
			SnifferLog:       s.config.SnifferLog,
			AllowMultiIP:     s.config.AllowMultiIP,

			MaxMuxSessions:        s.config.MaxMuxSessions,
			MuxSpareSessions:      s.config.MuxSpareSessions,
			NewConnRequestTimeout: time.Duration(s.config.NewConnRequestTimeout) * time.Second,
		}, s.logger)
		s.runtime = rt
		s.start = rt.Start

	case config.WS, config.WSS:
		rt := transport.NewWSServer(s.ctx, &transport.WsConfig{
			BindAddr:    s.config.BindAddr,
			Nodelay:     s.config.Nodelay,
			KeepAlive:   time.Duration(s.config.Keepalive) * time.Second,
			Heartbeat:   time.Duration(s.config.Heartbeat) * time.Second,
			Token:       s.config.Token,
			ChannelSize: s.config.ChannelSize,
			Ports:       s.config.Ports,
			Sniffer:     s.config.Sniffer,
			WebPort:     s.config.WebPort,
			SnifferLog:  s.config.SnifferLog,
			Mode:        s.config.Transport,
			TLSCertFile: s.config.TLSCertFile,
			TLSKeyFile:  s.config.TLSKeyFile,
		}, s.logger)
		s.runtime = rt
		s.start = rt.Start

	case config.WSMUX, config.WSSMUX:
		rt := transport.NewWSMuxServer(s.ctx, &transport.WsMuxConfig{
			BindAddr:         s.config.BindAddr,
			Nodelay:          s.config.Nodelay,
			KeepAlive:        time.Duration(s.config.Keepalive) * time.Second,
			Heartbeat:        time.Duration(s.config.Heartbeat) * time.Second,
			Token:            s.config.Token,
			ChannelSize:      s.config.ChannelSize,
			Ports:            s.config.Ports,
			MuxCon:           s.config.MuxCon,
			MuxVersion:       s.config.MuxVersion,
			MaxFrameSize:     s.config.MaxFrameSize,
			MaxReceiveBuffer: s.config.MaxReceiveBuffer,
			MaxStreamBuffer:  s.config.MaxStreamBuffer,
			Sniffer:          s.config.Sniffer,
			WebPort:          s.config.WebPort,
			SnifferLog:       s.config.SnifferLog,
			Mode:             s.config.Transport,
			TLSCertFile:      s.config.TLSCertFile,
			TLSKeyFile:       s.config.TLSKeyFile,
		}, s.logger)
		s.runtime = rt
		s.start = rt.Start

	case config.QUIC:
		rt := transport.NewQuicServer(s.ctx, &transport.QuicConfig{
			BindAddr:     s.config.BindAddr,
			Nodelay:      s.config.Nodelay,
			KeepAlive:    time.Duration(s.config.Keepalive) * time.Second,
			Heartbeat:    time.Duration(s.config.Heartbeat) * time.Second,
			Token:        s.config.Token,
			MuxCon:       s.config.MuxCon,
			ChannelSize:  s.config.ChannelSize,
			Ports:        s.config.Ports,
			Sniffer:      s.config.Sniffer,
			WebPort:      s.config.WebPort,
			SnifferLog:   s.config.SnifferLog,
			TLSCertFile:  s.config.TLSCertFile,
			TLSKeyFile:   s.config.TLSKeyFile,
			AllowMultiIP: s.config.AllowMultiIP,
		}, s.logger)
		s.runtime = rt
		s.start = rt.TunnelListener

	case config.UDP:
		rt := transport.NewUDPServer(s.ctx, &transport.UdpConfig{
			BindAddr:    s.config.BindAddr,
			Heartbeat:   time.Duration(s.config.Heartbeat) * time.Second,
			Token:       s.config.Token,
			ChannelSize: s.config.ChannelSize,
			Ports:       s.config.Ports,
			Sniffer:     s.config.Sniffer,
			WebPort:     s.config.WebPort,
			SnifferLog:  s.config.SnifferLog,
		}, s.logger)
		// UDP cannot serve a gateway-routed TCP/TLS stream, so it is not
		// registered as an inbound target.
		s.start = rt.Start

	default:
		s.start = func() { s.logger.Fatal("invalid transport type: ", s.config.Transport) }
	}
}

func (s *Server) Start() {
	// for pprof and debugging
	if s.config.PPROF {
		go func() {
			s.logger.Info("pprof started at port 6060")
			http.ListenAndServe("0.0.0.0:6060", nil)
		}()
	}

	go s.start()

	<-s.ctx.Done()

	s.logger.Info("all workers stopped successfully")

	// suppress other logs
	s.logger.SetLevel(logrus.FatalLevel)
}

// Stop shuts down the server gracefully
func (s *Server) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}
