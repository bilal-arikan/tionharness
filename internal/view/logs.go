package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/logbuf"
)

// LogsInput is the recent process log tail. The projection renders the last
// logsRows entries inline — the map's logs node is a leaf (like budget/tools),
// so there is no per-entry drill-down; the reader sees the recent stream and
// jumps to the Günlükler screen for filtering.
type LogsInput struct {
	Entries []logbuf.Entry
	// Now is the clock used for the asOf stamp. Zero means time.Now().
	Now time.Time
}

// logsRows is how many entries a card-level logs view keeps before the rest are
// reported as Elided. LevelFull keeps more, but always bounded — a log stream is
// the one entity that grows without bound, so the cap is the honest floor.
const (
	logsRows     = 30
	logsRowsFull = 120
)

// countLogLevels tallies the error and warning records in one slice. Level
// spellings vary by writer ("ERROR", "error", "warn", "WARNING"), so the match is
// case-insensitive and prefix-based.
func countLogLevels(entries []logbuf.Entry) (errs, warns int) {
	for _, e := range entries {
		switch lv := strings.ToLower(e.Level); {
		case strings.HasPrefix(lv, "err"), strings.HasPrefix(lv, "fatal"):
			errs++
		case strings.HasPrefix(lv, "warn"):
			warns++
		}
	}
	return errs, warns
}

// ProjectLogs renders the recent process log tail, oldest → newest (the stream
// reads top-down like a terminal that ended).
func ProjectLogs(in LogsInput, level Level) (View, error) {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	entries := in.Entries

	limit := logsRows
	if level == LevelFull {
		limit = logsRowsFull
	}
	kept := entries
	elided := 0
	elidedUnit := ""
	if len(kept) > limit {
		elided = len(kept) - limit
		elidedUnit = "kayıt"
		kept = kept[len(kept)-limit:]
	}

	v := View{
		Ref:        Ref{Kind: KindLogs, ID: LogsRefID},
		Level:      level,
		AsOf:       now,
		Source:     fmt.Sprintf("%d", len(entries)),
		Elided:     elided,
		ElidedUnit: elidedUnit,
	}

	// The failure count over the RENDERED window, not the whole buffer: it is the
	// one question a log tail is opened to answer, and counting the elided rows
	// too would promise a scan the body cannot back up.
	errs, warns := countLogLevels(kept)
	head := fmt.Sprintf("LOGS · %d kayıt", len(entries))
	if errs > 0 || warns > 0 {
		head += fmt.Sprintf(" · son %d kayıtta %d hata / %d uyarı", len(kept), errs, warns)
	}
	v.Header = head + " · asOf " + hhmmss(now)

	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	var l lines
	for _, e := range kept {
		// A log message is prose that routinely quotes a file it failed to open, and
		// clip cuts exactly that tail off — compactPaths shortens the path in place
		// so both the sentence and the file survive.
		msg := clip(compactPaths(e.Message), 90)
		src := e.Component
		if src == "" && e.Session != "" {
			src = "session:" + e.Session
		}
		if src == "" && e.Workspace != "" {
			src = "ws:" + e.Workspace
		}
		if src != "" {
			l.add("%s %-5s %s (%s)", hhmmss(tsMs(e.Time)), e.Level, msg, clip(src, 24))
		} else {
			l.add("%s %-5s %s", hhmmss(tsMs(e.Time)), e.Level, msg)
		}
	}
	if l.empty() {
		l.add("(log kaydı yok)")
	}

	v.Body = l.String()
	v.finalize()
	return v, nil
}
