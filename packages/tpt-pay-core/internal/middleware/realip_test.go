package middleware

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mustParse(t *testing.T, list string) []*net.IPNet {
	t.Helper()
	nets, err := ParseTrustedProxies(list)
	if err != nil {
		t.Fatalf("ParseTrustedProxies(%q): %v", list, err)
	}
	return nets
}

// remoteAddrSeenBy runs the middleware over a request and reports the RemoteAddr the next handler saw.
func remoteAddrSeenBy(t *testing.T, trusted []*net.IPNet, remote string, xff ...string) string {
	t.Helper()
	var seen string
	h := TrustedProxyRealIP(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.RemoteAddr
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/payment_intents", nil)
	req.RemoteAddr = remote
	for _, v := range xff {
		req.Header.Add("X-Forwarded-For", v)
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	return seen
}

func TestTrustedProxyRealIP_NoProxiesConfiguredIgnoresTheHeader(t *testing.T) {
	got := remoteAddrSeenBy(t, nil, "198.51.100.9:4000", "203.0.113.5")
	if got != "198.51.100.9:4000" {
		t.Fatalf("RemoteAddr = %q, want the connection address", got)
	}
}

func TestTrustedProxyRealIP_UntrustedPeerCannotSpoofItsAddress(t *testing.T) {
	trusted := mustParse(t, "10.0.0.0/8")
	got := remoteAddrSeenBy(t, trusted, "198.51.100.9:4000", "203.0.113.5")
	if got != "198.51.100.9:4000" {
		t.Fatalf("an untrusted peer's X-Forwarded-For was used: RemoteAddr = %q", got)
	}
}

func TestTrustedProxyRealIP_TrustedProxyYieldsTheClient(t *testing.T) {
	trusted := mustParse(t, "10.0.0.0/8")
	got := remoteAddrSeenBy(t, trusted, "10.1.2.3:5555", "203.0.113.5")
	if got != "203.0.113.5:5555" {
		t.Fatalf("RemoteAddr = %q, want the forwarded client with the connection port", got)
	}
}

func TestTrustedProxyRealIP_WalksPastTrustedHopsFromTheRight(t *testing.T) {
	trusted := mustParse(t, "10.0.0.0/8, 192.168.0.1")
	// client -> 203.0.113.5 -> 10.9.9.9 (proxy) -> 192.168.0.1 (proxy) -> our proxy at 10.1.2.3
	got := remoteAddrSeenBy(t, trusted, "10.1.2.3:443", "203.0.113.5, 10.9.9.9", "192.168.0.1")
	if got != "203.0.113.5:443" {
		t.Fatalf("RemoteAddr = %q, want the first untrusted hop from the right", got)
	}
}

func TestTrustedProxyRealIP_ForgedLeftmostEntryIsNotChosen(t *testing.T) {
	trusted := mustParse(t, "10.0.0.0/8")
	// A client sends its own X-Forwarded-For; the proxy appends the real peer on the right.
	got := remoteAddrSeenBy(t, trusted, "10.1.2.3:443", "1.1.1.1", "198.51.100.77")
	if got != "198.51.100.77:443" {
		t.Fatalf("RemoteAddr = %q, want the address the proxy appended, not the forged 1.1.1.1", got)
	}
}

func TestTrustedProxyRealIP_MalformedHopOnTheRightKeepsTheProxyAddress(t *testing.T) {
	trusted := mustParse(t, "10.0.0.0/8")
	got := remoteAddrSeenBy(t, trusted, "10.1.2.3:443", "203.0.113.5, not-an-ip")
	if got != "10.1.2.3:443" {
		t.Fatalf("RemoteAddr = %q, want the proxy address when the rightmost hop is malformed", got)
	}
}

func TestTrustedProxyRealIP_MalformedEntryBeyondTheClientIsIgnored(t *testing.T) {
	trusted := mustParse(t, "10.0.0.0/8")
	// The leftmost entry is whatever the client sent; the walk stops at the real client.
	got := remoteAddrSeenBy(t, trusted, "10.1.2.3:443", "not-an-ip, 203.0.113.5")
	if got != "203.0.113.5:443" {
		t.Fatalf("RemoteAddr = %q, want the appended client address", got)
	}
}

func TestTrustedProxyRealIP_IPv6Client(t *testing.T) {
	trusted := mustParse(t, "::1")
	got := remoteAddrSeenBy(t, trusted, "[::1]:8080", "2001:db8::42")
	if got != "[2001:db8::42]:8080" {
		t.Fatalf("RemoteAddr = %q", got)
	}
}

func TestTrustedProxyRealIP_RateLimitKeyFollowsTheClient(t *testing.T) {
	trusted := mustParse(t, "10.0.0.0/8")
	var key string
	h := TrustedProxyRealIP(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key = rateLimitKey(r)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:1000"
	req.Header.Set("X-Forwarded-For", "203.0.113.5")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if key != "ip:203.0.113.5" {
		t.Fatalf("rate limit key = %q, want the forwarded client", key)
	}
}

func TestParseTrustedProxies(t *testing.T) {
	if nets, err := ParseTrustedProxies("  "); err != nil || len(nets) != 0 {
		t.Fatalf("blank list: nets=%v err=%v", nets, err)
	}
	for _, bad := range []string{"10.0.0.0/33", "203.0.113", "10.0.0.1/", "example.com"} {
		if _, err := ParseTrustedProxies(bad); err == nil {
			t.Errorf("ParseTrustedProxies(%q) accepted an invalid entry", bad)
		}
	}
	nets := mustParse(t, "203.0.113.7, 10.0.0.0/8")
	if !containsIP(nets, net.ParseIP("203.0.113.7")) || containsIP(nets, net.ParseIP("203.0.113.8")) {
		t.Fatal("single address entry must match only that address")
	}
	if !containsIP(nets, net.ParseIP("10.200.0.1")) {
		t.Fatal("CIDR entry must match addresses in range")
	}
}
