package tools

import (
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// newGuardedHTTPClient builds an HTTP client whose dialer refuses connections to
// loopback, private, link-local and other non-public addresses (SSRF guard). The
// check runs on the *resolved* IP for every dial, so DNS-rebinding and redirect
// hops to internal hosts are blocked too.
//
// This is the single egress boundary shared by every outbound tool: the one-shot
// WebFetch tool and the long-lived monitor URL/WebSocket sources. Keeping it in
// one place is the point — a second, subtly different dialer is how an SSRF hole
// gets introduced later.
func newGuardedHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: newGuardedTransport()}
}

// newGuardedTransport builds the guarded transport on its own, for callers that
// need the transport rather than a client (the WebSocket dialer).
func newGuardedTransport() *http.Transport {
	return &http.Transport{
		DialContext:           newGuardedDialer().DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

// newGuardedDialer builds the dialer that enforces the public-address rule.
func newGuardedDialer() *net.Dialer {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	dialer.Control = func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if ip == nil || isBlockedIP(ip) {
			return fmt.Errorf("blocked address %q (private/loopback/link-local not allowed)", host)
		}
		return nil
	}
	return dialer
}

// isBlockedIP reports whether ip must not be reached by an outbound tool:
// loopback, unspecified, link-local (incl. the 169.254.169.254 cloud-metadata
// endpoint), private RFC1918 / unique-local ranges, and carrier-grade NAT.
func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsMulticast() {
		return true
	}
	// 100.64.0.0/10 (RFC 6598, carrier-grade NAT) is not covered by IsPrivate.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64 {
		return true
	}
	return false
}
