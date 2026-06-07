package transport

import (
	"net"
	"sync"

	"github.com/gorilla/websocket"
)

type TunnelChannel struct { // for websocket
	conn *websocket.Conn
	ping chan struct{}
	mu   *sync.Mutex
}

type LocalTCPConn struct {
	conn        net.Conn
	remoteAddr  string
	timeCreated int64
	// reportPort is the port used for usage/traffic statistics. When 0, the
	// port is derived from the connection's local address. This allows
	// SNI-routed connections (which all share the SNI listener's local port) to
	// be accounted per virtual target instead of being merged together.
	reportPort int
}

// usagePort returns the port used for traffic accounting for this connection.
func (l LocalTCPConn) usagePort() int {
	if l.reportPort > 0 {
		return l.reportPort
	}
	if addr, ok := l.conn.LocalAddr().(*net.TCPAddr); ok {
		return addr.Port
	}
	return 0
}

type LocalAcceptUDPConn struct {
	timeCreated int64
	payload     chan []byte
	remoteAddr  string
	listener    *net.UDPConn
	clientAddr  *net.UDPAddr
	IsCongested bool // for congested tcp connection
}

type LocalUDPConn struct {
	timeCreated int64
	payload     chan []byte
	remoteAddr  string
	listener    *net.UDPConn
	addr        *net.UDPAddr
}

type TunnelUDPConn struct {
	timeCreated int64
	payload     chan []byte
	addr        *net.UDPAddr
	listener    *net.UDPConn
	ping        chan struct{}
	mu          *sync.Mutex //mutex for ping channel
}
