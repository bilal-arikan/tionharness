package view

import (
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TodoRollup is the state of a session's checklist: the items themselves plus
// the two derived facts every consumer wants — how many are done and what is in
// flight.
//
// It is the single home for "find the newest checklist in a transcript", which
// had grown three independent copies: the session projection, the system-prompt
// todo block (internal/api) and — declared but never populated — the handoff
// environment snapshot.
type TodoRollup struct {
	Items []TodoItem
	Done  int
	// Active is the content of the first in-progress item, or "" when nothing is
	// in flight.
	Active string
}

// Empty reports whether the session has no checklist at all.
func (r TodoRollup) Empty() bool { return len(r.Items) == 0 }

// AllDone reports whether every item is completed. Callers use this to hide a
// finished list: a checklist with nothing left to do is noise, not state.
func (r TodoRollup) AllDone() bool { return len(r.Items) > 0 && r.Done == len(r.Items) }

// LatestTodos returns the newest checklist across a session's messages, scanning
// newest-first. Only the most recent list matters: todo_write replaces the list
// wholesale, so an older one is superseded state, not extra state.
func LatestTodos(msgs []db.Message) TodoRollup {
	for i := len(msgs) - 1; i >= 0; i-- {
		steps := DecodeSteps(msgs[i].Steps)
		for j := len(steps) - 1; j >= 0; j-- {
			if items := steps[j].TodoItems(); len(items) > 0 {
				return newTodoRollup(items)
			}
		}
	}
	return TodoRollup{}
}

// newTodoRollup derives the counts once so no caller re-walks the list.
func newTodoRollup(items []TodoItem) TodoRollup {
	r := TodoRollup{Items: items}
	for _, t := range items {
		switch t.Status {
		case "completed":
			r.Done++
		case "in_progress":
			if r.Active == "" {
				r.Active = t.Content
			}
		}
	}
	return r
}

// RenderChecklist renders the items as a 1-based `N. [x] content` list.
//
// English and index-bearing on purpose: both consumers are model-facing prompt
// text, and the indices are the handle the compact todo_write form
// (`{"set":{"3":"completed"}}`) refers to. A renderer that dropped them would
// leave the agent unable to update the list it was just shown.
//
// Returns "" for an empty rollup. A COMPLETED list still renders — the caller
// decides whether finished work is worth showing (a system prompt hides it; a
// handoff keeps it, because "these are already done" is exactly what stops a
// fresh agent redoing them).
func (r TodoRollup) RenderChecklist() string {
	if len(r.Items) == 0 {
		return ""
	}
	var b strings.Builder
	for i, t := range r.Items {
		mark := " "
		switch t.Status {
		case "completed":
			mark = "x"
		case "in_progress":
			mark = "~"
		}
		fmt.Fprintf(&b, "%d. [%s] %s\n", i+1, mark, t.Content)
	}
	return strings.TrimRight(b.String(), "\n")
}
