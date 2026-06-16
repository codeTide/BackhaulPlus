package transport

import (
	"net"
	"sync"
)

// InboundTarget is the minimal contract a server runtime exposes so that an SNI
// gateway can dispatch an inbound connection into it without knowing the
// underlying transport (tcp, tcpmux, ws, wss, wsmux, wssmux, quic).
type InboundTarget interface {
	// EnqueueInbound hands an accepted inbound connection to the runtime's
	// local pipeline. target is the virtual tunnel target advertised to the
	// external client (e.g. "443"); reportPort is the port used for usage/traffic
	// accounting (0 derives it from the connection's local address). It returns
	// false if the connection could not be enqueued (runtime not ready, channel
	// full, ...), in which case the caller must close conn.
	EnqueueInbound(conn net.Conn, target string, reportPort int) bool

	// IsReady reports whether the runtime currently has an established control
	// channel and can accept inbound connections.
	IsReady() bool
}

// Registry maps a server name to its inbound runtime. It is safe for concurrent
// use; runtimes are registered at startup and looked up per gateway connection.
type Registry struct {
	mu      sync.RWMutex
	targets map[string]InboundTarget
}

// NewRegistry returns an empty runtime registry.
func NewRegistry() *Registry {
	return &Registry{targets: make(map[string]InboundTarget)}
}

// Register associates a server name with its inbound runtime.
func (r *Registry) Register(name string, target InboundTarget) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.targets[name] = target
}

// Lookup returns the runtime registered for name, if any.
func (r *Registry) Lookup(name string) (InboundTarget, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.targets[name]
	return t, ok
}
