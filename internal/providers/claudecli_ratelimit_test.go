package providers

import "testing"

// TestCLIParserRateLimitStatus: the CLI emits a "rate_limit_event" on every turn
// carrying the subscription window state. The "allowed" family (plain "allowed"
// and "allowed_warning", the latter just signalling the window is filling up)
// must NOT flag the turn as rate-limited — only a status outside that family
// (e.g. "rejected"/"blocked") is a real refusal. Regression guard: matching the
// exact string "allowed" mis-classified "allowed_warning" as a hard usage limit,
// reported a bogus "usage/rate limit reached" and masked the real exit cause.
func TestCLIParserRateLimitStatus(t *testing.T) {
	cases := []struct {
		name        string
		status      string
		wantLimited bool
	}{
		{"allowed", "allowed", false},
		{"allowed_warning", "allowed_warning", false},
		{"allowed_warning mixed case", "Allowed_Warning", false},
		{"rejected", "rejected", true},
		{"blocked", "blocked", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newCLIParser("claude-opus-4-8", nil)
			p.feed(`{"type":"rate_limit_event","rate_limit_info":{"status":"` + tc.status +
				`","rateLimitType":"seven_day","utilization":0.64,"isUsingOverage":false}}`)
			if p.rateLimited != tc.wantLimited {
				t.Fatalf("status %q: rateLimited = %v, want %v", tc.status, p.rateLimited, tc.wantLimited)
			}
		})
	}
}

// TestCLIParserRateLimitEmptyStatus: an absent/empty status must never flag a
// rate limit (the event fires on normal turns too, sometimes without a status).
func TestCLIParserRateLimitEmptyStatus(t *testing.T) {
	p := newCLIParser("claude-opus-4-8", nil)
	p.feed(`{"type":"rate_limit_event","rate_limit_info":{"status":"","rateLimitType":"seven_day"}}`)
	if p.rateLimited {
		t.Fatalf("empty status flagged rateLimited")
	}
}
