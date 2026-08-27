package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// SessionInput is everything the session projection reads.
//
// Messages is the transcript tail, NOT the whole session: the projection derives
// the todo checklist, the last error and the recent turns from it. The loader
// decides how much tail to hand over (see Projector.loadSession) so a 10k-message
// session costs the same as a fresh one.
type SessionInput struct {
	Session db.Session
	// Messages is the tail of the transcript, oldest first.
	Messages []db.Message
	// TailFrom is the index the tail starts at within the full transcript, so the
	// projection can report how much it did not look at.
	TailFrom int
	// WaitingAsk is set when the session is durably suspended on a question.
	WaitingAsk *db.SessionAsk
	// Now is the clock used for age computations. Zero means time.Now().
	Now time.Time
}

const (
	// sessionTailMessages is how many trailing messages the loader reads. Enough
	// to find the current checklist and the last failure without touching the
	// whole transcript.
	sessionTailMessages = 40
	sessionRecentTurns  = 3
)

// ProjectSession renders a session: what it is working on, how far the checklist
// got, what it cost, and whether it is in trouble.
func ProjectSession(in SessionInput, level Level) (View, error) {
	if in.Session.ID == "" {
		return View{}, fmt.Errorf("session input has no session")
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	v := View{
		Ref:    Ref{Kind: KindSession, ID: in.Session.ID},
		Level:  level,
		AsOf:   now,
		Source: fmt.Sprintf("%d@%d", in.Session.MessageCount, in.Session.UpdatedAt),
	}
	v.Header = sessionHeader(in, now)

	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	var l lines
	if s := strings.TrimSpace(in.Session.Summary); s != "" {
		l.add("özet: %s", clip(s, summaryWidth(level)))
	}
	if todo := sessionTodoLine(in.Messages); todo != "" {
		l.add("%s", todo)
	}
	for _, s := range sessionSignals(in, now) {
		l.add("%s", s)
	}

	if level == LevelFull {
		if detail := sessionRecent(in.Messages, now); detail != "" {
			l.add("--")
			l.add("%s", detail)
		}
	}
	// Everything before the tail was never read; say so rather than letting the
	// reader assume the whole transcript was considered.
	v.Elided, v.ElidedUnit = in.TailFrom, "eski mesaj"
	v.Body = l.String()
	v.Handles = sessionHandles(in, level)
	v.finalize()
	return v, nil
}

// summaryWidth grows the rolling summary allowance with the budget tier.
func summaryWidth(level Level) int {
	if level == LevelFull {
		return 1200
	}
	return 300
}

// sessionHeader is the always-present identity + state line. Token/cost figures
// are deliberately not part of it — spend belongs to the budget view.
func sessionHeader(in SessionInput, now time.Time) string {
	s := in.Session
	title := s.Title
	if title == "" {
		title = "(başlıksız)"
	}

	head := fmt.Sprintf("SES:%s %q · %d msg · %s · agent:%s",
		s.ID, clip(title, 60), s.MessageCount,
		age(tsSec(s.CreatedAt), now)+" önce açıldı", orDash(s.AgentID))

	if s.Kind != "" && s.Kind != "chat" {
		head += " · " + s.Kind
	}
	// Coordination lineage changes how a reader should interpret everything else
	// (a worker's "stuck" is its coordinator's problem too), so it belongs in the
	// header rather than buried among the signals.
	switch {
	case s.IsCoordinator() && s.IsWorker():
		head += fmt.Sprintf("\n  ⇅ mid-level coordinator (depth %d, root:%s)", s.CoordinatorDepth, s.RootCoordinator())
	case s.IsCoordinator():
		head += "\n  ⇵ coordinator"
	case s.IsWorker():
		head += "\n  ↑ worker of session:" + s.CoordinatorSessionID
	}
	return head
}

// sessionSignals is the L1 layer for a session.
func sessionSignals(in SessionInput, now time.Time) []string {
	var out []string
	s := in.Session

	if s.StuckTurns > 0 {
		out = append(out, fmt.Sprintf("⚠ StuckTurns %d — ardışık başarısız tur", s.StuckTurns))
	}
	if in.WaitingAsk != nil {
		out = append(out, fmt.Sprintf("⏸ cevap bekleyen soru (%s, %s'dir bekliyor) — ask:%s",
			orDash(in.WaitingAsk.Kind), age(tsSec(in.WaitingAsk.CreatedAt), now), in.WaitingAsk.ID))
	}
	if err := lastErrorStep(in.Messages); err != "" {
		out = append(out, "✗ son hata: "+clip(err, 180))
	}
	if tags := signalTags(s.Tags); len(tags) > 0 {
		out = append(out, "🏷 "+strings.Join(tags, ", "))
	}
	if s.ParentSessionID != "" {
		out = append(out, "↩ handoff ile session:"+s.ParentSessionID+"'den devam ediyor")
	}
	if s.Summary != "" && s.SummaryMsgCount > 0 {
		out = append(out, fmt.Sprintf("⤺ ilk %d mesaj özete katlandı (compaction)", s.SummaryMsgCount))
	}
	if s.UpdatedAt > 0 {
		out = append(out, fmt.Sprintf("son hareket: %s önce", age(tsSec(s.UpdatedAt), now)))
	}
	return out
}

// signalTags keeps only the tags that mean something is wrong. Organisational
// tags are the user's business, not a health signal.
func signalTags(tags []string) []string {
	interesting := map[string]bool{
		"stuck": true, "error": true, "tool-error": true, "auth-error": true,
		"goal": true, "archived": true,
	}
	var out []string
	for _, t := range tags {
		if interesting[t] {
			out = append(out, t)
		}
	}
	return out
}

// sessionTodoLine renders the most recent checklist as "7/11 tamam" plus what is
// in flight — usually the single most informative line about a working session.
// The scan itself is shared (LatestTodos); only this one-line Turkish shape is
// local, because the other consumers are English prompt text.
func sessionTodoLine(msgs []db.Message) string {
	r := LatestTodos(msgs)
	if r.Empty() {
		return ""
	}
	line := fmt.Sprintf("todo: %d/%d tamam", r.Done, len(r.Items))
	if r.Active != "" {
		line += " · şu an: " + clip(r.Active, 70)
	}
	return line
}

// lastErrorStep returns the newest failure in the tail — an error step or a tool
// call that came back as an error.
func lastErrorStep(msgs []db.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		steps := DecodeSteps(msgs[i].Steps)
		for j := len(steps) - 1; j >= 0; j-- {
			s := steps[j]
			if s.Kind == "error" {
				return orDash(s.Reason) + ": " + s.Text
			}
			if s.Kind == "tool" && s.IsError {
				return s.Tool + ": " + s.Output
			}
		}
	}
	return ""
}

// sessionRecent is the LevelFull tail: the last few turns, one line each.
func sessionRecent(msgs []db.Message, now time.Time) string {
	if len(msgs) == 0 {
		return ""
	}
	start := len(msgs) - sessionRecentTurns*2 // a turn is roughly a user + reply pair
	if start < 0 {
		start = 0
	}
	var l lines
	for _, m := range msgs[start:] {
		text := m.Text
		if text == "" {
			text = fmt.Sprintf("(%d adım)", len(DecodeSteps(m.Steps)))
		}
		l.add("%-9s %s", m.Role, clip(text, 160))
	}
	return l.String()
}

// sessionHandles points at the parts the projection left out.
func sessionHandles(in SessionInput, level Level) []Handle {
	var hs []Handle
	if level != LevelFull {
		hs = append(hs, Handle{
			Label: "son turlar + tam özet",
			Ref:   Ref{Kind: KindSession, ID: in.Session.ID},
			Level: LevelFull,
		})
	}
	return hs
}

// compactCount renders a large number as 187k / 2.4M so a header line stays one
// line.
func compactCount(n int64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 1_000_000:
		return fmt.Sprintf("%dk", n/1000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
}

// orDash renders an empty string as "-" so a column never collapses.
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
