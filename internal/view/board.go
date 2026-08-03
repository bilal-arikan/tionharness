package view

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// BoardInput is everything the board projection reads.
type BoardInput struct {
	Tasks []db.Task
	// Now is the clock used for age computations. Zero means time.Now().
	Now time.Time
}

// Board projection limits. A board view is a SIGNAL report, not a card list:
// naming 187 cards would cost more than reading the board itself.
const (
	boardStaleDays   = 3  // a card sitting in a working column this long is called out
	boardSignalCards = 6  // how many card ids a single signal line names
	boardFullCards   = 40 // per-card detail lines at LevelFull
	// boardWorkingWIP is the soft in-progress ceiling. It is a heuristic, not a
	// configured limit — the view flags a suspiciously wide "doing" column, it does
	// not enforce policy.
	boardWorkingWIP = 8
)

// ProjectBoard renders the kanban board: how work is distributed across columns,
// what has stopped moving, and what changed recently.
//
// Columns are derived from the tasks themselves rather than from the workspace's
// configured column list. That keeps this package free of a settings dependency
// AND keeps the view honest: it reports the board that exists, not the board that
// was configured. A configured-but-empty column simply does not appear.
func ProjectBoard(in BoardInput, level Level, lens Lens) (View, error) {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	v := View{
		Ref:    Ref{Kind: KindBoard, ID: BoardRefID},
		Level:  level,
		Lens:   lens,
		AsOf:   now,
		Source: fmt.Sprintf("%d/%d", len(in.Tasks), boardRevision(in.Tasks)),
	}

	cols := boardColumns(in.Tasks)
	v.Header = fmt.Sprintf("BOARD · %d kart · %d sütun · asOf %s",
		len(in.Tasks), len(cols), hhmmss(now))

	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	var l lines
	if len(in.Tasks) == 0 {
		l.add("(panoda kart yok)")
		v.Body = l.String()
		v.finalize()
		return v, nil
	}

	l.add("%s", boardHistogram(cols))
	for _, s := range boardSignals(in, cols, now, lens) {
		l.add("%s", s)
	}

	if level == LevelFull {
		detail, dropped := boardDetail(in.Tasks, now)
		if detail != "" {
			l.add("--")
			l.add("%s", detail)
		}
		v.Elided, v.ElidedUnit = dropped, "kart"
	} else {
		// At card level the individual cards are deliberately not listed; say so
		// with a number rather than letting the reader assume they were all shown.
		v.Elided, v.ElidedUnit = len(in.Tasks), "kart"
	}

	v.Body = l.String()
	v.Handles = []Handle{{
		Label: "kart listesi",
		Ref:   Ref{Kind: KindBoard, ID: BoardRefID},
		Level: LevelFull,
	}}
	v.finalize()
	return v, nil
}

// boardColumn is one column's rollup.
type boardColumn struct {
	Key   string
	Tasks []db.Task
}

// boardColumns groups tasks by board state, ordering the built-in columns first
// (in their canonical board order) and appending workspace-custom keys
// alphabetically, so the histogram reads left-to-right like the real board.
func boardColumns(tasks []db.Task) []boardColumn {
	byKey := map[string][]db.Task{}
	for _, t := range tasks {
		key := t.BoardState
		if key == "" {
			key = "(boş)"
		}
		byKey[key] = append(byKey[key], t)
	}

	order := map[string]int{}
	for i, c := range db.DefaultBoardColumns() {
		order[c.Key] = i
	}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		oi, ki := order[keys[i]]
		oj, kj := order[keys[j]]
		if ki != kj {
			return ki // known columns come before unknown ones
		}
		if ki && kj {
			return oi < oj
		}
		return keys[i] < keys[j]
	})

	out := make([]boardColumn, 0, len(keys))
	for _, k := range keys {
		out = append(out, boardColumn{Key: k, Tasks: byKey[k]})
	}
	return out
}

// boardHistogram is the one line that carries most of a board view's value.
func boardHistogram(cols []boardColumn) string {
	parts := make([]string, 0, len(cols))
	for _, c := range cols {
		p := fmt.Sprintf("%s %d", c.Key, len(c.Tasks))
		if isWorkingColumn(c.Key) && len(c.Tasks) > boardWorkingWIP {
			p += "⚠WIP"
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, " | ")
}

// boardSignals is the L1 layer: what deserves attention right now.
func boardSignals(in BoardInput, cols []boardColumn, now time.Time, lens Lens) []string {
	var out []string

	if lens != LensRecent {
		if stale := staleCards(cols, now); len(stale) > 0 {
			out = append(out, fmt.Sprintf("⚠ %d kart >%dg hareketsiz: %s",
				len(stale), boardStaleDays, namesOf(stale, boardSignalCards)))
		}
		if failed := cardsInColumn(cols, db.BoardFailed); len(failed) > 0 {
			out = append(out, fmt.Sprintf("✗ %d başarısız kart: %s",
				len(failed), namesOf(failed, boardSignalCards)))
		}
		if blocked := blockedCards(in.Tasks); len(blocked) > 0 {
			out = append(out, fmt.Sprintf("⛔ %d kart bağımlılıkla bloke: %s",
				len(blocked), namesOf(blocked, boardSignalCards)))
		}
		if over := overdueCards(in.Tasks, now); len(over) > 0 {
			out = append(out, fmt.Sprintf("📅 %d kart gecikmiş: %s",
				len(over), namesOf(over, boardSignalCards)))
		}
	}
	if lens == LensStale || lens == LensErrors {
		// These lenses are single-purpose: everything below is noise for them.
		return out
	}

	if recent := recentlyTouched(in.Tasks, now, 24*time.Hour); len(recent) > 0 {
		out = append(out, fmt.Sprintf("Δ24s: %d kart değişti (%s)",
			len(recent), namesOf(recent, boardSignalCards)))
	}
	if n := unassignedCount(in.Tasks); n > 0 {
		out = append(out, fmt.Sprintf("• %d kartın sahibi yok", n))
	}
	return out
}

// boardDetail is the LevelFull tail: one line per card, most recently touched
// first. Returns the text and how many cards it dropped.
func boardDetail(tasks []db.Task, now time.Time) (string, int) {
	sorted := append([]db.Task(nil), tasks...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].UpdatedAt > sorted[j].UpdatedAt })

	dropped := 0
	if len(sorted) > boardFullCards {
		dropped = len(sorted) - boardFullCards
		sorted = sorted[:boardFullCards]
	}

	var l lines
	for _, t := range sorted {
		owner := t.OwnerAgentID
		if owner == "" {
			owner = "-"
		}
		prio := t.Priority
		if prio == "" {
			prio = "-"
		}
		l.add("%-12s %-10s p:%-8s %-14s %s", t.ID, t.BoardState, prio,
			age(t.UpdatedAt, now)+" önce", clip(t.Title, 70))
	}
	if l.empty() {
		return "", dropped
	}
	return l.String(), dropped
}

// isWorkingColumn reports whether a column represents work in flight — the only
// place a wide column is a warning rather than a backlog.
func isWorkingColumn(key string) bool {
	return key == db.BoardInProgress || key == db.BoardReview
}

// staleCards are cards sitting untouched in a working column. A backlog card that
// has not moved is normal; an in-progress one that has not moved is not.
func staleCards(cols []boardColumn, now time.Time) []db.Task {
	cutoff := now.Add(-time.Duration(boardStaleDays) * 24 * time.Hour).UnixMilli()
	var out []db.Task
	for _, c := range cols {
		if !isWorkingColumn(c.Key) {
			continue
		}
		for _, t := range c.Tasks {
			if t.UpdatedAt > 0 && t.UpdatedAt < cutoff {
				out = append(out, t)
			}
		}
	}
	return out
}

// cardsInColumn returns every card in one column.
func cardsInColumn(cols []boardColumn, key string) []db.Task {
	for _, c := range cols {
		if c.Key == key {
			return c.Tasks
		}
	}
	return nil
}

// blockedCards are cards with at least one dependency that is not done.
func blockedCards(tasks []db.Task) []db.Task {
	done := map[string]bool{}
	for _, t := range tasks {
		done[t.ID] = t.BoardState == db.BoardDone
	}
	var out []db.Task
	for _, t := range tasks {
		if t.BoardState == db.BoardDone {
			continue
		}
		for _, dep := range parseDependencies(t.Dependencies) {
			if !done[dep] {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

// parseDependencies decodes Task.Dependencies (a JSON array of task ids). A
// malformed value yields no dependencies rather than an error: this is a signal
// line, and a card with unreadable deps must not break the whole board view.
func parseDependencies(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	return ids
}

// overdueCards are unfinished cards whose due date has passed.
func overdueCards(tasks []db.Task, now time.Time) []db.Task {
	today := now.Format("2006-01-02")
	var out []db.Task
	for _, t := range tasks {
		if t.DueDate != "" && t.DueDate < today && t.BoardState != db.BoardDone {
			out = append(out, t)
		}
	}
	return out
}

// recentlyTouched are cards updated inside the window.
func recentlyTouched(tasks []db.Task, now time.Time, window time.Duration) []db.Task {
	cutoff := now.Add(-window).UnixMilli()
	var out []db.Task
	for _, t := range tasks {
		if t.UpdatedAt >= cutoff {
			out = append(out, t)
		}
	}
	return out
}

// unassignedCount totals cards with no owner agent, excluding finished work
// (nobody needs to own a done card).
func unassignedCount(tasks []db.Task) int {
	n := 0
	for _, t := range tasks {
		if t.OwnerAgentID == "" && t.BoardState != db.BoardDone {
			n++
		}
	}
	return n
}

// namesOf lists up to max card ids, reporting the remainder as a count so the
// signal line never implies it named everything.
func namesOf(tasks []db.Task, max int) string {
	ids := make([]string, 0, max)
	for i, t := range tasks {
		if i >= max {
			return strings.Join(ids, ", ") + fmt.Sprintf(" +%d", len(tasks)-max)
		}
		ids = append(ids, t.ID)
	}
	return strings.Join(ids, ", ")
}

// boardRevision is a cheap change stamp: the newest UpdatedAt on the board. Two
// views with the same (count, revision) describe the same board.
func boardRevision(tasks []db.Task) int64 {
	var newest int64
	for _, t := range tasks {
		if t.UpdatedAt > newest {
			newest = t.UpdatedAt
		}
	}
	return newest
}
