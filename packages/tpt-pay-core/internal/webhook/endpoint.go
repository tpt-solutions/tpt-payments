package webhook

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// maxEndpointURL bounds stored URLs.
const maxEndpointURL = 2048

// ValidateEndpointURL checks a webhook destination before it is stored.
//
// Live endpoints must use https and must not point at loopback, private, link-local or
// unspecified addresses. Test endpoints may also use http and loopback targets so that
// local receivers work. Link-local targets (including cloud metadata addresses such as
// 169.254.169.254) are refused in both modes.
//
// This check applies to the literal host only. Hostnames are resolved again at delivery
// time, where the same address rules must be enforced (tracked with the delivery engine).
func ValidateEndpointURL(raw string, live bool) error {
	if raw == "" {
		return errors.New("url is required")
	}
	if len(raw) > maxEndpointURL {
		return fmt.Errorf("url must be at most %d characters", maxEndpointURL)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("url is not a valid URL")
	}
	if u.User != nil {
		return errors.New("url must not contain credentials")
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("url must include a host")
	}

	loopback := isLoopbackHost(host)
	switch u.Scheme {
	case "https":
	case "http":
		if live || !loopback {
			return errors.New("url must use https (http is only allowed for localhost in test mode)")
		}
	default:
		return errors.New("url must use https")
	}

	if ip := net.ParseIP(host); ip != nil {
		if blockedIP(ip, live) {
			return errors.New("url must not point to a private, loopback or link-local address")
		}
	} else if live && loopback {
		return errors.New("url must not point to localhost in live mode")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// blockedIP reports whether a literal address is refused. Link-local, multicast and
// unspecified addresses are always refused. Live mode also refuses loopback and private
// ranges.
func blockedIP(ip net.IP, live bool) bool {
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if live && (ip.IsLoopback() || ip.IsPrivate()) {
		return true
	}
	return false
}

// ValidateEvents checks enabled_events. "*" selects every event; otherwise each entry
// must be a known event type.
func ValidateEvents(events []string) error {
	if len(events) == 0 {
		return errors.New("enabled_events must not be empty")
	}
	known := map[string]bool{"*": true}
	for _, e := range KnownEventTypes() {
		known[string(e)] = true
	}
	for _, e := range events {
		if !known[e] {
			return fmt.Errorf("unknown event type %q", e)
		}
	}
	return nil
}

// KnownEventTypes lists every event type the platform emits.
func KnownEventTypes() []EventType {
	return []EventType{
		EventPaymentIntentCreated,
		EventPaymentIntentSucceeded,
		EventPaymentIntentCanceled,
		EventPaymentIntentFailed,
		EventPaymentIntentFraud,
		EventChargeRefunded,
	}
}
