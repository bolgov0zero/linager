package server

import (
	"net"
	"net/http"
	"net/netip"
)

// remoteAddr is the address of the connection. The application port is a direct TLS connection by design: nothing stands in
// front of it, the forwarding headers are dropped before the handler (stripForwarded), so this is the address of the client.
func remoteAddr(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, _ := netip.ParseAddr(host)
	return a
}

// clientIP is remoteAddr as the text the audit, the sessions and the ban lists keep.
func clientIP(r *http.Request) string { return remoteAddr(r).Unmap().String() }
