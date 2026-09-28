package db

import "context"

type MessagePage struct {
	Items        []Message         `json:"items"`
	Offset       int               `json:"offset"`
	Total        int               `json:"total"`
	HasMore      bool              `json:"hasMore"`
	HasNewer     bool              `json:"hasNewer"`
	Truncated    bool              `json:"truncated"`
	Summary      TranscriptSummary `json:"summary"`
	Participants []string          `json:"participants"`
}

// ListMessagePage uses message identities as cursors so inserts/deletes before
// the window cannot shift it silently. Empty cursors select the newest page.
func (d *DB) ListMessagePage(ctx context.Context, sid string, limit int, before, after, around, start string) (MessagePage, error) {
	tl := d.transcriptLock(sid)
	tl.Lock()
	defer tl.Unlock()
	release, err := d.pinTranscript(sid)
	if err != nil {
		return MessagePage{}, err
	}
	defer release(false)
	d.mu.RLock()
	defer d.mu.RUnlock()
	msgs := d.messages[sid]
	limit = max(1, min(limit, 200))
	lo, hi := max(0, len(msgs)-limit), len(msgs)
	cursor := before
	if after != "" {
		cursor = after
	}
	if around != "" {
		cursor = around
	}
	if start != "" {
		cursor = start
	}
	if cursor != "" {
		idx := -1
		for i := range msgs {
			if msgs[i].ID == cursor {
				idx = i
				break
			}
		}
		if idx < 0 {
			return MessagePage{}, ErrMessageNotFound
		}
		switch {
		case before != "":
			hi = idx
			lo = max(0, hi-limit)
		case after != "":
			lo = idx + 1
			hi = min(len(msgs), lo+limit)
		case start != "":
			lo = idx
			hi = min(len(msgs), lo+limit)
		default:
			lo = max(0, idx-limit/2)
			hi = min(len(msgs), lo+limit)
		}
	}
	items := make([]Message, hi-lo)
	copy(items, msgs[lo:hi])
	return MessagePage{Items: items, Offset: lo, Total: len(msgs), HasMore: lo > 0, HasNewer: hi < len(msgs), Truncated: lo > 0 || hi < len(msgs), Summary: d.transcriptCache[sid].summary, Participants: append([]string(nil), d.sessions[sid].Participants...)}, nil
}
