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
//
// The prefix is copied into a tightly sized slice so the connection does not
// retain a larger backing array (e.g. the preread scratch buffer) for the whole
// lifetime of a long-lived HTTP/XHTTP connection.
func NewPrefixedConn(conn net.Conn, prefix []byte) net.Conn {
	if len(prefix) == 0 {
		return conn
	}
	prefix = append([]byte(nil), prefix...)
	return &PrefixedConn{Conn: conn, prefix: prefix}
}

// Read first drains the prefix, then delegates to the embedded connection. Once
// the prefix has been fully consumed it is released so the bytes can be garbage
// collected instead of being retained for the life of the connection.
func (p *PrefixedConn) Read(b []byte) (int, error) {
	if len(p.prefix) > 0 {
		n := copy(b, p.prefix[p.off:])
		p.off += n
		if p.off >= len(p.prefix) {
			p.prefix = nil
			p.off = 0
		}
		return n, nil
	}
	return p.Conn.Read(b)
}
