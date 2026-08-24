package view

import (
	"fmt"
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

// ProjectLogs renders the recent process log tail, oldest → newest (the stream
// reads top-down like a terminal that ended). The errors lens keeps only ERROR
// entries, so the node doubles as "son hatalar".
func ProjectLogs(in LogsInput, level Level, lens Lens) (View, error) {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	entries := filterLogEntries(in.Entries, lens)

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
		if lens == LensErrors {
			elidedUnit = "hata kaydı"
		}
		kept = kept[len(kept)-limit:]
	}

	v := View{
		Ref:        Ref{Kind: KindLogs, ID: LogsRefID},
		Level:      level,
		Lens:       lens,
		AsOf:       now,
		Source:     fmt.Sprintf("%d", len(entries)),
		Elided:     elided,
		ElidedUnit: elidedUnit,
	}

	unit := "kayıt"
	if lens == LensErrors {
		unit = "hata kaydı"
	}
	v.Header = fmt.Sprintf("LOGS · %d %s · asOf %s", len(entries), unit, hhmmss(now))

	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	var l lines
	for _, e := range kept {
		msg := clip(e.Message, 90)
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

// filterLogEntries narrows the stream by lens. Only the errors lens filters —
// every other lens passes the full tail through.
func filterLogEntries(entries []logbuf.Entry, lens Lens) []logbuf.Entry {
	if lens != LensErrors {
		return entries
	}
	out := make([]logbuf.Entry, 0, len(entries))
	for _, e := range entries {
		if e.Level == "ERROR" {
			out = append(out, e)
		}
	}
	return out
}
