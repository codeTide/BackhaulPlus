package transport

import (
	"bytes"
	"errors"
	"net"
	"strings"
	"time"
)

const (
	// defaultMaxHTTPHeaderBytes bounds how many bytes we read while inspecting an
	// HTTP request header, to avoid memory abuse from malicious peers.
	defaultMaxHTTPHeaderBytes = 32 * 1024

	// prereadChunkSize is the size of the scratch buffer used while reading the
	// HTTP request header in chunks. Reading in chunks (instead of one byte per
	// syscall) keeps preread cheap under high connection counts.
	prereadChunkSize = 4096

	// HTTPDefaultActionReject closes connections whose Host does not match a route.
	HTTPDefaultActionReject = "reject"
)

var (
	errHTTPHeaderTooLarge     = errors.New("HTTP header exceeds maximum allowed size")
	errMalformedRequestLine   = errors.New("malformed HTTP request line")
	errUnsupportedHTTPVersion = errors.New("unsupported HTTP version (only HTTP/1.0 and HTTP/1.1 are supported)")
	errNoHostHeader           = errors.New("no Host header in HTTP request")
	errMultipleHostHeaders    = errors.New("multiple Host headers in HTTP request")
	errEmptyHost              = errors.New("empty Host header")
	errNonHTTPTraffic         = errors.New("non-HTTP traffic on cleartext HTTP gateway")
)

var (
	httpHeaderTerminator = []byte("\r\n\r\n")
	crlf                 = []byte("\r\n")
	hostHeaderName       = []byte("host")
)

// ReadHTTPHost reads a cleartext HTTP/1.x request header from conn (without
// terminating TLS), extracts the Host header and returns it normalized.
// firstBytes contains every byte read from conn so the caller can replay them to
// the destination with a PrefixedConn. On error the caller should close the
// connection.
//
// The header is read in chunks rather than one byte at a time. Because a single
// chunk may read a few bytes of the request body past the header terminator,
// those read-ahead bytes are always included in firstBytes: the request body is
// never lost, it is replayed downstream via PrefixedConn. Only the header bytes
// (up to and including the terminating CRLFCRLF) are parsed.
//
// This only supports cleartext HTTP/1.x Host routing: TLS/REALITY/HTTPS, HTTP/2
// and h2c are rejected and not terminated here.
func ReadHTTPHost(conn net.Conn, timeout time.Duration, maxHeaderBytes int) (host string, firstBytes []byte, err error) {
	if timeout <= 0 {
		timeout = time.Second
	}
	if maxHeaderBytes <= 0 {
		maxHeaderBytes = defaultMaxHTTPHeaderBytes
	}
	if derr := conn.SetReadDeadline(time.Now().Add(timeout)); derr != nil {
		return "", nil, derr
	}
	defer conn.SetReadDeadline(time.Time{})

	buf := make([]byte, 0, min(maxHeaderBytes, prereadChunkSize))
	tmp := make([]byte, prereadChunkSize)
	requestLineChecked := false

	for {
		n, e := conn.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)

			// Reject obvious non-HTTP (e.g. TLS) traffic as soon as the first byte
			// is available, so probes/scanners on :443 are dropped immediately
			// rather than after the inspect timeout.
			if rerr := rejectObviousNonHTTPPrefix(buf); rerr != nil {
				return "", buf, rerr
			}

			// Validate the request line as soon as a full one is available so an
			// HTTP/2 preface or other unsupported version is rejected early.
			if !requestLineChecked {
				if lineEnd := bytes.Index(buf, crlf); lineEnd >= 0 {
					if verr := validateRequestLineBytes(buf[:lineEnd]); verr != nil {
						return "", buf, verr
					}
					requestLineChecked = true
				}
			}

			if idx := bytes.Index(buf, httpHeaderTerminator); idx >= 0 {
				headerEnd := idx + len(httpHeaderTerminator)
				if headerEnd > maxHeaderBytes {
					return "", buf, errHTTPHeaderTooLarge
				}
				h, perr := parseHTTPHostFromHeader(buf[:headerEnd])
				if perr != nil {
					return "", buf, perr
				}
				// Return all bytes read, including any small read-ahead body bytes
				// past the header, so the request is preserved end-to-end.
				return h, buf, nil
			}

			if len(buf) > maxHeaderBytes {
				return "", buf, errHTTPHeaderTooLarge
			}
		}

		if e != nil {
			return "", buf, e
		}
	}
}

// rejectObviousNonHTTPPrefix rejects input that cannot be the start of a
// cleartext HTTP/1.x request. A valid request line begins with a method token,
// which is printable ASCII; a TLS record (handshake/alert/etc.) or any control
// byte at the very front means a non-HTTP client (commonly a TLS probe on :443)
// hit the cleartext HTTP gateway. This does NOT whitelist HTTP methods: any
// method with a valid request line is accepted by validateRequestLineBytes.
func rejectObviousNonHTTPPrefix(buf []byte) error {
	if len(buf) == 0 {
		return nil
	}
	switch buf[0] {
	// TLS record content types: change_cipher_spec(0x14), alert(0x15),
	// handshake(0x16), application_data(0x17).
	case 0x14, 0x15, 0x16, 0x17:
		return errNonHTTPTraffic
	}
	// HTTP method tokens are printable ASCII; a NUL or other control byte (or
	// DEL) can never start a request line.
	if buf[0] < 0x20 || buf[0] == 0x7f {
		return errNonHTTPTraffic
	}
	return nil
}

// parseHTTPHostFromHeader parses a full HTTP/1.x request header (ending in
// CRLFCRLF), validates the request line and returns the single normalized Host.
// It works on byte slices and only converts the final Host value to a string to
// keep allocations low under high request rates.
func parseHTTPHostFromHeader(b []byte) (string, error) {
	lineEnd := bytes.Index(b, crlf)
	if lineEnd <= 0 {
		return "", errMalformedRequestLine
	}
	if err := validateRequestLineBytes(b[:lineEnd]); err != nil {
		return "", err
	}

	var hostVal []byte
	found := 0
	rest := b[lineEnd+len(crlf):]
	for len(rest) > 0 {
		nl := bytes.Index(rest, crlf)
		if nl < 0 {
			break
		}
		line := rest[:nl]
		rest = rest[nl+len(crlf):]
		if len(line) == 0 {
			// Blank line terminates the header block.
			break
		}
		colon := bytes.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		if bytes.EqualFold(bytes.TrimSpace(line[:colon]), hostHeaderName) {
			found++
			hostVal = line[colon+1:]
		}
	}
	if found == 0 {
		return "", errNoHostHeader
	}
	if found > 1 {
		return "", errMultipleHostHeaders
	}

	host := NormalizeHTTPHost(string(hostVal))
	if host == "" {
		return "", errEmptyHost
	}
	return host, nil
}

// validateRequestLineBytes checks a request line of the form
// "METHOD SP REQUEST-TARGET SP HTTP/1.x". It requires exactly three
// space-separated fields. HTTP/2 prefaces (e.g. "PRI * HTTP/2.0") and any
// non-1.x version are rejected. The method is not restricted to a whitelist:
// any non-empty method token is accepted.
func validateRequestLineBytes(line []byte) error {
	first := bytes.IndexByte(line, ' ')
	if first <= 0 {
		// Missing method/target separator, or empty method.
		return errMalformedRequestLine
	}
	rest := line[first+1:]
	second := bytes.IndexByte(rest, ' ')
	if second < 0 {
		return errMalformedRequestLine
	}
	target := rest[:second]
	version := rest[second+1:]
	if len(target) == 0 {
		return errMalformedRequestLine
	}
	// Exactly three fields: a trailing space (a fourth field) is malformed.
	if bytes.IndexByte(version, ' ') >= 0 {
		return errMalformedRequestLine
	}
	if !bytes.Equal(version, []byte("HTTP/1.0")) && !bytes.Equal(version, []byte("HTTP/1.1")) {
		return errUnsupportedHTTPVersion
	}
	return nil
}

// NormalizeHTTPHost normalizes an HTTP Host header value: trims spaces,
// lowercases, strips an optional :port, and removes a single trailing dot.
func NormalizeHTTPHost(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return ""
	}
	// Strip an optional port. SplitHostPort only succeeds when a port-like
	// suffix is present; leave bare hosts (and unbracketed IPv6) untouched.
	if stripped, _, err := net.SplitHostPort(h); err == nil {
		h = stripped
	}
	h = strings.TrimSuffix(h, ".")
	return h
}
