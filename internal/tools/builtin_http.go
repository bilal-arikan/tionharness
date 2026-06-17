package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"

	"github.com/bilal/swarmgo/internal/providers"
)

const httpGetMaxBytes = 64 * 1024 // cap the body fed back to the model

// HTTPGetTool fetches a URL with GET and returns the (truncated) body. It is a
// read-only network tool; no headers/auth are supported by design.
type HTTPGetTool struct {
	client *http.Client
}

// NewHTTPGetTool builds the tool with a bounded-timeout client whose dialer
// refuses connections to loopback, private, link-local and other non-public
// addresses (SSRF guard). The check runs on the *resolved* IP for every dial,
// so DNS-rebinding and redirect hops to internal hosts are blocked too.
func NewHTTPGetTool() HTTPGetTool {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	dialer.Control = func(network, address string, _ syscall.RawConn) error {
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
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	return HTTPGetTool{client: &http.Client{Timeout: 20 * time.Second, Transport: transport}}
}

// isBlockedIP reports whether ip must not be reached by the http_get tool:
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

func (HTTPGetTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "http_get",
		Description: "Fetch a URL over HTTP GET and return the response body (truncated to 64KB). Read-only.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"url":{"type":"string","description":"Absolute http(s) URL to fetch"}},
			"required":["url"],
			"additionalProperties":false
		}`),
	}
}

func (t HTTPGetTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.URL == "" {
		return "", fmt.Errorf("url is required")
	}
	if u, err := url.Parse(args.URL); err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	} else if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported url scheme %q (only http/https)", u.Scheme)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, args.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "swarmgo/0.0.1")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, httpGetMaxBytes))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("HTTP %d\n\n%s", resp.StatusCode, string(body)), nil
}
