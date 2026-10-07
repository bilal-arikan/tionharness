package tools

import (
	"encoding/json"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/view"
)

// IsReadTool reports whether a call of the tool can stamp the read ledger, so
// callers skip their own lookups for every other tool.
func IsReadTool(name string) bool {
	switch name {
	case "get_view", "read_artifact", "use_skill", "note_expand":
		return true
	}
	return false
}

// ReadRefFor names the Explorer map node a successful read-tool call read, so
// the runtime can stamp it in the workspace read ledger (db.RecordViewRead) and
// the map can show "last read by an agent". Only tools whose whole purpose is
// reading one entity count: get_view, read_artifact, use_skill and note_expand.
// Listings and searches do not — they touch many nodes without reading any.
//
// resolveNote maps a note_expand argument (an id OR an exact title) to the note
// id; nil or a miss records nothing rather than a ref no node carries.
func ReadRefFor(name string, input json.RawMessage, resolveNote func(string) (string, bool)) (string, bool) {
	if !IsReadTool(name) {
		return "", false
	}
	var in struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
		Sub  string `json:"sub"`
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", false
	}
	id := strings.TrimSpace(in.ID)
	switch name {
	case "get_view":
		kind := view.Kind(strings.TrimSpace(in.Kind))
		switch {
		case id == "" && kind == view.KindBoard:
			id = view.BoardRefID
		case id == "" && kind == view.KindSpace:
			id = view.WorkspaceRefID
		}
		if kind == "" || id == "" {
			return "", false
		}
		return view.Ref{Kind: kind, ID: id, Sub: strings.TrimSpace(in.Sub)}.String(), true
	case "read_artifact":
		if id == "" {
			return "", false
		}
		return view.Ref{Kind: view.KindArtifact, ID: id}.String(), true
	case "use_skill":
		slug := strings.TrimSpace(in.Slug)
		if slug == "" {
			return "", false
		}
		return view.Ref{Kind: view.KindSkill, ID: slug}.String(), true
	case "note_expand":
		if id == "" || resolveNote == nil {
			return "", false
		}
		noteID, ok := resolveNote(id)
		if !ok {
			return "", false
		}
		return view.Ref{Kind: view.KindNote, ID: noteID}.String(), true
	}
	return "", false
}
