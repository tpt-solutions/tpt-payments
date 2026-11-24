package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// ParseTrustedProxies parses a comma-separated list of IP addresses and CIDR ranges, such as
// "10.0.0.0/8, 203.0.113.7, ::1". An empty string means no proxies are trusted.
func ParseTrustedProxies(list string) ([]*net.IPNet, error) {
	var nets []*net.IPNet
	for _, raw := range strings.Split(list, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			_, n, err := net.ParseCIDR(entry)
			if err != nil {
				return nil, fmt.Errorf("invalid trusted proxy range %q: %w", entry, err)
			}
			nets = append(nets, n)
			continue
		}
		ip := net.ParseIP(entry)
		if ip == nil {
			return nil, fmt.Errorf("invalid trusted proxy address %q", entry)
		}
		bits := 8 * net.IPv4len
		if ip.To4() == nil {
			bits = 8 * net.IPv6len
		}
		nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	return nets, nil
}

// TrustedProxyRealIP replaces r.RemoteAddr with the client address from X-Forwarded-For, but only
// when the connection comes from one of the trusted proxies. Every other request keeps the
// connection's address, so a client cannot choose its own IP by sending the header.
//
// The header is read right to left: each hop that is itself a trusted proxy is skipped, and the
// first untrusted address is the client. If any entry is not an IP address, the header is ignored
// and the proxy's address stays in place. With no trusted proxies the middleware does nothing.
//
// Rate limiting and the audit log read r.RemoteAddr, so they follow the rewritten address.
func TrustedProxyRealIP(trusted []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if len(trusted) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if client, ok := forwardedClient(r, trusted); ok {
				r.RemoteAddr = client
			}
			next.ServeHTTP(w, r)
		})
	}
}

// forwardedClient returns the client address to use in place of r.RemoteAddr, keeping the port
// the connection used. It reports false when the request should keep its connection address.
func forwardedClient(r *http.Request, trusted []*net.IPNet) (string, bool) {
	peerHost, port, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peerHost, port = r.RemoteAddr, ""
	}
	if !containsIP(trusted, net.ParseIP(peerHost)) {
		return "", false
	}

	var hops []string
	for _, value := range r.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(value, ",") {
			if p := strings.TrimSpace(part); p != "" {
				hops = append(hops, p)
			}
		}
	}
	if len(hops) == 0 {
		return "", false
	}

	var client net.IP
	for i := len(hops) - 1; i >= 0; i-- {
		ip := net.ParseIP(hops[i])
		if ip == nil {
			return "", false
		}
		client = ip
		if !containsIP(trusted, ip) {
			break
		}
	}
	return net.JoinHostPort(client.String(), port), true
}

func containsIP(nets []*net.IPNet, ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
