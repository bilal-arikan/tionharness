// Package archive holds the reversible "put away" state shared by kanban cards,
// skills, artifacts, automations and agents: an archived item is never
// deleted, it just leaves the default lists (and the Map) until it is restored.
//
// This leaf package is the one place the list filter and the "refuse to use an
// archived item" error are defined, so every entity answers the same way. It
// imports nothing from the app, so both the store (internal/db) and the
// file-based skill catalog (internal/skills) can depend on it.
package archive

import (
	"errors"
	"fmt"
	"strings"
)

// ErrArchived marks an operation refused because its target is archived.
// Callers match it with errors.Is (the API maps it to 409 Conflict); the
// wrapped message names the entity so the user knows what to restore.
var ErrArchived = errors.New("archived")

// Error builds the explicit error returned when an archived entity is asked to
// do work (run an agent, fire an automation, load a skill).
func Error(kind, id, name string) error {
	label := strings.TrimSpace(name)
	if label == "" || label == id {
		label = id
	} else {
		label = fmt.Sprintf("%s (%s)", label, id)
	}
	return fmt.Errorf("%w: %s %s is archived; unarchive it before using it", ErrArchived, kind, label)
}

// Filter selects which side of the archive a list returns.
type Filter int

const (
	// Active keeps only live (non-archived) items — every default list.
	Active Filter = iota
	// Only keeps only archived items — the "archive view".
	Only
	// All keeps both.
	All
)

// Keep reports whether an item with the given archived flag passes the filter.
func (f Filter) Keep(archived bool) bool {
	switch f {
	case Only:
		return archived
	case All:
		return true
	default:
		return !archived
	}
}

// ParseFilter reads the `archived` list parameter shared by the REST list
// endpoints: "true"/"1"/"only" lists only archived items, "false"/"0"/"active"
// only live ones, "all" both. An empty value yields def. Anything else is an
// error so a typo does not silently show the wrong side of the archive.
func ParseFilter(v string, def Filter) (Filter, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return def, nil
	case "true", "1", "only":
		return Only, nil
	case "false", "0", "active":
		return Active, nil
	case "all":
		return All, nil
	}
	return def, fmt.Errorf("archived must be true, false or all (got %q)", v)
}

// FromBool maps the self-management tools' boolean `archived` argument (the
// list_tasks convention): true lists only archived items, false only live ones.
func FromBool(archived bool) Filter {
	if archived {
		return Only
	}
	return Active
}

// Apply returns the items that pass f, preserving order. archived reads the
// entity's own archive flag.
func Apply[T any](items []T, f Filter, archived func(T) bool) []T {
	out := make([]T, 0, len(items))
	for _, it := range items {
		if f.Keep(archived(it)) {
			out = append(out, it)
		}
	}
	return out
}
