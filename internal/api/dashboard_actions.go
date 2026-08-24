package api

// The action queue: the workspace overview's "what needs a human" list. It is
// the SAME set of signals the workspace projection (internal/view) counts, but
// emitted as structured, addressable rows so the UI can link each one to the
// session, card, run or schedule it points at. The projection text stays the
// source of truth for the counts; this is its clickable sibling.

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// dashActionStaleDays mirrors the board projection's staleness threshold: a card
// sitting in a working column this long is called out. Kept in sync with
// view.boardStaleDays by intent (that constant is unexported).
const dashActionStaleDays = 3

// actionItem is one row of the attention queue. Severity drives colour and sort;
// Kind + ID let the UI route the click to the right screen.
type actionItem struct {
	Severity string `json:"severity"` // danger | warn
	Kind     string `json:"kind"`     // session | run | card | schedule
	ID       string `json:"id"`
	Label    string `json:"label"`
	Detail   string `json:"detail,omitempty"`
	Age      string `json:"age,omitempty"`
	// stamp is the unix-second timestamp the item is aged from; unexported so it
	// sorts the queue without leaking into the JSON.
	stamp int64
}

// dashboardActions builds the attention queue from the same inputs the workspace
// projection reads. Danger rows (something failed or is stuck) sort ahead of warn
// rows (something is waiting or aging); within a severity the oldest item leads,
// because time-in-trouble is what makes one more urgent than another.
func dashboardActions(sessions []db.Session, tasks []db.Task, runs []db.FlowRun,
	schedules []db.Schedule, asks []db.SessionAsk, now time.Time) []actionItem {

	title := make(map[string]string, len(sessions))
	for _, s := range sessions {
		title[s.ID] = s.Title
	}

	// Non-nil so a clean workspace serializes as [] rather than null — the UI reads
	// .length on it, and a null there is a crash, not an empty list.
	out := make([]actionItem, 0)

	// Stuck sessions — the self-heal loop gave up. Danger.
	for _, s := range sessions {
		if s.State == "archived" || s.StuckTurns == 0 {
			continue
		}
		out = append(out, actionItem{
			Severity: "danger", Kind: "session", ID: s.ID,
			Label:  firstNonEmpty(s.Title, s.ID),
			Detail: "StuckTurns > 0", Age: age(now, s.UpdatedAt), stamp: s.UpdatedAt,
		})
	}

	// Failed flow runs. Danger.
	for _, r := range runs {
		if r.Status != db.FlowFailure {
			continue
		}
		out = append(out, actionItem{
			Severity: "danger", Kind: "run", ID: r.ID,
			Label:  "Akış koşusu başarısız",
			Detail: truncate(r.Error, 100), Age: age(now, r.UpdatedAt), stamp: r.UpdatedAt,
		})
	}

	// Failed cards. Danger.
	for _, t := range tasks {
		if t.Archived || t.BoardState != db.BoardFailed {
			continue
		}
		out = append(out, actionItem{
			Severity: "danger", Kind: "card", ID: t.ID,
			Label: firstNonEmpty(t.Title, t.ID), Age: age(now, t.UpdatedAt), stamp: t.UpdatedAt,
		})
	}

	// Sessions waiting on a human answer. Warn — nothing is broken, but the
	// workspace is blocked on the user.
	for i := range asks {
		a := asks[i]
		out = append(out, actionItem{
			Severity: "warn", Kind: "session", ID: a.SessionID,
			Label:  firstNonEmpty(title[a.SessionID], a.SessionID),
			Detail: "cevap bekliyor", Age: age(now, a.CreatedAt), stamp: a.CreatedAt,
		})
	}

	// Enabled schedules whose last fire errored. Warn. A disabled schedule is a
	// deliberate pause and is intentionally excluded — reporting it as broken
	// trains the reader to ignore the row.
	for _, sc := range schedules {
		if !sc.Enabled || sc.LastDeliveryStatus != "error" {
			continue
		}
		out = append(out, actionItem{
			Severity: "warn", Kind: "schedule", ID: sc.ID,
			Label:  "Zamanlama son çalışmada hata verdi",
			Detail: truncate(firstNonEmpty(sc.LastDeliveryError, sc.Prompt), 100),
			Age:    age(now, sc.LastRunAt), stamp: sc.LastRunAt,
		})
	}

	// Cards aging in a working column. Warn — a backlog card sitting still is
	// normal; one stuck in in_progress/review is not.
	staleCut := now.Add(-dashActionStaleDays * 24 * time.Hour).Unix()
	for _, t := range tasks {
		if t.Archived || !isWorkingColumn(t.BoardState) || t.UpdatedAt == 0 || t.UpdatedAt > staleCut {
			continue
		}
		out = append(out, actionItem{
			Severity: "warn", Kind: "card", ID: t.ID,
			Label:  firstNonEmpty(t.Title, t.ID),
			Detail: ">" + strconv.Itoa(dashActionStaleDays) + "g hareketsiz",
			Age:    age(now, t.UpdatedAt), stamp: t.UpdatedAt,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if r := severityRank(out[i].Severity) - severityRank(out[j].Severity); r != 0 {
			return r < 0
		}
		return out[i].stamp < out[j].stamp // oldest first within a severity
	})
	return out
}

// isWorkingColumn reports whether a board column is one where a card is expected
// to move (so sitting still is a problem). Mirrors view.workingColumn.
func isWorkingColumn(state string) bool {
	return state == db.BoardInProgress || state == db.BoardReview
}

// severityRank orders danger ahead of warn.
func severityRank(sev string) int {
	if sev == "danger" {
		return 0
	}
	return 1
}

// age renders how long ago a unix-second stamp was, in coarse calendar units. A
// zero/unset stamp yields "" (the UI simply omits the age) rather than a fake
// "1970" span.
func age(now time.Time, stamp int64) string {
	if stamp <= 0 {
		return ""
	}
	d := now.Sub(time.Unix(stamp, 0))
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + "sn"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "dk"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "sa"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "g"
	}
}

// truncate collapses whitespace and clips to at most max runes with an ellipsis,
// keeping a detail line to one line.
func truncate(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}
