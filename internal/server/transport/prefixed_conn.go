package transport

import "net"

// PrefixedConn wraps a net.Conn and replays a prefix of bytes (already read from
// the underlying connection) before continuing to read from the connection
// itself. This is used so that bytes consumed while inspecting a connection
// (e.g. a TLS ClientHello for SNI routing) are not lost and are delivered to the
// final destination intact.
//
// All other net.Conn methods (Write, Close, LocalAddr, RemoteAddr, deadlines)
// are delegated to the embedded connection.
type PrefixedConn struct {
	net.Conn
	prefix []byte
	off    int
}

// NewPrefixedConn returns a net.Conn that first yields prefix on Read and then
// continues reading from conn. If prefix is empty, conn is returned unchanged.
func NewPrefixedConn(conn net.Conn, prefix []byte) net.Conn {
	if len(prefix) == 0 {
		return conn
	}
	return &PrefixedConn{Conn: conn, prefix: prefix}
}

func (p *PrefixedConn) Read(b []byte) (int, error) {
	if p.off < len(p.prefix) {
		n := copy(b, p.prefix[p.off:])
		p.off += n
		return n, nil
	}
	return p.Conn.Read(b)
}
