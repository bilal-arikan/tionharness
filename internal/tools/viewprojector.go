package tools

// This file is the SINGLE place a view.Projector is wired up. Before it, four
// call sites built their own (api/views.go, api/dashboard.go, get_view, expand)
// and only one of them attached the optional sources and the workspace name —
// so the same node rendered differently depending on who asked for it: the agent
// got "skill catalog unavailable" where the Explorer map showed the skill, and
// the dashboard header said "WORKSPACE" where the panel said WORKSPACE "Name".
//
// It lives in `tools` because this is the only package that already imports all
// three optional sources (skills, insight, logbuf) plus view. The view package
// cannot do it itself: insight imports view, so the reverse edge would cycle.

import (
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/insight"
	"github.com/bilal-arikan/tionharness/internal/logbuf"
	"github.com/bilal-arikan/tionharness/internal/skills"
	"github.com/bilal-arikan/tionharness/internal/view"
)

// ViewSources are the optional, non-store inputs a fully-wired projector needs.
// Concrete pointer types rather than view's interfaces on purpose: a nil
// *skills.Store assigned to an interface field yields a NON-nil interface
// holding a nil pointer, and the projector's `== nil` guard would miss it.
// Converting here keeps that trap in one place.
type ViewSources struct {
	Skills *skills.Store
	Logs   *logbuf.Buffer
	// DefaultAgentID is the workspace's default agent (workspace settings, not the
	// store), so the agent projection can mark it. Empty = unknown, and the marker
	// is simply omitted.
	DefaultAgentID string
	// Running is the set of session ids executing right now (runtime liveness),
	// for the Explorer map's live layer. Nil = nothing live / not known. A plain
	// map on purpose: a nil interface value would be a non-nil interface.
	Running map[string]bool
}

// ViewProjector builds the projection resolver for a workspace with every
// optional source attached. wsName may be empty (the workspace header then falls
// back to the generic label); each source may be nil, in which case its
// projection reports the source as unavailable rather than failing the map.
func ViewProjector(database *db.DB, wsName string, src ViewSources) *view.Projector {
	s := view.Sources{}
	if src.Skills != nil {
		s.Skills = src.Skills
	}
	if src.Logs != nil {
		s.Logs = src.Logs
	}
	// Built-in tool groups are static code (categories.go): always attached.
	s.ToolGroups = builtinToolGroups{}
	if src.Running != nil {
		s.Live = view.RunningSet(src.Running)
	}
	// The findings sidecar lives next to the workspace store. A store that will
	// not open degrades the insight nodes to "unavailable" — it must not take the
	// rest of the map down with it.
	if database != nil {
		if store, err := insight.OpenFindingStore(database.Root()); err == nil {
			s.Findings = viewFindingsSource{store}
		}
	}
	return view.NewProjector(database).
		WithName(wsName).
		WithDefaultAgent(src.DefaultAgentID).
		WithSources(s)
}

// builtinToolGroups adapts categories.go to view.ToolGroupsSource. Labels are
// the category keys; the UI localizes them (toolMeta.ts CATEGORY_LABELS).
type builtinToolGroups struct{}

func (builtinToolGroups) ToolGroups() []view.ToolGroup {
	cats := BuiltinToolsByCategory()
	out := make([]view.ToolGroup, 0, len(cats))
	for _, c := range cats {
		out = append(out, view.ToolGroup{Key: c.Key, Label: c.Key, Tools: c.Tools})
	}
	return out
}

// viewFindingsSource adapts *insight.FindingStore to view.FindingsSource.
type viewFindingsSource struct{ store *insight.FindingStore }

func (f viewFindingsSource) ListFindings() []view.InsightFinding {
	raw := f.store.List("", "")
	out := make([]view.InsightFinding, 0, len(raw))
	for _, fd := range raw {
		out = append(out, view.InsightFinding{
			ID:                 fd.ID,
			LensID:             fd.LensID,
			Channel:            string(fd.Channel),
			Title:              fd.Title,
			RootCause:          fd.RootCause,
			ProposedFix:        fd.ProposedFix,
			Severity:           fd.Severity,
			Status:             string(fd.Status),
			Occurrences:        fd.Occurrences,
			EvidenceSessionIDs: fd.EvidenceSessionIDs,
			Regressed:          fd.Regressed,
			LastSeen:           fd.LastSeen,
		})
	}
	return out
}
