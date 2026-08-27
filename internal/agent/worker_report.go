package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/tools"
)

// worker_report.go bounds how much of a worker's output crosses back into the
// COORDINATOR session (_Docs/47, P0+P2). A worker's final turn text was injected
// verbatim into the coordinator as the <result> of its <task-notification>, with
// no length limit — the single largest, unbounded source of coordinator context
// growth. A chatty worker (or a verifier that pastes whole diffs and test logs)
// could fill the coordinator's window on one report, defeating the whole point of
// delegating: the coordinator is meant to stay thin and only route.
//
// Two guarantees live here:
//   - capText: a rune-safe hard cap on any single result string. Rune-safe, not
//     byte-safe, because a byte-offset slice splits a multi-byte UTF-8 rune and
//     panics on Turkish text (the contextdiff.go regression — see _Docs/57).
//   - buildWorkerResult: for a leaf worker whose output overflows the cap, the
//     FULL text is persisted as an artifact on the worker's own session and the
//     coordinator receives a compact head plus a handle (artifact id + worker
//     session id). Nothing is lost; the coordinator pulls the detail on demand
//     (open the artifact, or send_to_worker to ask the worker to elaborate)
//     instead of paying for it on every subsequent turn. This is the
//     "preview envelope + artifact handle" pattern applied to worker reports.
const coordinatorResultCapChars = 6000

// capText truncates s to at most maxRunes runes, appending a short notice of how
// many runes were dropped. Returns the (possibly truncated) text and whether a
// truncation happened. Rune-safe: never splits a multi-byte UTF-8 sequence.
func capText(s string, maxRunes int) (string, bool) {
	if maxRunes <= 0 {
		return s, false
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s, false
	}
	dropped := len(r) - maxRunes
	return string(r[:maxRunes]) + fmt.Sprintf("\n\n…[%d karakter kırpıldı]", dropped), true
}

// buildWorkerResult turns a leaf worker's raw final text into the string the
// coordinator will actually read. Under the cap it is returned unchanged. Over
// the cap, the full text is written as a text artifact on the worker session and
// the returned string is a capped head followed by a handle line pointing at both
// the artifact and the worker session, so the coordinator can retrieve the full
// output deliberately rather than receiving it inline.
//
// Best-effort on the artifact: if persisting it fails, the caller still gets the
// capped text (with the worker-session pointer), so a report is never dropped for
// want of an artifact.
func (r *Runtime) buildWorkerResult(ctx context.Context, sessionID, agentID, status, text string) string {
	capped, truncated := capText(text, coordinatorResultCapChars)
	if !truncated {
		return text
	}
	handle := fmt.Sprintf("worker oturumu %s", sessionID)
	sink := r.NewArtifactSink(sessionID, agentID)
	art, err := sink.CreateArtifact(ctx, tools.CreateArtifactSpec{
		Title:   fmt.Sprintf("Worker sonucu (%s) — tam çıktı", status),
		Kind:    "text",
		Content: text,
	})
	if err != nil {
		r.logger.Warn("worker: full-result artifact failed; coordinator gets capped text only",
			"session", sessionID, "error", err)
	} else {
		handle = fmt.Sprintf("artifact %s (%s)", art.ID, handle)
	}
	return capped + fmt.Sprintf(
		"\n\n📎 Tam çıktı context'e alınmadı — kaynağı: %s. "+
			"Gerekirse artifact'ı aç ya da `send_to_worker` ile bu worker'dan ayrıntı iste; hepsini buraya çekme.",
		strings.TrimSpace(handle))
}
