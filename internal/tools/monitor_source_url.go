package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// urlSourceMinInterval floors how often a monitor may re-fetch a URL. The
	// manager ticks every second; polling a remote host that fast is abusive, so
	// the source gates itself independently of the monitor cooldown (which bounds
	// wakes, not requests).
	urlSourceMinInterval = 30 * time.Second
	// urlSourceMaxBody bounds the downloaded body. Only a change signal is needed,
	// not the whole document, and the payload handed to the agent is capped far
	// lower again.
	urlSourceMaxBody = 256 * 1024
	// urlSourceMaxFailures is how many consecutive failed fetches are tolerated
	// before the monitor is declared terminal. A single blip must not kill a
	// long-running watch, but an endpoint that is simply gone must not be polled
	// for the rest of the session either.
	urlSourceMaxFailures = 5
)

// urlSource fetches a URL periodically and reports an event whenever the response
// body CHANGES. The event payload is the new body (capped), so a regex filter can
// match on the content the way it matches a log line.
//
// Egress goes through the same SSRF-guarded client the WebFetch tool uses
// (newGuardedHTTPClient): the resolved IP of every dial is checked, so loopback,
// private, link-local and cloud-metadata addresses are refused even via DNS
// rebinding or a redirect hop. A monitor is a long-lived, unattended fetch loop,
// which makes that boundary more important here than in a one-shot fetch, not
// less.
type urlSource struct {
	client   *http.Client
	rawURL   string
	interval time.Duration

	// next is when the source is allowed to fetch again. Polls before it are
	// no-ops, which is how the 1s manager tick is decoupled from the fetch rate.
	next time.Time
	// digest is the hash of the last body seen. The first fetch establishes the
	// baseline and does NOT fire: a monitor reports what CHANGES from now on.
	digest string
	// haveBaseline distinguishes "no body seen yet" from "last body was empty".
	haveBaseline bool
	// failures counts consecutive fetch failures, reset by any success.
	failures int
}

// NewURLSource binds a monitor source to rawURL, polled every interval (floored
// at urlSourceMinInterval). A non-http(s) or unparseable URL is rejected up
// front rather than failing forever in the background.
func NewURLSource(rawURL string, interval time.Duration) (MonitorSource, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("url is required to monitor a URL")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url %q: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("invalid url %q: only http and https can be monitored", rawURL)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid url %q: no host", rawURL)
	}
	if interval < urlSourceMinInterval {
		interval = urlSourceMinInterval
	}
	return &urlSource{
		client:   newGuardedHTTPClient(30 * time.Second),
		rawURL:   rawURL,
		interval: interval,
		// Fetch once immediately to establish the baseline.
		next: time.Now(),
	}, nil
}

// Poll fetches the URL when the interval has elapsed and reports an event when
// the body differs from the previous fetch.
func (s *urlSource) Poll(ctx context.Context) ([]MonitorEvent, bool, string, error) {
	if time.Now().Before(s.next) {
		return nil, false, "", nil
	}
	s.next = time.Now().Add(s.interval)

	body, err := s.fetch(ctx)
	if err != nil {
		s.failures++
		if s.failures >= urlSourceMaxFailures {
			return nil, true, fmt.Sprintf("%s unreachable after %d consecutive attempts: %v", s.rawURL, s.failures, err), nil
		}
		return nil, false, "", err
	}
	s.failures = 0

	sum := sha256.Sum256([]byte(body))
	digest := hex.EncodeToString(sum[:])
	if !s.haveBaseline {
		s.haveBaseline = true
		s.digest = digest
		return nil, false, "", nil
	}
	if digest == s.digest {
		return nil, false, "", nil
	}
	s.digest = digest

	payload := strings.TrimSpace(body)
	if len(payload) > monitorPayloadBytes {
		payload = payload[:monitorPayloadBytes] + "…[truncated]"
	}
	return []MonitorEvent{{At: time.Now(), Payload: payload}}, false, "", nil
}

// fetch performs one bounded GET and returns the body as text.
func (s *urlSource) fetch(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "tionharness/0.0.1")
	req.Header.Set("Accept", "text/plain,text/html,application/json;q=0.9,*/*;q=0.5")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	// A status change is itself a change worth reporting, so the status line is
	// part of the hashed payload rather than an error on non-2xx.
	b, err := io.ReadAll(io.LimitReader(resp.Body, urlSourceMaxBody))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(b))), nil
}

// Describe identifies the source in the monitor list.
func (s *urlSource) Describe() string { return "url " + s.rawURL }

// Close releases the source's idle connections.
func (s *urlSource) Close() { s.client.CloseIdleConnections() }
