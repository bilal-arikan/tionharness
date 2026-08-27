package view

import (
	"fmt"
	"strings"
	"time"
)

// Category ids. A category is a structural bucket in the Explorer map: it counts
// its members and lists the top-N as drill-down handles. sessions/flows/agents
// are the workspace-level buckets; a board column is a per-column bucket whose id
// carries the column key. artifacts/automations/skills/insights are the map
// extension buckets (TSK66): artifacts + automations read from the store, skills
// from the runtime catalog and insights from the findings sidecar.
const (
	CategorySessions    = "sessions"
	CategoryFlows       = "flows"
	CategoryAgents      = "agents"
	CategoryArtifacts   = "artifacts"
	CategoryAutomations = "automations"
	CategorySkills      = "skills"
	CategoryInsights    = "insights"
	// categoryColumnPrefix marks a board-column category: ID = "col:in_progress".
	categoryColumnPrefix = "col:"
)

// categoryTopN is the per-node child cap (see _Docs/68 §8.1): a category lists at
// most this many members as handles and reports the remainder as Elided, so a
// 300-session workspace cannot blow the map up.
const categoryTopN = 50

// categoryFullMaxBytes budgets the LevelFull member listing (~600 tokens). It is
// deliberately BELOW what categoryTopN full-width rows would occupy (~3.1KB): a
// cap that the largest legal input cannot reach is decoration, and the elision
// path it guards would never run in production or in a test.
const categoryFullMaxBytes = 2400

// CategoryInput is a resolved category: its id plus the FULL (uncapped, already
// member list. The projection counts the members and caps the
// handles — the count is honest because it sees every member, the handle list is
// bounded because the map cannot render every one.
type CategoryInput struct {
	ID      string
	Members []Handle
	// Now is the clock used for the asOf stamp. Zero means time.Now().
	Now time.Time
}

// ProjectCategory renders a group node: how many members the bucket holds and the
// top-N as handles, reporting the rest as Elided. An unknown category id is an
// error, not an empty node — a blank category would read like a real but empty
// bucket and hide the caller's mistake.
func ProjectCategory(in CategoryInput, level Level) (View, error) {
	label, unit, ok := categoryMeta(in.ID)
	if !ok {
		return View{}, fmt.Errorf("view: unknown category %q", in.ID)
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	v := View{
		Ref:    Ref{Kind: KindCategory, ID: in.ID},
		Level:  level,
		AsOf:   now,
		Source: fmt.Sprintf("%s/%d", in.ID, len(in.Members)),
	}
	v.Header = fmt.Sprintf("%s · %d %s · asOf %s", label, len(in.Members), unit, hhmmss(now))

	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	members := in.Members
	if len(members) > categoryTopN {
		v.Elided, v.ElidedUnit = len(members)-categoryTopN, unit
		members = members[:categoryTopN]
	}
	v.Handles = members

	if len(members) == 0 {
		// An empty bucket says so rather than rendering a blank body.
		var l lines
		l.add("(bu kategoride %s yok)", unit)
		v.Body = l.String()
		v.finalize()
		return v, nil
	}

	// LevelFull spells the members out as text. At card level they are handles
	// only — clickable in the panel, but invisible to anyone reading the DSL, so
	// a category rendered card and full identically and the panel's `full` button
	// did nothing. The budget tier has to change what the TEXT says, otherwise it
	// is not a budget tier.
	if level == LevelFull {
		rows := make([]string, 0, len(members))
		for _, m := range members {
			rows = append(rows, fmt.Sprintf("  %-46s %s", clip(m.Label, 46), m.Ref.String()))
		}
		kept, dropped := CapLines(rows, categoryFullMaxBytes)
		v.Body = strings.Join(kept, "\n")
		// Add to the topN overflow rather than replacing it: both are members this
		// view is not showing, and reporting only one of them would understate the
		// gap.
		if dropped > 0 {
			v.Elided, v.ElidedUnit = v.Elided+dropped, unit
		}
	}
	v.finalize()
	return v, nil
}

// categoryMeta resolves a category id to its display label and member unit,
// reporting whether the id is known.
func categoryMeta(id string) (label, unit string, ok bool) {
	switch id {
	case CategorySessions:
		return "OTURUMLAR", "oturum", true
	case CategoryFlows:
		return "AKIŞLAR", "koşu", true
	case CategoryAgents:
		return "AJANLAR", "ajan", true
	case CategoryArtifacts:
		return "ARTIFACTS", "artifact", true
	case CategoryAutomations:
		return "OTOMASYONLAR", "otomasyon", true
	case CategorySkills:
		return "SKILL'LAR", "skill", true
	case CategoryInsights:
		return "İÇGÖRÜLER", "bulgu", true
	}
	if key, found := strings.CutPrefix(id, categoryColumnPrefix); found && key != "" {
		return "SÜTUN:" + key, "kart", true
	}
	return "", "", false
}
