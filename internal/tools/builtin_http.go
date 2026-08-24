package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

const (
	// webFetchMaxFetchBytes caps the raw body we download before conversion, so a
	// huge page can't exhaust memory; the converted/returned text is capped lower.
	webFetchMaxFetchBytes = 3 * 1024 * 1024
	// webFetchMaxOutBytes caps the text/markdown fed back to the model (context).
	webFetchMaxOutBytes = 96 * 1024
)

// WebFetchTool fetches a web page and returns its readable content as Markdown
// (HTML is stripped of scripts/styles/nav and converted; non-HTML text is
// returned as-is). It is the native counterpart of the claude-cli WebFetch tool
// and a rich upgrade of the former plain-GET http_get. Read-only; public hosts
// only (SSRF-guarded dialer).
type WebFetchTool struct {
	client *http.Client
}

// NewWebFetchTool builds the tool with a bounded-timeout client whose dialer
// refuses connections to loopback, private, link-local and other non-public
// addresses (SSRF guard). The check runs on the *resolved* IP for every dial,
// so DNS-rebinding and redirect hops to internal hosts are blocked too.
func NewWebFetchTool() WebFetchTool {
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
	return WebFetchTool{client: &http.Client{Timeout: 30 * time.Second, Transport: transport}}
}

// isBlockedIP reports whether ip must not be reached by the WebFetch tool:
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

func (WebFetchTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "WebFetch",
		Description: "Fetch a web page over HTTP(S) and return its readable content as Markdown: " +
			"HTML is stripped of scripts/styles/nav/forms and converted to headings, links, lists and " +
			"text; already-textual responses (markdown/plain/JSON) are returned as-is; binary responses " +
			"are summarised, not dumped. Read-only, public hosts only. Set raw=true to get the unprocessed body.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"url":{"type":"string","description":"Absolute http(s) URL to fetch"},
				"raw":{"type":"boolean","description":"Return the unprocessed response body instead of converted Markdown (default false)"}
			},
			"required":["url"],
			"additionalProperties":false
		}`),
	}
}

func (t WebFetchTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		URL string `json:"url"`
		Raw bool   `json:"raw"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
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
	req.Header.Set("User-Agent", "tionharness/0.0.1")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/markdown,text/plain,application/json;q=0.9,*/*;q=0.5")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, webFetchMaxFetchBytes))
	if err != nil {
		return "", err
	}

	// The final URL after any redirects — used both for the header line and to
	// resolve relative links when converting HTML.
	finalURL := args.URL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	ctype := strings.ToLower(resp.Header.Get("Content-Type"))

	var body string
	switch {
	case args.Raw:
		body = string(raw)
	case isHTMLContent(ctype, raw):
		var base *url.URL
		if u, e := url.Parse(finalURL); e == nil {
			base = u
		}
		body = htmlToMarkdown(string(raw), base)
	case isTextualContent(ctype, raw):
		body = string(raw)
	default:
		return fmt.Sprintf("HTTP %d %s\nURL: %s\n\n[%d bytes of non-text content (%s) — not shown; set raw=true to fetch bytes as text]",
			resp.StatusCode, http.StatusText(resp.StatusCode), finalURL, len(raw), firstNonEmpty(ctype, "unknown type")), nil
	}

	body = strings.TrimSpace(body)
	truncated := ""
	if len(body) > webFetchMaxOutBytes {
		body = body[:webFetchMaxOutBytes]
		truncated = fmt.Sprintf("\n\n[truncated at %dKB]", webFetchMaxOutBytes/1024)
	}
	return fmt.Sprintf("HTTP %d %s\nURL: %s\n\n%s%s",
		resp.StatusCode, http.StatusText(resp.StatusCode), finalURL, body, truncated), nil
}

// isHTMLContent decides whether to run the Markdown converter: trust an explicit
// HTML content-type, else sniff the first bytes for an HTML signature (servers
// that send text/plain or no type for HTML still convert).
func isHTMLContent(ctype string, body []byte) bool {
	if strings.Contains(ctype, "text/html") || strings.Contains(ctype, "application/xhtml") {
		return true
	}
	if ctype != "" && !strings.Contains(ctype, "text/plain") {
		return false
	}
	head := strings.ToLower(strings.TrimSpace(string(body[:min(len(body), 1024)])))
	return strings.HasPrefix(head, "<!doctype html") || strings.Contains(head, "<html") || strings.Contains(head, "<body")
}

// isTextualContent reports whether the body can be returned verbatim as text
// (markdown/plain/json/xml/csv/source), as opposed to binary.
func isTextualContent(ctype string, body []byte) bool {
	for _, t := range []string{"text/", "application/json", "application/xml", "application/javascript", "application/x-ndjson", "+json", "+xml"} {
		if strings.Contains(ctype, t) {
			return true
		}
	}
	if ctype == "" {
		return !isBinary(body) // reuse builtin_fs.go's NUL-byte heuristic
	}
	return false
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
