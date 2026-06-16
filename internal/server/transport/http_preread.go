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
)

var httpHeaderTerminator = []byte("\r\n\r\n")

// ReadHTTPHost reads a cleartext HTTP/1.x request header from conn (without
// terminating TLS or consuming the body), extracts the Host header and returns
// it normalized. firstBytes contains every byte read from conn so the caller
// can replay them to the destination with a PrefixedConn. On error the caller
// should close the connection.
//
// It reads byte-by-byte up to the header terminator so the request body is left
// untouched on the wire. This only supports HTTP/1.x cleartext: TLS, HTTP/2 and
// h2c are rejected.
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

	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 1)
	for {
		if len(buf) >= maxHeaderBytes {
			return "", buf, errHTTPHeaderTooLarge
		}
		n, e := conn.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[0])
			if bytes.HasSuffix(buf, httpHeaderTerminator) {
				break
			}
		}
		if e != nil {
			return "", buf, e
		}
	}

	host, e := parseHTTPHostFromHeader(buf)
	if e != nil {
		return "", buf, e
	}
	return host, buf, nil
}

// parseHTTPHostFromHeader parses a full HTTP/1.x request header (ending in
// CRLFCRLF), validates the request line and returns the single normalized Host.
func parseHTTPHostFromHeader(b []byte) (string, error) {
	text := strings.TrimSuffix(string(b), "\r\n\r\n")
	lines := strings.Split(text, "\r\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", errMalformedRequestLine
	}
	if err := validateRequestLine(lines[0]); err != nil {
		return "", err
	}

	var hostVal string
	found := 0
	for _, line := range lines[1:] {
		if line == "" {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(line[:colon]), "host") {
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

	host := NormalizeHTTPHost(hostVal)
	if host == "" {
		return "", errEmptyHost
	}
	return host, nil
}

// validateRequestLine checks a request line of the form
// "METHOD SP REQUEST-TARGET SP HTTP/1.x". HTTP/2 prefaces (e.g. "PRI * HTTP/2.0")
// and any non-1.x version are rejected.
func validateRequestLine(line string) error {
	parts := strings.Split(line, " ")
	if len(parts) != 3 {
		return errMalformedRequestLine
	}
	method, target, version := parts[0], parts[1], parts[2]
	if method == "" || target == "" {
		return errMalformedRequestLine
	}
	if version != "HTTP/1.0" && version != "HTTP/1.1" {
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
