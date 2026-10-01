package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

const sessionDecisionsFile = "decisions.json"

// DecisionItem identifies one bounded candidate and the action selected for it.
type DecisionItem struct {
	Key      string  `json:"key"`
	Kind     string  `json:"kind"`
	Label    string  `json:"label"`
	Action   string  `json:"action"`
	Strength float64 `json:"strength,omitempty"`
}

// SessionDecision records a recommendation separately from the applied result.
type SessionDecision struct {
	ID          string         `json:"id"`
	At          int64          `json:"at"`
	Authority   string         `json:"authority"`
	Mode        string         `json:"mode"`
	Status      string         `json:"status"`
	TraceID     string         `json:"traceId,omitempty"`
	Recommended string         `json:"recommended,omitempty"`
	Applied     string         `json:"applied,omitempty"`
	Baseline    string         `json:"baseline,omitempty"`
	Items       []DecisionItem `json:"items"`
	Error       string         `json:"error,omitempty"`
}

// DecisionMemory retains a bounded context fragment and its provenance.
type DecisionMemory struct {
	Key            string `json:"key"`
	Label          string `json:"label"`
	SourceID       string `json:"sourceId,omitempty"`
	Text           string `json:"text,omitempty"`
	Pinned         bool   `json:"pinned"`
	Mandatory      bool   `json:"mandatory"`
	AddedAtCompact int    `json:"addedAtCompact"`
	LastReminded   int    `json:"lastReminded"`
}

type DecisionRoute struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Pinned   bool   `json:"pinned"`
}

type DecisionFeedback struct {
	DecisionID string `json:"decisionId"`
	Rating     string `json:"rating"`
	Note       string `json:"note,omitempty"`
	At         int64  `json:"at"`
}

// SessionDecisions is a session-local sidecar; the transcript stays canonical.
type SessionDecisions struct {
	SessionID      string             `json:"sessionId"`
	Entries        []SessionDecision  `json:"entries"`
	Memories       []DecisionMemory   `json:"memories"`
	SelectedSkills []string           `json:"selectedSkills"`
	SkillBodies    map[string]string  `json:"skillBodies,omitempty"`
	SelectedTools  []string           `json:"selectedTools"`
	Route          *DecisionRoute     `json:"route,omitempty"`
	CompactCount   int                `json:"compactCount"`
	Feedback       []DecisionFeedback `json:"feedback"`
}

// ReadSessionDecisions returns an empty initialized state for an existing
// session without a sidecar. Unknown sessions are never used as filesystem paths.
func (d *DB) ReadSessionDecisions(ctx context.Context, sessionID string) (SessionDecisions, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if err := d.checkDecisionSession(ctx, sessionID); err != nil {
		return SessionDecisions{}, err
	}
	return d.readSessionDecisionsLocked(sessionID)
}

// UpdateSessionDecisions serializes the entire read/mutate/write transaction.
// The callback must not re-enter DB methods. A rejected or canceled mutation
// leaves the persisted sidecar unchanged.
func (d *DB) UpdateSessionDecisions(ctx context.Context, sessionID string, fn func(*SessionDecisions) error) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkDecisionSession(ctx, sessionID); err != nil {
		return err
	}
	if fn == nil {
		return errors.New("decision mutation is required")
	}
	state, err := d.readSessionDecisionsLocked(sessionID)
	if err != nil {
		return err
	}
	if err := fn(&state); err != nil {
		return err
	}
	if state.SessionID != sessionID {
		return errors.New("decision session identity cannot change")
	}
	if err := boundSessionDecisions(&state); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return atomicWriteJSON(d.dir(dirSessions, sessionID, sessionDecisionsFile), state)
}

// AppendSessionDecision allocates an event identity and millisecond timestamp
// when the caller does not already have them.
func (d *DB) AppendSessionDecision(ctx context.Context, sessionID string, entry SessionDecision) error {
	if entry.ID == "" {
		entry.ID = newID()
	}
	if entry.At == 0 {
		entry.At = time.Now().UnixMilli()
	}
	return d.UpdateSessionDecisions(ctx, sessionID, func(state *SessionDecisions) error {
		state.Entries = append(state.Entries, entry)
		return nil
	})
}

func (d *DB) checkDecisionSession(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if sessionID == "" || sessionID == "." || sessionID == ".." || strings.ContainsAny(sessionID, "/\\:\x00") {
		return ErrNotFound
	}
	if _, ok := d.sessions[sessionID]; !ok {
		return ErrNotFound
	}
	return nil
}

func (d *DB) readSessionDecisionsLocked(sessionID string) (SessionDecisions, error) {
	state := SessionDecisions{SessionID: sessionID}
	b, err := os.ReadFile(d.dir(dirSessions, sessionID, sessionDecisionsFile))
	if err != nil && !os.IsNotExist(err) {
		return SessionDecisions{}, err
	}
	if err == nil {
		if err := json.Unmarshal(b, &state); err != nil {
			return SessionDecisions{}, fmt.Errorf("read session decisions: %w", err)
		}
		if state.SessionID != sessionID {
			return SessionDecisions{}, errors.New("decision session identity mismatch")
		}
	}
	if err := boundSessionDecisions(&state); err != nil {
		return SessionDecisions{}, err
	}
	return state, nil
}

func boundSessionDecisions(state *SessionDecisions) error {
	if len(state.SkillBodies) > 16 {
		return errors.New("too many loaded skill bodies")
	}
	remainingBodies := 32768
	for key, body := range state.SkillBodies {
		if len(key) > 200 || len(body) > remainingBodies {
			return errors.New("loaded skill body budget exceeded")
		}
		remainingBodies -= len(body)
	}
	if state.CompactCount < 0 {
		return errors.New("compact count cannot be negative")
	}
	if len(state.Entries) > 128 {
		state.Entries = state.Entries[len(state.Entries)-128:]
	}
	seen := map[string]bool{}
	for i := range state.Entries {
		entry := &state.Entries[i]
		if entry.ID == "" || len(entry.ID) > 200 || seen[entry.ID] || entry.At < 0 {
			return errors.New("invalid decision identity or timestamp")
		}
		seen[entry.ID] = true
		entry.Authority = decisionText(entry.Authority, 80)
		entry.Mode = decisionText(entry.Mode, 32)
		entry.Status = decisionText(entry.Status, 32)
		entry.TraceID = decisionText(entry.TraceID, 200)
		entry.Recommended = decisionText(entry.Recommended, 2048)
		entry.Applied = decisionText(entry.Applied, 2048)
		entry.Baseline = decisionText(entry.Baseline, 2048)
		entry.Error = decisionText(entry.Error, 200)
		if len(entry.Items) > 64 {
			entry.Items = entry.Items[:64]
		}
		for j := range entry.Items {
			item := &entry.Items[j]
			if math.IsNaN(item.Strength) || math.IsInf(item.Strength, 0) || item.Strength < 0 || item.Strength > 1 {
				return errors.New("invalid decision strength")
			}
			item.Key = decisionText(item.Key, 200)
			item.Kind = decisionText(item.Kind, 64)
			item.Label = decisionText(item.Label, 160)
			item.Action = decisionText(item.Action, 64)
		}
		if entry.Items == nil {
			entry.Items = []DecisionItem{}
		}
	}
	// Keep pinned and mandatory memories ahead of ordinary fragments when the
	// candidate list grows. Within each category retain the most recent entries.
	if len(state.Memories) > 32 {
		keep := make([]bool, len(state.Memories))
		n := 0
		for _, protected := range []bool{true, false} {
			for i := len(state.Memories) - 1; i >= 0 && n < 32; i-- {
				memory := state.Memories[i]
				if (memory.Pinned || memory.Mandatory) == protected {
					keep[i], n = true, n+1
				}
			}
		}
		memories := make([]DecisionMemory, 0, 32)
		for i, memory := range state.Memories {
			if keep[i] {
				memories = append(memories, memory)
			}
		}
		state.Memories = memories
	}
	seen = map[string]bool{}
	for i := range state.Memories {
		memory := &state.Memories[i]
		if memory.Key == "" || len(memory.Key) > 200 || seen[memory.Key] || memory.AddedAtCompact < 0 || memory.LastReminded < 0 {
			return errors.New("invalid decision memory identity or compact count")
		}
		seen[memory.Key] = true
		memory.Label = decisionText(memory.Label, 160)
		memory.SourceID = decisionText(memory.SourceID, 200)
		memory.Text = decisionText(memory.Text, 8192)
	}
	remaining := 32768
	for _, protected := range []bool{true, false} {
		for i := range state.Memories {
			memory := &state.Memories[i]
			if (memory.Pinned || memory.Mandatory) == protected {
				memory.Text = decisionText(memory.Text, remaining)
				remaining -= len(memory.Text)
			}
		}
	}
	state.SelectedSkills = decisionNames(state.SelectedSkills, 16)
	state.SelectedTools = decisionNames(state.SelectedTools, 32)
	if state.Route != nil {
		state.Route.Provider = decisionText(state.Route.Provider, 200)
		state.Route.Model = decisionText(state.Route.Model, 200)
	}
	if len(state.Feedback) > 128 {
		state.Feedback = state.Feedback[len(state.Feedback)-128:]
	}
	for i := range state.Feedback {
		feedback := &state.Feedback[i]
		if feedback.DecisionID == "" || len(feedback.DecisionID) > 200 || (feedback.Rating != "helpful" && feedback.Rating != "correction") || feedback.At < 0 {
			return errors.New("invalid decision feedback")
		}
		feedback.Note = decisionText(feedback.Note, 2000)
	}
	if state.Entries == nil {
		state.Entries = []SessionDecision{}
	}
	if state.Memories == nil {
		state.Memories = []DecisionMemory{}
	}
	if state.Feedback == nil {
		state.Feedback = []DecisionFeedback{}
	}
	return nil
}

func decisionNames(names []string, limit int) []string {
	result := make([]string, 0, min(len(names), limit))
	seen := map[string]bool{}
	for _, name := range names {
		name = decisionText(strings.TrimSpace(name), 200)
		if name != "" && !seen[name] && len(result) < limit {
			result = append(result, name)
			seen[name] = true
		}
	}
	return result
}

// decisionText enforces byte budgets without cutting a UTF-8 sequence.
func decisionText(text string, limit int) string {
	text = strings.ToValidUTF8(text, "")
	if len(text) <= limit {
		return text
	}
	text = text[:limit]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text
}
