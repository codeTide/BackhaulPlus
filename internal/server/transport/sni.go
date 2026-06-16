package transport

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	// maxClientHelloSize bounds how many bytes we are willing to read while
	// inspecting a TLS ClientHello, to avoid memory abuse from malicious peers.
	maxClientHelloSize = 64 * 1024

	// SNIDefaultActionReject closes connections whose SNI does not match a route.
	SNIDefaultActionReject = "reject"

	tlsRecordTypeHandshake  byte   = 0x16
	tlsHandshakeClientHello byte   = 0x01
	tlsExtensionServerName  uint16 = 0x0000
)

// ReadTLSClientHelloSNI reads the TLS ClientHello from conn (without terminating
// TLS), extracts the SNI server_name, and returns it normalized (lowercase,
// trailing dot stripped). firstBytes contains every byte read from conn so the
// caller can replay them to the destination. On error the caller should close
// the connection.
func ReadTLSClientHelloSNI(conn net.Conn, timeout time.Duration) (sni string, firstBytes []byte, err error) {
	if timeout <= 0 {
		timeout = time.Second
	}
	if derr := conn.SetReadDeadline(time.Now().Add(timeout)); derr != nil {
		return "", nil, derr
	}
	defer conn.SetReadDeadline(time.Time{})

	var raw []byte       // every byte read from the wire (records, for replay)
	var handshake []byte // assembled handshake message payload

	readRecord := func() error {
		header := make([]byte, 5)
		if _, e := io.ReadFull(conn, header); e != nil {
			return e
		}
		if header[0] != tlsRecordTypeHandshake {
			return fmt.Errorf("not a TLS handshake record (type=0x%02x)", header[0])
		}
		recLen := int(binary.BigEndian.Uint16(header[3:5]))
		if recLen == 0 {
			return errors.New("empty TLS record")
		}
		if len(raw)+5+recLen > maxClientHelloSize {
			return errors.New("ClientHello exceeds maximum allowed size")
		}
		body := make([]byte, recLen)
		if _, e := io.ReadFull(conn, body); e != nil {
			return e
		}
		raw = append(raw, header...)
		raw = append(raw, body...)
		handshake = append(handshake, body...)
		return nil
	}

	if e := readRecord(); e != nil {
		return "", raw, e
	}

	// Ensure we have the 4-byte handshake header.
	for len(handshake) < 4 {
		if e := readRecord(); e != nil {
			return "", raw, e
		}
	}

	if handshake[0] != tlsHandshakeClientHello {
		return "", raw, fmt.Errorf("not a ClientHello (handshake type=0x%02x)", handshake[0])
	}

	hsLen := int(handshake[1])<<16 | int(handshake[2])<<8 | int(handshake[3])
	if hsLen > maxClientHelloSize {
		return "", raw, fmt.Errorf("ClientHello too large: %d", hsLen)
	}

	// The ClientHello may be fragmented across multiple records.
	for len(handshake) < 4+hsLen {
		if e := readRecord(); e != nil {
			return "", raw, e
		}
	}

	host, e := parseSNIFromClientHello(handshake[4 : 4+hsLen])
	if e != nil {
		return "", raw, e
	}

	return NormalizeSNIHost(host), raw, nil
}

var errMalformedClientHello = errors.New("malformed TLS ClientHello")

func parseSNIFromClientHello(b []byte) (string, error) {
	// client_version(2) + random(32)
	if len(b) < 34 {
		return "", errMalformedClientHello
	}
	pos := 34

	// session_id
	if pos+1 > len(b) {
		return "", errMalformedClientHello
	}
	sidLen := int(b[pos])
	pos++
	pos += sidLen

	// cipher_suites
	if pos+2 > len(b) {
		return "", errMalformedClientHello
	}
	csLen := int(binary.BigEndian.Uint16(b[pos:]))
	pos += 2
	pos += csLen

	// compression_methods
	if pos+1 > len(b) {
		return "", errMalformedClientHello
	}
	cmLen := int(b[pos])
	pos++
	pos += cmLen

	// extensions
	if pos+2 > len(b) {
		return "", errors.New("ClientHello has no extensions / no SNI")
	}
	extTotal := int(binary.BigEndian.Uint16(b[pos:]))
	pos += 2
	end := pos + extTotal
	if end > len(b) {
		end = len(b)
	}

	for pos+4 <= end {
		extType := binary.BigEndian.Uint16(b[pos:])
		extLen := int(binary.BigEndian.Uint16(b[pos+2:]))
		pos += 4
		if pos+extLen > end {
			return "", errMalformedClientHello
		}
		if extType == tlsExtensionServerName {
			return parseServerNameExtension(b[pos : pos+extLen])
		}
		pos += extLen
	}

	return "", errors.New("no SNI extension found in ClientHello")
}

func parseServerNameExtension(b []byte) (string, error) {
	if len(b) < 2 {
		return "", errMalformedClientHello
	}
	listLen := int(binary.BigEndian.Uint16(b))
	pos := 2
	end := pos + listLen
	if end > len(b) {
		end = len(b)
	}

	for pos+3 <= end {
		nameType := b[pos]
		nameLen := int(binary.BigEndian.Uint16(b[pos+1:]))
		pos += 3
		if pos+nameLen > end {
			return "", errMalformedClientHello
		}
		if nameType == 0 { // host_name
			if nameLen == 0 {
				return "", errors.New("empty SNI host_name")
			}
			return string(b[pos : pos+nameLen]), nil
		}
		pos += nameLen
	}

	return "", errors.New("no host_name in SNI extension")
}

// NormalizeSNIHost normalizes an SNI/host value: trims spaces, lowercases, and
// strips a single trailing dot (the FQDN root label).
func NormalizeSNIHost(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, ".")
	return s
}

// PortFromTarget extracts a numeric port from a tunnel target string such as
// "10001" or "1.1.1.1:5201". It returns 0 if no valid port can be derived.
func PortFromTarget(target string) int {
	t := strings.TrimSpace(target)
	if i := strings.LastIndex(t, ":"); i >= 0 {
		t = t[i+1:]
	}
	p, err := strconv.Atoi(t)
	if err != nil || p < 1 || p > 65535 {
		return 0
	}
	return p
}
