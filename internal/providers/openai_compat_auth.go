package providers

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// checkAuth reports whether this client has the credential its endpoint needs.
// A hosted OpenAI-compatible endpoint always needs a bearer key, so an empty
// key is a configuration error caught before the request is built. A local
// endpoint (LM Studio, Ollama, llama.cpp, vLLM on loopback) authenticates
// nothing, so an empty key is the normal case there and must not fail.
func (m *OpenAICompat) checkAuth() error {
	if m.apiKey != "" || m.localEndpoint() {
		return nil
	}
	return fmt.Errorf("%s: missing API key", m.name)
}

// authHeaders returns the request headers carrying this client's credential.
// With no key (a local endpoint) it returns no Authorization header at all
// rather than an empty "Bearer " value: some local servers reject a malformed
// Authorization header outright instead of ignoring it.
func (m *OpenAICompat) authHeaders() map[string]string {
	if m.apiKey == "" {
		return map[string]string{}
	}
	return map[string]string{"Authorization": "Bearer " + m.apiKey}
}

// localEndpoint reports whether this client's base URL points at a model server
// running on the user's own machine or private network — the case where there
// is no credential to supply. It is decided from the host, not from the kind
// name, so a "lmstudio" instance pointed at a remote authenticated proxy is
// still treated as remote (and keeps requiring a key), while a generic
// "openai-compat" instance pointed at localhost works without one.
func (m *OpenAICompat) localEndpoint() bool {
	u, err := url.Parse(m.baseURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "" {
		// A base URL with no scheme parses with an empty Hostname (the whole value
		// lands in Path), so fall back to matching the raw prefix.
		host = strings.SplitN(strings.TrimPrefix(m.baseURL, "//"), "/", 2)[0]
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
	}
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
