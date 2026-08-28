package view

import (
	"fmt"
	"strings"
	"time"
)

// insightEvidenceShown is how many evidence session ids one finding spells out
// before the rest collapse into a "+N" tail.
const insightEvidenceShown = 6

// InsightInput is one insight finding (a retrospective scan result). The
// finding is the view-local shape (InsightFinding) — the api layer adapts
// insight.Findings to it, keeping this package a leaf.
type InsightInput struct {
	Finding InsightFinding
	// Now is the clock used for age stamps. Zero means time.Now().
	Now time.Time
}

// ProjectInsight renders one finding: severity, lifecycle status, recurrence and
// evidence — enough to decide whether the proposed fix is worth acting on.
func ProjectInsight(in InsightInput, level Level) (View, error) {
	f := in.Finding
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	if f.ID == "" {
		return View{}, fmt.Errorf("view: insight has no id")
	}

	v := View{
		Ref:    Ref{Kind: KindInsight, ID: f.ID},
		Level:  level,
		AsOf:   now,
		Source: fmt.Sprintf("%s/%d/%d", f.Status, f.Occurrences, f.LastSeen),
	}

	sev := orDash(f.Severity)
	if sev == "" {
		sev = "?"
	}
	title := clip(orDash(f.Title), 60)
	if title == "?" {
		title = "(başlıksız)"
	}
	v.Header = fmt.Sprintf("INSIGHT · %s · [%s] %s · asOf %s", f.ID, sev, title, hhmmss(now))

	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	var l lines
	status := orDash(f.Status)
	if f.Regressed {
		status = "regressed"
	}
	var meta []string
	meta = append(meta, "durum: "+status)
	if f.Channel != "" {
		meta = append(meta, "kanal: "+f.Channel)
	}
	if f.LensID != "" {
		meta = append(meta, "lens: "+f.LensID)
	}
	meta = append(meta, fmt.Sprintf("%d oluşum", f.Occurrences))
	if f.LastSeen > 0 {
		meta = append(meta, "son "+age(tsSec(f.LastSeen), now)+" önce")
	}
	l.add("%s", strings.Join(meta, " · "))

	// A recurring finding accumulates one evidence id per occurrence, so this list
	// grows without bound. Capped through clipList, which appends the "+N"
	// remainder rather than implying it named every session.
	if len(f.EvidenceSessionIDs) > 0 {
		l.add("kanıt: %s", strings.Join(clipList(f.EvidenceSessionIDs, insightEvidenceShown), ", "))
	}
	if f.RootCause != "" {
		l.add("kök neden: %s", clip(f.RootCause, 100))
	}
	if f.ProposedFix != "" {
		l.add("öneri: %s", clip(f.ProposedFix, 100))
	}

	v.Body = l.String()
	v.finalize()
	return v, nil
}
