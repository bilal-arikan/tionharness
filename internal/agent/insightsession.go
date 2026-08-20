package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/insight"
)

// newInsightRunID mints a scan run's stable identity, shared by the run log
// record and its session's SourceID.
func newInsightRunID() string { return "IRUN-" + uuid.NewString() }

// insightRunReport is everything the run session's transcript is rendered from.
// It carries only data the scan already produced — building the report never
// issues an LLM call.
type insightRunReport struct {
	RunID    string
	Trigger  string   // "manual" | "auto" | "agent" | "" (unknown)
	LensIDs  []string // the lenses this run actually resolved to
	AgentID  string   // analysis agent that served the run
	Duration time.Duration
	Result   insight.ScanResult
}

// insightRunSessionWorthy decides whether a run deserves its own transcript
// session. The hourly cron fires whether or not there is anything to look at, so
// a run that resolved no lens, or that did no work at all (nothing analysed,
// nothing found, nothing failed), would otherwise pile up identical empty
// transcripts. Such a run still gets its run-log row — that rollup is one cheap
// line and keeps "the cron did fire" observable.
//
// Ledger-skipped and prefiltered pairs count as no work on purpose: they cost no
// LLM call and produce nothing to read.
func insightRunSessionWorthy(rep insightRunReport) bool {
	if len(rep.LensIDs) == 0 {
		return false
	}
	res := rep.Result
	return res.Analyzed > 0 || res.Findings > 0 || len(res.Errors) > 0
}

// insightRunTitle names a scan session the way the sessions list reads it.
func insightRunTitle(lensCount, findings int) string {
	return fmt.Sprintf("İçgörü taraması — %d lens, %d bulgu", lensCount, findings)
}

// insightRunTranscript renders the run as one readable markdown turn: scope,
// coverage counters, produced findings and errors.
func insightRunTranscript(rep insightRunReport) string {
	var b strings.Builder
	res := rep.Result

	fmt.Fprintf(&b, "## İçgörü taraması\n\n")
	fmt.Fprintf(&b, "- **Run id:** `%s`\n", rep.RunID)
	if rep.Trigger != "" {
		fmt.Fprintf(&b, "- **Tetikleyici:** %s\n", rep.Trigger)
	}
	fmt.Fprintf(&b, "- **Süre:** %.1fs\n", rep.Duration.Seconds())
	if len(rep.LensIDs) > 0 {
		fmt.Fprintf(&b, "- **Lensler (%d):** %s\n", len(rep.LensIDs), strings.Join(rep.LensIDs, ", "))
	} else {
		b.WriteString("- **Lensler:** (yok)\n")
	}

	b.WriteString("\n### Kapsam\n\n")
	fmt.Fprintf(&b, "| Oturum | Analiz | Atlandı | Prefiltre | Bulgu | Hata |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|\n")
	fmt.Fprintf(&b, "| %d | %d | %d | %d | %d | %d |\n",
		res.Sessions, res.Analyzed, res.Skipped, res.Prefiltered, res.Findings, len(res.Errors))

	b.WriteString("\n### Bulgular\n\n")
	if len(res.Produced) == 0 {
		b.WriteString("Bu taramada yeni bulgu üretilmedi.\n")
	} else {
		for _, f := range res.Produced {
			sev := f.Severity
			if sev == "" {
				sev = "?"
			}
			fmt.Fprintf(&b, "- **[%s]** %s _(lens: %s, kanal: %s)_\n", sev, f.Title, f.LensID, f.Channel)
		}
	}

	if len(res.Errors) > 0 {
		b.WriteString("\n### Hatalar\n\n")
		for _, e := range res.Errors {
			fmt.Fprintf(&b, "- %s\n", e)
		}
	}
	return b.String()
}

// recordInsightSession opens the run's read-only transcript session and writes
// the report into it. It returns the new session id. Errors are returned (never
// swallowed) so the caller can log them and still write the run log.
func (r *Runtime) recordInsightSession(ctx context.Context, rep insightRunReport) (string, error) {
	if r == nil || r.db == nil {
		return "", fmt.Errorf("insight: runtime not ready")
	}
	sess, err := r.db.CreateSession(ctx, db.Session{
		AgentID:  rep.AgentID,
		Kind:     db.SessionKindInsight,
		SourceID: rep.RunID,
		Title:    insightRunTitle(len(rep.LensIDs), rep.Result.Findings),
	})
	if err != nil {
		return "", err
	}
	if _, err := r.db.AddMessage(ctx, db.Message{
		SessionID:  sess.ID,
		Role:       "assistant",
		AgentID:    rep.AgentID,
		AuthorKind: db.AuthorAgent,
		AuthorID:   rep.AgentID,
		Text:       insightRunTranscript(rep),
	}); err != nil {
		return sess.ID, err
	}
	return sess.ID, nil
}

// archiveOldInsightSessions keeps the newest `keep` scan sessions live and
// ARCHIVES the rest — the transcripts stay readable (same treatment as any other
// archived session), they just leave the live list. keep <= 0 means keep all.
//
// db.ListSessions returns newest-first (pinned first), so "the newest keep" is
// simply the first `keep` insight sessions in that order; a pinned one therefore
// survives, which is the intent of pinning.
func (r *Runtime) archiveOldInsightSessions(ctx context.Context, keep int) (int, error) {
	if keep <= 0 {
		return 0, nil
	}
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("insight: runtime not ready")
	}
	sessions, err := r.db.ListSessions(ctx, "")
	if err != nil {
		return 0, err
	}
	seen, archived := 0, 0
	for _, sess := range sessions {
		if sess.Kind != db.SessionKindInsight || sess.State == "archived" {
			continue
		}
		seen++
		if seen <= keep {
			continue
		}
		if err := r.db.SetSessionState(ctx, sess.ID, "archived"); err != nil {
			return archived, err
		}
		archived++
	}
	return archived, nil
}
