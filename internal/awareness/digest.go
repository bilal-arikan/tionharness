package awareness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/notes"
	"github.com/bilal-arikan/tionharness/internal/view"
)

// Digest is the LLM-free end-of-turn record of one session: what it touched,
// what it produced, what it left open. It is computed from the store after
// every completed turn (cheap, deterministic), persisted next to the transcript
// and indexed workspace-wide so the next session's brief can say "since you
// were last here, these sessions did this".
//
// Every number is counted in code. An optional narrative layer may be added
// later; it must never produce numbers.
type Digest struct {
	SessionID string `json:"sessionId"`
	AgentID   string `json:"agentId,omitempty"`
	AgentName string `json:"agentName,omitempty"`
	Title     string `json:"title"`
	Kind      string `json:"kind,omitempty"`
	State     string `json:"state,omitempty"`
	// At is when the digest was computed; StartedAt the session's creation.
	At          int64 `json:"at"`
	StartedAt   int64 `json:"startedAt,omitempty"`
	DurationSec int64 `json:"durationSec,omitempty"`

	Messages   int `json:"messages"`
	ToolCalls  int `json:"toolCalls"`
	ToolErrors int `json:"toolErrors"`
	StuckTurns int `json:"stuckTurns,omitempty"`
	// Tools is the per-tool histogram over the examined tail, most used first.
	Tools []ToolCount `json:"tools,omitempty"`
	// Truncated reports that only the transcript tail was examined.
	Truncated bool `json:"truncated,omitempty"`

	Artifacts []Ref `json:"artifacts,omitempty"`
	// Notes are the memory notes this session wrote.
	Notes []Ref    `json:"notes,omitempty"`
	Todo  TodoStat `json:"todo"`

	Children       int    `json:"children,omitempty"`
	ChildrenFailed int    `json:"childrenFailed,omitempty"`
	WaitingAsk     bool   `json:"waitingAsk,omitempty"`
	LastError      string `json:"lastError,omitempty"`
	// OpenLoops are the session-level loose ends: pending checklist items, a
	// waiting question, failed children.
	OpenLoops []string `json:"openLoops,omitempty"`
	// SuggestNote is set when the wrap-up-memory rule (or authority) thinks the
	// session produced something worth a durable note.
	SuggestNote bool `json:"suggestNote,omitempty"`

	// Hash fingerprints the facts (not At), so an unchanged digest is detected.
	Hash string `json:"hash"`
	// Text is the rendered digest, budgeted.
	Text string `json:"text"`
}

// ToolCount is one row of the tool histogram.
type ToolCount struct {
	Name   string `json:"name"`
	Calls  int    `json:"calls"`
	Errors int    `json:"errors,omitempty"`
}

// Ref names an entity by id and title.
type Ref struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
}

// TodoStat summarises the session checklist.
type TodoStat struct {
	Total      int      `json:"total"`
	Done       int      `json:"done"`
	InProgress int      `json:"inProgress"`
	Open       []string `json:"open,omitempty"`
}

// digestTail is how many trailing messages the tool histogram examines.
const digestTail = 200

// BuildDigest computes the digest for in.Session from the store.
func BuildDigest(ctx context.Context, in Input) (Digest, error) {
	s := in.Session
	if s.ID == "" {
		return Digest{}, errors.New("awareness: digest needs a session")
	}
	now := in.now()
	d := Digest{
		SessionID: s.ID, AgentID: s.AgentID, Title: strings.TrimSpace(s.Title), Kind: s.Kind, State: s.State,
		At: now.Unix(), StartedAt: s.CreatedAt, Messages: s.MessageCount, ToolCalls: s.ToolCallCount, StuckTurns: s.StuckTurns,
	}
	if n := strings.TrimSpace(in.Agent.Name); n != "" && in.Agent.ID == s.AgentID {
		d.AgentName = n
	}
	if s.CreatedAt > 0 {
		end := s.UpdatedAt
		if end < s.CreatedAt {
			end = now.Unix()
		}
		d.DurationSec = end - s.CreatedAt
	}
	msgs, from, err := in.Store.ListMessagesTail(ctx, s.ID, digestTail)
	if err != nil {
		return Digest{}, err
	}
	d.Truncated = from > 0
	counts := map[string]*ToolCount{}
	for _, m := range msgs {
		for _, st := range view.DecodeSteps(m.Steps) {
			switch st.Kind {
			case "tool":
				name := st.Tool
				if name == "" {
					name = "(unnamed)"
				}
				c := counts[name]
				if c == nil {
					c = &ToolCount{Name: name}
					counts[name] = c
				}
				c.Calls++
				if st.IsError {
					c.Errors++
					d.ToolErrors++
					d.LastError = clip(firstLine(st.Output, 200), 200)
				}
			case "error":
				if t := strings.TrimSpace(st.Text); t != "" {
					d.LastError = clip(firstLine(t, 200), 200)
				}
			}
		}
	}
	for _, c := range counts {
		d.Tools = append(d.Tools, *c)
	}
	sort.Slice(d.Tools, func(i, j int) bool {
		if d.Tools[i].Calls != d.Tools[j].Calls {
			return d.Tools[i].Calls > d.Tools[j].Calls
		}
		return d.Tools[i].Name < d.Tools[j].Name
	})
	if len(d.Tools) > 12 {
		d.Tools = d.Tools[:12]
	}
	roll := view.LatestTodos(msgs)
	d.Todo = TodoStat{Total: len(roll.Items), Done: roll.Done}
	for _, t := range roll.Items {
		switch t.Status {
		case "in_progress":
			d.Todo.InProgress++
			d.Todo.Open = append(d.Todo.Open, "~ "+clip(t.Content, 80))
		case "pending":
			d.Todo.Open = append(d.Todo.Open, "  "+clip(t.Content, 80))
		}
	}
	if arts, err := in.Store.ListArtifacts(ctx, s.ID); err == nil {
		for _, a := range arts {
			d.Artifacts = append(d.Artifacts, Ref{ID: a.ID, Title: clip(a.Title, 60)})
		}
	}
	if in.Notes != nil {
		for _, n := range in.Notes.List(notes.Filter{SourceSession: s.ID, IncludeRetired: true}) {
			d.Notes = append(d.Notes, Ref{ID: n.ID, Title: clip(n.Title, 60)})
		}
	}
	if sessions, err := in.Store.ListSessions(ctx, ""); err == nil {
		for _, c := range sessions {
			if c.CoordinatorSessionID == s.ID || c.ParentSessionID == s.ID {
				d.Children++
				if hasTag(c.Tags, "error") || c.StuckTurns > 0 {
					d.ChildrenFailed++
				}
			}
		}
	}
	if asks, err := in.Store.ListWaitingSessionAsks(ctx); err == nil {
		for _, a := range asks {
			if a.SessionID == s.ID {
				d.WaitingAsk = true
				break
			}
		}
	}
	if d.WaitingAsk {
		d.OpenLoops = append(d.OpenLoops, "a question to the user is still waiting for an answer")
	}
	if open := len(d.Todo.Open); open > 0 {
		d.OpenLoops = append(d.OpenLoops, fmt.Sprintf("%d checklist item(s) still open", open))
	}
	if d.ChildrenFailed > 0 {
		d.OpenLoops = append(d.OpenLoops, fmt.Sprintf("%d child session(s) failed or stuck", d.ChildrenFailed))
	}
	if d.StuckTurns > 0 {
		d.OpenLoops = append(d.OpenLoops, fmt.Sprintf("%d consecutive turn(s) ended badly", d.StuckTurns))
	}
	d.SuggestNote = SuggestNoteRule(d)
	if in.SuggestNote != nil {
		if s, ok := in.SuggestNote(ctx, d); ok {
			d.SuggestNote = s
		}
	}
	d.Hash = d.factsHash()
	d.Text = d.Render(in.Settings.DigestBudgetBytes)
	return d, nil
}

// SuggestNoteRule is the rule-based baseline for "did this session learn
// something worth keeping": it hit real tool errors, got stuck, or lost a
// child — and wrote no note about it yet.
func SuggestNoteRule(d Digest) bool {
	if len(d.Notes) > 0 {
		return false
	}
	return d.ToolErrors >= 2 || d.StuckTurns > 0 || d.ChildrenFailed > 0
}

func (d Digest) factsHash() string {
	cp := d
	cp.At, cp.Hash, cp.Text = 0, "", ""
	cp.DurationSec = 0
	b, _ := json.Marshal(cp)
	return HashText(string(b))
}

// Line is the one-line form used by the brief's recent-work list.
func (d Digest) Line() string {
	var parts []string
	title := d.Title
	if title == "" {
		title = "(untitled)"
	}
	if d.AgentName != "" {
		parts = append(parts, d.AgentName)
	}
	parts = append(parts, fmt.Sprintf("%q", clip(title, 50)))
	parts = append(parts, fmt.Sprintf("%d msg · %d tool calls", d.Messages, d.ToolCalls))
	if d.ToolErrors > 0 {
		parts = append(parts, fmt.Sprintf("%d tool errors", d.ToolErrors))
	}
	if len(d.Artifacts) > 0 {
		parts = append(parts, fmt.Sprintf("%d artifact(s)", len(d.Artifacts)))
	}
	if len(d.Notes) > 0 {
		parts = append(parts, fmt.Sprintf("%d note(s)", len(d.Notes)))
	}
	if d.Todo.Total > 0 {
		parts = append(parts, fmt.Sprintf("todo %d/%d", d.Todo.Done, d.Todo.Total))
	}
	if n := len(d.OpenLoops); n > 0 {
		parts = append(parts, fmt.Sprintf("%d open loop(s)", n))
	}
	return strings.Join(parts, " · ")
}

// Render formats the digest for the user and for a future brief, within budget.
func (d Digest) Render(budget int) string {
	var b strings.Builder
	title := d.Title
	if title == "" {
		title = "(untitled)"
	}
	fmt.Fprintf(&b, "# Session digest · %s · %q\n", d.SessionID, clip(title, 80))
	who := d.AgentName
	if who == "" {
		who = d.AgentID
	}
	fmt.Fprintf(&b, "agent: %s · kind: %s · state: %s · duration: %s · %d messages · %d tool calls",
		orDash(who), orDash(d.Kind), orDash(d.State), dur(d.DurationSec), d.Messages, d.ToolCalls)
	if d.ToolErrors > 0 {
		fmt.Fprintf(&b, " · %d tool errors", d.ToolErrors)
	}
	if d.StuckTurns > 0 {
		fmt.Fprintf(&b, " · stuck turns: %d", d.StuckTurns)
	}
	b.WriteString("\n")
	if len(d.Tools) > 0 {
		var parts []string
		for _, t := range d.Tools {
			if t.Errors > 0 {
				parts = append(parts, fmt.Sprintf("%s×%d(%d err)", t.Name, t.Calls, t.Errors))
			} else {
				parts = append(parts, fmt.Sprintf("%s×%d", t.Name, t.Calls))
			}
		}
		b.WriteString("tools: " + strings.Join(parts, " "))
		if d.Truncated {
			b.WriteString(" (tail only)")
		}
		b.WriteString("\n")
	}
	if d.Todo.Total > 0 {
		fmt.Fprintf(&b, "checklist: %d/%d done", d.Todo.Done, d.Todo.Total)
		if d.Todo.InProgress > 0 {
			fmt.Fprintf(&b, ", %d in progress", d.Todo.InProgress)
		}
		b.WriteString("\n")
		for _, o := range d.Todo.Open {
			b.WriteString("  " + o + "\n")
		}
	}
	if len(d.Artifacts) > 0 {
		var parts []string
		for _, a := range d.Artifacts {
			parts = append(parts, a.ID+" "+fmt.Sprintf("%q", a.Title))
		}
		b.WriteString("artifacts: " + joinCapped(parts, 8) + "\n")
	}
	if len(d.Notes) > 0 {
		var parts []string
		for _, n := range d.Notes {
			parts = append(parts, n.ID+" "+fmt.Sprintf("%q", n.Title))
		}
		b.WriteString("notes written: " + joinCapped(parts, 8) + "\n")
	}
	if d.Children > 0 {
		fmt.Fprintf(&b, "child sessions: %d", d.Children)
		if d.ChildrenFailed > 0 {
			fmt.Fprintf(&b, " (%d failed/stuck)", d.ChildrenFailed)
		}
		b.WriteString("\n")
	}
	if d.LastError != "" {
		b.WriteString("last error: " + d.LastError + "\n")
	}
	if len(d.OpenLoops) > 0 {
		b.WriteString("open loops:\n")
		for _, l := range d.OpenLoops {
			b.WriteString("- " + l + "\n")
		}
	}
	if d.SuggestNote {
		b.WriteString("suggestion: this session hit failures it recorded no note about; a `remember` with what was learned would spare the next session.\n")
	}
	text := strings.TrimRight(b.String(), "\n")
	if budget > 0 && len(text) > budget {
		cut := strings.LastIndex(text[:budget-40], "\n")
		if cut < budget/2 {
			cut = budget - 40
		}
		text = strings.TrimRight(text[:cut], "\n") + "\n[…digest cut to fit its budget]"
	}
	return text
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func dur(sec int64) string {
	switch {
	case sec <= 0:
		return "-"
	case sec < 60:
		return fmt.Sprintf("%ds", sec)
	case sec < 3600:
		return fmt.Sprintf("%dm", sec/60)
	default:
		return fmt.Sprintf("%dh%02dm", sec/3600, (sec%3600)/60)
	}
}

// ---- persistence --------------------------------------------------------------------

// digestFile is the per-session sidecar next to session.jsonl.
const digestFile = "digest.json"

// DigestIndexEntry is one row of the workspace-wide recent-digests index.
type DigestIndexEntry struct {
	SessionID string `json:"sessionId"`
	AgentID   string `json:"agentId,omitempty"`
	Title     string `json:"title"`
	At        int64  `json:"at"`
	Line      string `json:"line"`
	Hash      string `json:"hash"`
	OpenLoops int    `json:"openLoops,omitempty"`
	Errors    int    `json:"errors,omitempty"`
	Artifacts int    `json:"artifacts,omitempty"`
	Notes     int    `json:"notes,omitempty"`
	Suggest   bool   `json:"suggestNote,omitempty"`
}

func (d Digest) indexEntry() DigestIndexEntry {
	return DigestIndexEntry{
		SessionID: d.SessionID, AgentID: d.AgentID, Title: d.Title, At: d.At, Line: d.Line(), Hash: d.Hash,
		OpenLoops: len(d.OpenLoops), Errors: d.ToolErrors, Artifacts: len(d.Artifacts), Notes: len(d.Notes), Suggest: d.SuggestNote,
	}
}

// digestIndexCap bounds the index (newest kept).
const digestIndexCap = 200

type digestIndex struct {
	Version int                `json:"version"`
	Entries []DigestIndexEntry `json:"entries"`
}

func loadDigestIndex(path string) digestIndex {
	var idx digestIndex
	b, err := os.ReadFile(path)
	if err != nil {
		return idx
	}
	_ = json.Unmarshal(b, &idx)
	return idx
}

func saveDigestIndex(path string, idx digestIndex) error {
	idx.Version = 1
	sort.SliceStable(idx.Entries, func(i, j int) bool { return idx.Entries[i].At > idx.Entries[j].At })
	if len(idx.Entries) > digestIndexCap {
		idx.Entries = idx.Entries[:digestIndexCap]
	}
	return writeJSONAtomic(path, idx)
}

func (idx *digestIndex) upsert(e DigestIndexEntry) {
	for i := range idx.Entries {
		if idx.Entries[i].SessionID == e.SessionID {
			idx.Entries[i] = e
			return
		}
	}
	idx.Entries = append(idx.Entries, e)
}

func (idx *digestIndex) remove(sessionID string) {
	out := idx.Entries[:0]
	for _, e := range idx.Entries {
		if e.SessionID != sessionID {
			out = append(out, e)
		}
	}
	idx.Entries = out
}

func writeJSONAtomic(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// sessionAge is the recency window beyond which a digest no longer counts as
// "recent" for the brief.
const sessionAge = 14 * 24 * time.Hour
