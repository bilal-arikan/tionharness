package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// The chat's "all file changes" popup needs every file mutation a session ever
// made, with its FULL patch. Two things make that impossible to assemble from
// the transcript listing: that endpoint trims long fields to stepFieldCap (see
// steps_trim.go), and re-fetching each turn's untrimmed trace one by one would
// ship the whole session — Read/Grep/Bash outputs included — just to find a
// handful of edits.
//
// So this endpoint walks the transcript once and returns ONLY the file-mutating
// steps, untrimmed. Everything else is dropped, which is what makes it cheap.
//
// The steps are handed back as raw TurnStep JSON rather than a parsed
// {path, patch, added, removed} shape on purpose: for the claude-cli path the
// CLI applies the edit itself, so no server-side FileDiff was ever recorded and
// the patch has to be synthesized from the tool input. That synthesis already
// exists in the frontend (shared/lib/diff.ts) and drives the in-chat diff card;
// duplicating it in Go would give the popup and the card two sources of truth.

// editToolBases are the file-mutating tool names whose plain `tool` step still
// represents a real file change (the claude-cli path). Compared against the
// tool's base name — MCP namespace prefix stripped, lower-cased.
var editToolBases = map[string]bool{
	"edit":       true,
	"edit_file":  true,
	"write":      true,
	"write_file": true,
	"multiedit":  true,
}

// sessionChange is one file-mutating step lifted out of a session's transcript,
// tagged with the turn it belongs to so the popup can group by turn and
// deep-link back to the message.
type sessionChange struct {
	MsgID     string `json:"msgId"`
	AgentID   string `json:"agentId,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	// Nested: this change was made by a subagent inside the turn, not by the
	// turn's own agent. Surfaced so the popup can label it rather than implying
	// the main agent made the edit.
	Nested bool `json:"nested,omitempty"`
	// Step is the raw TurnStep JSON, untrimmed.
	Step json.RawMessage `json:"step"`
}

// handleSessionChanges returns every file mutation in a session, oldest first.
// GET /api/sessions/{id}/changes
func (s *Server) handleSessionChanges(w http.ResponseWriter, r *http.Request) {
	out := []sessionChange{}
	// Streamed, not listed: this walks every turn but keeps only the file-mutating
	// steps, so materialising a copy of the whole transcript first would be pure
	// waste. The callback only parses and appends — no store calls, as
	// StreamMessages requires.
	err := ws(r).DB.StreamMessages(r.Context(), r.PathValue("id"), func(m db.Message) bool {
		if len(m.Steps) < 2 {
			return true
		}
		var steps []json.RawMessage
		// A trace that will not parse is skipped rather than failing the request:
		// one unreadable turn should not hide every other change in the session.
		if json.Unmarshal([]byte(m.Steps), &steps) != nil {
			return true
		}
		for _, raw := range steps {
			out = appendChanges(out, raw, m.ID, m.AgentID, m.CreatedAt, false)
		}
		return true
	})
	if writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// appendChanges appends raw to out when it is a file mutation, recursing into a
// subagent step's nested trace (a subagent's edits are real file changes too).
func appendChanges(out []sessionChange, raw json.RawMessage, msgID, agentID string, at int64, nested bool) []sessionChange {
	var st struct {
		Kind     string            `json:"kind"`
		Tool     string            `json:"tool"`
		IsError  bool              `json:"isError"`
		SubSteps []json.RawMessage `json:"subSteps"`
	}
	if json.Unmarshal(raw, &st) != nil {
		return out
	}
	for _, sub := range st.SubSteps {
		out = appendChanges(out, sub, msgID, agentID, at, true)
	}
	// An errored edit changed nothing on disk — listing it as a change would be
	// a lie. The in-chat trace still shows the failure.
	if st.IsError {
		return out
	}
	// `diff` steps carry a server-recorded patch; `tool` steps of an edit tool
	// are the claude-cli path, where the frontend synthesizes one from the input.
	if st.Kind != "diff" && !(st.Kind == "tool" && editToolBases[toolBaseName(st.Tool)]) {
		return out
	}
	return append(out, sessionChange{
		MsgID:     msgID,
		AgentID:   agentID,
		CreatedAt: at,
		Nested:    nested,
		Step:      raw,
	})
}

// toolBaseName strips an MCP namespace prefix (`server__tool`) and lower-cases
// the result, mirroring toolBase() in the frontend.
func toolBaseName(name string) string {
	if i := strings.LastIndex(name, "__"); i >= 0 {
		name = name[i+2:]
	}
	return strings.ToLower(name)
}
