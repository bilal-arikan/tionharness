package db

import (
	"context"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/textutil"
)

// SearchHit is one message that matched a cross-session search, with enough
// context to render a result line and deep-link back to the message.
type SearchHit struct {
	SessionID    string  `json:"sessionId"`
	SessionTitle string  `json:"sessionTitle"`
	MessageID    string  `json:"messageId"`
	Role         string  `json:"role"`
	AgentID      string  `json:"agentId,omitempty"`
	Snippet      string  `json:"snippet"`
	Score        float64 `json:"score"`
	CreatedAt    int64   `json:"createdAt"`
}

// SearchOpts parameterizes SearchMessages. Query is split on whitespace into
// terms that are AND-matched (case-insensitive substring). Zero-value fields
// mean "no constraint" except Kinds, which defaults to chat sessions.
type SearchOpts struct {
	Query     string
	Kinds     []string // session kinds to include; empty = ["chat"]
	Roles     []string // message roles to include; empty = all
	ExcludeID string   // session id to skip (e.g. the current one)
	OnlyID    string   // restrict to this single session id; empty = all sessions
	SinceUnix int64    // only messages created at/after this unix time; 0 = no floor
	Limit     int      // max hits; <=0 = 20
}

// SearchMessages scans every loaded session's messages for ones containing all
// query terms. Because the store keeps all sessions in memory (loaded at boot),
// this is a pure in-RAM scan — no index, no disk I/O, no external tool. Results
// are ranked by match density and recency, newest/most-relevant first.
//
// The scan runs under the store's read lock, so it is kept allocation-free:
// terms are matched with a rune-folding substring search instead of lowering a
// copy of every message, and only the top `limit` candidates are retained (a
// bounded heap) so a broad query over a large workspace never materialises
// thousands of hits — nor cuts a snippet for any of them — before truncating.
func (d *DB) SearchMessages(ctx context.Context, o SearchOpts) ([]SearchHit, error) {
	terms := strings.Fields(textutil.FoldLower(o.Query))
	if len(terms) == 0 {
		return nil, nil
	}
	limit := o.Limit
	if limit <= 0 {
		limit = 20
	}
	kinds := o.Kinds
	if len(kinds) == 0 {
		kinds = []string{"chat"}
	}
	kindOK := map[string]bool{}
	for _, k := range kinds {
		kindOK[k] = true
	}
	var roleOK map[string]bool
	if len(o.Roles) > 0 {
		roleOK = map[string]bool{}
		for _, r := range o.Roles {
			roleOK[r] = true
		}
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	// newest session first so equal-score hits keep a stable, recent-leaning order
	sessions := make([]Session, 0, len(d.sessions))
	for _, s := range d.sessions {
		if !kindOK[s.Kind] || s.ID == o.ExcludeID {
			continue
		}
		if o.OnlyID != "" && s.ID != o.OnlyID {
			continue
		}
		sessions = append(sessions, s)
	}
	sort.SliceStable(sessions, func(i, j int) bool { return sessions[i].UpdatedAt > sessions[j].UpdatedAt })

	now := now()
	top := newSearchTopK(limit)
	seq := 0
	for si := range sessions {
		s := &sessions[si]
		msgs := d.messages[s.ID]
		for mi := range msgs {
			m := &msgs[mi]
			if roleOK != nil && !roleOK[m.Role] {
				continue
			}
			if o.SinceUnix > 0 && m.CreatedAt < o.SinceUnix {
				continue
			}
			matches, hadAll := countTerms(m.Text, terms)
			if !hadAll {
				continue
			}
			top.offer(searchCandidate{
				session:   s,
				message:   m,
				score:     score(matches, now-m.CreatedAt),
				createdAt: m.CreatedAt,
				seq:       seq,
			})
			seq++
		}
	}

	ranked := top.ranked()
	hits := make([]SearchHit, 0, len(ranked))
	for _, c := range ranked {
		hits = append(hits, SearchHit{
			SessionID:    c.session.ID,
			SessionTitle: strings.TrimSpace(c.session.Title),
			MessageID:    c.message.ID,
			Role:         c.message.Role,
			AgentID:      c.message.AgentID,
			Snippet:      makeSnippet(c.message.Text, terms[0]),
			Score:        c.score,
			CreatedAt:    c.createdAt,
		})
	}
	return hits, nil
}

// MessagesAround returns the message with id mid in session sid plus up to
// `before` preceding and `after` following messages, in chronological order.
// It powers conversation_search's verbatim-context mode: after a compaction has
// folded early turns into a summary, the agent can pull the EXACT earlier wording
// (e.g. the user's first question) back out of the raw transcript. Returns nil
// when the session or message is unknown.
func (d *DB) MessagesAround(ctx context.Context, sid, mid string, before, after int) []Message {
	d.mu.RLock()
	defer d.mu.RUnlock()
	msgs := d.messages[sid]
	idx := -1
	for i := range msgs {
		if msgs[i].ID == mid {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	lo := idx - before
	if lo < 0 {
		lo = 0
	}
	hi := idx + after + 1
	if hi > len(msgs) {
		hi = len(msgs)
	}
	out := make([]Message, hi-lo)
	copy(out, msgs[lo:hi])
	return out
}

// countTerms returns the total occurrence count of all terms and whether every
// term appears at least once (AND semantics). The terms must be folded with
// textutil.FoldLower; hay is folded on the fly without being copied.
func countTerms(hay string, terms []string) (total int, all bool) {
	for _, t := range terms {
		n := textutil.CountFold(hay, t)
		if n == 0 {
			return 0, false
		}
		total += n
	}
	return total, true
}

// score weighs match density against age: more matches rank higher, but recency
// gently lifts fresher messages so equal-density hits surface the recent one.
// Mirrors the relevance+recency philosophy of the memory recall roadmap (C5).
func score(matches int, ageSec int64) float64 {
	const day = 86400.0
	recency := 1.0 / (1.0 + float64(ageSec)/(7*day)) // ~half weight after a week
	return float64(matches) + recency
}

// makeSnippet returns a ~160-rune window of text centred on the first occurrence
// of term, cut on rune boundaries (UTF-8 safe — Turkish characters stay intact).
func makeSnippet(text, term string) string {
	const window = 160
	collapsed := strings.Join(strings.Fields(text), " ")
	runes := []rune(collapsed)
	if len(runes) <= window {
		return collapsed
	}
	idx := textutil.IndexFold(collapsed, term)
	if idx < 0 {
		return string(runes[:window]) + "…"
	}
	// rune offset of the byte index
	start := len([]rune(collapsed[:idx])) - window/2
	if start < 0 {
		start = 0
	}
	end := start + window
	if end > len(runes) {
		end = len(runes)
		start = end - window
	}
	out := string(runes[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(runes) {
		out += "…"
	}
	return out
}
