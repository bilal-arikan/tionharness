package tools

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// listpage.go — the shared pagination / sorting contract behind EVERY list tool
// (TSK68). list_agents, list_tasks, list_flows, list_automations,
// list_schedules, list_workers and list_workspaces all accept the same
// limit/offset/sort arguments and reply with the same envelope, so an agent
// that learns one can page through all of them.
//
// The envelope is:
//
//	{ "items": [...], "total": N, "offset": O, "limit": L, "hasMore": bool }
//
// total is the count AFTER filters but BEFORE paging; an agent reaches the
// whole result set by re-issuing the call with offset += limit while hasMore.
// Legacy parameter-less calls keep working: they are a page 1 with the default
// limit (20) — nothing errors, nothing silently returns a different ordering.

const (
	// defaultPageLimit is the page size when the caller omits limit.
	defaultPageLimit = 20
	// maxPageLimit caps a single page so a misbehaving caller cannot pull the
	// whole store into one response.
	maxPageLimit = 100
)

// pageArgs normalizes the shared limit/offset arguments of every list tool:
// limit <= 0 → defaultPageLimit, limit > maxPageLimit → maxPageLimit,
// offset < 0 → 0. No error path: a caller asking for a page that does not
// exist gets an empty items list with the true total instead.
func PageArgs(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = defaultPageLimit
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// pageResult wraps a page of items in the standard listing envelope. items is
// the already-marshalled row slice (never nil — an empty page marshals as []).
func pageResult(items any, total, offset, limit int) (string, error) {
	b, err := json.Marshal(struct {
		Items   any  `json:"items"`
		Total   int  `json:"total"`
		Offset  int  `json:"offset"`
		Limit   int  `json:"limit"`
		HasMore bool `json:"hasMore"`
	}{Items: items, Total: total, Offset: offset, Limit: limit, HasMore: offset+limit < total})
	if err != nil {
		return "", fmt.Errorf("marshal page: %w", err)
	}
	return string(b), nil
}

// slicePage cuts [offset, offset+limit) out of a filtered list. Returns the
// page plus the total, mirroring pageResult's numbers so callers can hand them
// straight over.
func SlicePage[T any](all []T, offset, limit int) ([]T, int) {
	total := len(all)
	if offset >= total {
		return []T{}, total
	}
	end := min(offset+limit, total)
	return all[offset:end], total
}

// sortOrder splits a sort argument into its field and direction halves. The
// valid keys are updated_desc (the default), updated_asc, created_desc,
// created_asc, name_asc, name_desc. Anything else is an explicit error — a
// typo must NOT silently fall back to a different ordering.
//
// Entities without an updated timestamp (workspaces) map that key to a
// documented surrogate; the mapping is stated in each tool's description, never
// hidden.
func SortOrder(sortArg string) (field string, asc bool, err error) {
	s := strings.TrimSpace(sortArg)
	if s == "" {
		return "updated", false, nil
	}
	field, dir, ok := strings.Cut(s, "_")
	if !ok {
		return "", false, fmt.Errorf("sort must be one of updated_desc, updated_asc, created_desc, created_asc, name_asc, name_desc; got %q", s)
	}
	switch dir {
	case "asc":
		asc = true
	case "desc":
		asc = false
	default:
		return "", false, fmt.Errorf("sort direction must be \"asc\" or \"desc\"; got %q", s)
	}
	switch field {
	case "updated", "created", "name":
	default:
		return "", false, fmt.Errorf("sort field must be updated, created or name; got %q", s)
	}
	return field, asc, nil
}

// SortByField is a tiny combinator for the per-tool sort switches: it returns a
// three-way comparator for a resolved sort key, or an error for a key the entity
// cannot satisfy. The comparator is already direction-aware, so callers can hand
// it straight to slices.SortStableFunc.
//
// The three extraction closures answer "what is this row's updated-at /
// created-at / name". A nil getter for a key means the entity has no such
// field; the caller decides between rejecting the key and mapping it to a
// documented surrogate.
//
// id is the MANDATORY tiebreak and is what makes paging safe. The store hands
// back rows in Go map-iteration order (see db.dbList), which is randomized per
// call, and the timestamps are unix SECONDS — so equal keys are common and would
// otherwise land in a different order on every request. Since offset=0 and
// offset=20 are separate calls, an unstable tie makes a paging reader skip rows
// and see others twice. Ordering by id last pins the sequence.
func SortByField[T any](field string, asc bool,
	updated, created func(T) int64,
	name func(T) string,
	id func(T) string) (func(a, b T) int, error) {

	if updated == nil && field == "updated" {
		return nil, fmt.Errorf("sort field \"updated\" is not supported for this entity")
	}
	if created == nil && field == "created" {
		return nil, fmt.Errorf("sort field \"created\" is not supported for this entity")
	}
	if name == nil && field == "name" {
		return nil, fmt.Errorf("sort field \"name\" is not supported for this entity")
	}
	if id == nil {
		return nil, fmt.Errorf("sort requires an id tiebreak for stable paging")
	}
	// compare is the ascending three-way order for the resolved field with the
	// id tiebreak; desc swaps the arguments, which keeps equal keys equal (a
	// strict weak order) so SortStableFunc's stability is preserved.
	compare := func(a, b T) int {
		var c int
		switch field {
		case "name":
			c = cmp.Compare(strings.ToLower(name(a)), strings.ToLower(name(b)))
		case "created":
			c = cmp.Compare(created(a), created(b))
		default: // updated
			c = cmp.Compare(updated(a), updated(b))
		}
		if c != 0 {
			return c
		}
		return cmp.Compare(id(a), id(b))
	}
	if asc {
		return compare, nil
	}
	return func(a, b T) int { return compare(b, a) }, nil
}

// splitTags parses a comma-separated tags filter argument ("a, b" → ["a","b"]).
func SplitTags(s string) []string {
	var out []string
	for t := range strings.SplitSeq(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// hasAllTags reports whether got contains every required tag (the AND
// semantics of the tags filter — a card carrying all of them passes).
func HasAllTags(got, required []string) bool {
	for _, r := range required {
		found := slices.Contains(got, r)
		if !found {
			return false
		}
	}
	return true
}
