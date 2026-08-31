package db

import (
	"errors"
	"fmt"
	"regexp"
)

// Board grouping axes. The board's columns are DERIVED from this: "status" uses
// the workspace's BoardColumnDef list (the classic kanban), the others build
// columns on the fly from the tasks themselves. Dragging a card between columns
// writes the field the axis names, so the same drag gesture re-assigns an agent
// under GroupByAgent and re-prioritises under GroupByPriority.
const (
	GroupByStatus   = "status"
	GroupByAgent    = "agent"
	GroupByPriority = "priority"
	GroupByTag      = "tag"
)

// Board sort orders applied within each column.
const (
	SortUpdated  = "updated"
	SortPriority = "priority"
	SortDeps     = "deps"
	SortTitle    = "title"
)

// Dependency filter buckets.
const (
	DepBlocked = "blocked" // has at least one dependency that is not done
	DepReady   = "ready"   // has dependencies and all of them are done
)

// UnassignedAgentID is the sentinel used in BoardFilter.AgentIDs to match tasks
// with no owner agent. A real agent id can never be "-", so this cannot collide.
const UnassignedAgentID = "-"

// BoardFilter narrows the board to a subset of tasks.
//
// Facets combine with AND (a task must satisfy every active facet); values
// WITHIN one facet combine with OR. An empty slice / empty string means the
// facet is inactive and matches everything — so the zero value is "show all".
//
// Evaluation happens client-side: ListTasks already returns the whole board and
// the UI holds it in state, so this type exists to be STORED (inside a
// BoardViewDef) rather than executed here. Keeping it typed on the backend lets
// agents author views through self-management tools and lets the projection
// layer (_Docs/66) reuse it as a board lens.
type BoardFilter struct {
	Text       string   `json:"text,omitempty"`       // case-insensitive substring over title + description
	Priorities []string `json:"priorities,omitempty"` // critical|high|medium|low
	Tags       []string `json:"tags,omitempty"`
	AgentIDs   []string `json:"agentIds,omitempty"` // UnassignedAgentID matches ownerless tasks
	Columns    []string `json:"columns,omitempty"`  // boardState keys to keep
	// Dep stays single-valued: blocked and ready are mutually exclusive states of
	// the same task, so the UI renders it as a radio, not a checklist.
	Dep string `json:"dep,omitempty"`
}

// IsZero reports whether no facet is active (the filter matches every task).
func (f BoardFilter) IsZero() bool {
	return f.Text == "" && len(f.Priorities) == 0 && len(f.Tags) == 0 &&
		len(f.AgentIDs) == 0 && len(f.Columns) == 0 && f.Dep == ""
}

// Validate rejects unknown enum values so a client typo cannot silently persist
// a filter that matches nothing. Free-form facets (Text, Tags, AgentIDs,
// Columns) are not checked: tags and columns are user-defined, and an agent id
// that no longer resolves is handled by the UI, not by rejecting the save.
func (f BoardFilter) Validate() error {
	for _, p := range f.Priorities {
		if p == "" || !ValidPriority(p) {
			return fmt.Errorf("unknown priority in filter: %q", p)
		}
	}
	switch f.Dep {
	case "", DepBlocked, DepReady:
	default:
		return fmt.Errorf("unknown dependency filter: %q", f.Dep)
	}
	return nil
}

// boardViewIDRe constrains saved-view ids to a slug shape, so an id is safe to
// use as a React key, a localStorage suffix and a future URL segment.
var boardViewIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// BoardViewDef is a named filter + layout preset stored per workspace. It
// captures everything needed to reproduce a board state: what to show
// (Filter), how to split it into columns (GroupBy) and how to order each
// column (Sort).
//
// Built-in views (Tümü / Bugün / Bloke / Ajansız / Gecikmiş) are defined in the
// client and are NOT stored here — only user-created views are persisted.
type BoardViewDef struct {
	ID      string      `json:"id"`
	Label   string      `json:"label"`
	Icon    string      `json:"icon,omitempty"` // single emoji shown in the view menu
	Filter  BoardFilter `json:"filter"`
	GroupBy string      `json:"groupBy,omitempty"` // "" = GroupByStatus
	Sort    string      `json:"sort,omitempty"`    // "" = SortUpdated
}

// Validate checks a single saved view.
func (v BoardViewDef) Validate() error {
	if !boardViewIDRe.MatchString(v.ID) {
		return fmt.Errorf("board view id %q must be a lowercase slug (a-z, 0-9, _, -)", v.ID)
	}
	if v.Label == "" {
		return errors.New("board view label is required")
	}
	switch v.GroupBy {
	case "", GroupByStatus, GroupByAgent, GroupByPriority, GroupByTag:
	default:
		return fmt.Errorf("unknown board groupBy: %q", v.GroupBy)
	}
	switch v.Sort {
	case "", SortUpdated, SortPriority, SortDeps, SortTitle:
	default:
		return fmt.Errorf("unknown board sort: %q", v.Sort)
	}
	return v.Filter.Validate()
}

// ValidateBoardViews checks a whole saved-view list and rejects duplicate ids
// (which would make "update this view" ambiguous).
func ValidateBoardViews(views []BoardViewDef) error {
	seen := make(map[string]bool, len(views))
	for _, v := range views {
		if err := v.Validate(); err != nil {
			return err
		}
		if seen[v.ID] {
			return fmt.Errorf("duplicate board view id: %q", v.ID)
		}
		seen[v.ID] = true
	}
	return nil
}
