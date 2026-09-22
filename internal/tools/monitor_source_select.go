package tools

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// monitorSourceArgs is the source half of the monitor tool's start arguments:
// exactly one of ShellID/Path/URL selects the backend.
type monitorSourceArgs struct {
	ShellID  string
	Path     string
	URL      string
	Interval int // seconds, http(s) polling rate only
}

// buildSource turns the start arguments into a MonitorSource.
//
// Exactly one source must be named. Naming none leaves nothing to watch, and
// naming several is ambiguous in a way that would silently watch the wrong
// thing — both are rejected with an actionable message rather than guessed at.
func (t MonitorTool) buildSource(ctx context.Context, args monitorSourceArgs) (MonitorSource, error) {
	shellID := strings.TrimSpace(args.ShellID)
	path := strings.TrimSpace(args.Path)
	rawURL := strings.TrimSpace(args.URL)

	var named []string
	if shellID != "" {
		named = append(named, "shell_id")
	}
	if path != "" {
		named = append(named, "path")
	}
	if rawURL != "" {
		named = append(named, "url")
	}
	switch len(named) {
	case 0:
		return nil, fmt.Errorf("action \"start\" needs a source: pass shell_id (a background shell), path (a file) or url (http/https/ws/wss)")
	case 1:
	default:
		return nil, fmt.Errorf("action \"start\" takes exactly one source, but %s were given — arm one monitor per source", strings.Join(named, " and "))
	}

	switch {
	case shellID != "":
		return NewShellSource(t.shell, shellID)
	case path != "":
		return NewFileSource(t.sb, path)
	default:
		return newURLishSource(ctx, rawURL, args.Interval)
	}
}

// newURLishSource routes a url argument to the polling or the socket source by
// its scheme, so the agent passes one `url` field instead of choosing a backend.
func newURLishSource(ctx context.Context, rawURL string, interval int) (MonitorSource, error) {
	lower := strings.ToLower(rawURL)
	switch {
	case strings.HasPrefix(lower, "ws://"), strings.HasPrefix(lower, "wss://"):
		return NewWSSource(ctx, rawURL)
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		return NewURLSource(rawURL, time.Duration(interval)*time.Second)
	default:
		return nil, fmt.Errorf("invalid url %q: expected an http://, https://, ws:// or wss:// url", rawURL)
	}
}
