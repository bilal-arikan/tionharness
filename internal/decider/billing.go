package decider

import (
	"net"
	"net/url"
	"strings"
)

// Billing. Every decision call lands in the usage table under a price-table
// provider and model id, so its cost shows up next to chat spend. The provider
// follows from where the call went, not from the backend: the same System One
// request costs money at TypeSafe or OpenRouter and nothing on the user's own
// GPU.
const (
	// billingLocal is a server on the user's own machine or network. The price
	// table treats it as a known zero, like LM Studio.
	billingLocal      = "local"
	billingOpenRouter = "openrouter"
	billingTypeSafe   = "typesafe"

	openRouterHost = "openrouter.ai"
	typeSafeHost   = "api.typesafe.ai"
)

// billingProviderFor names the price-table provider of a call to rawURL made
// with credentials borrowed from a provider instance of kind ("" = the model's
// own credentials). An unknown host bills under the borrowed kind, or under
// nothing (unpriced) with own credentials.
func billingProviderFor(rawURL, kind string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return kind
	}
	host := strings.ToLower(u.Hostname())
	switch {
	case isLocalHost(host):
		return billingLocal
	case host == openRouterHost:
		return billingOpenRouter
	case host == typeSafeHost:
		return billingTypeSafe
	}
	return kind
}

// isLocalHost reports a loopback, private-network, link-local or mDNS host:
// somewhere the tokens are processed on hardware the user already owns.
func isLocalHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

// hostOf returns rawURL's host for messages, or the raw string when it does not
// parse.
func hostOf(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		return u.Host
	}
	return rawURL
}
