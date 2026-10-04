package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/notes"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Hata→ders döngüsü (self-healing, external-context-agent background_review analogue).
// After a turn that ended badly, a background reflection call distills the
// failure into ONE short, generalizable lesson and files it as a workspace
// memory note (internal/notes, kind "lesson"). The session briefing serves the
// notes that reach a session, so the same failure shape is not repeated across
// sessions; note_search finds the rest (_Docs/94).

// lessonInsightSignaturePrefix marks lessons promoted from the insight scanner
// (lessons-mining lens) so the analyzer can tell them from reflector lessons.
const lessonInsightSignaturePrefix = "lesson:"

// lessonEvidenceMax bounds how many failing steps feed one reflection prompt.
const lessonEvidenceMax = 3

// lessonSnippetRunes caps each evidence snippet (input/error) in the prompt.
const lessonSnippetRunes = 400

// The reflector's system prompt lives in the central registry
// (internal/prompts, key "lesson"); it keeps the reflector terse, honest and
// generalizable, and readPrompt resolves the workspace override.

// lessonEvidence is one failing observation extracted from a turn's trace.
type lessonEvidence struct {
	tool  string
	input string
	errs  string
}

// maybeReflectLessons inspects a finished turn and, when it ended badly, fires
// a BACKGROUND reflection that writes a lesson to the workspace store. Gated by
// the LessonReflect setting. Fire-and-forget: it never blocks or fails the turn.
func (r *Runtime) maybeReflectLessons(ctx context.Context, sessionID string, steps []TurnStep, turnErr string) {
	if r == nil || r.db == nil || !r.tun.LessonReflect() || sessionID == "" {
		return
	}
	// Test fixtures suppress only the dispatch, not the setting (the reflection
	// launches a real background provider call). Always false in production.
	if r.skipLessonDispatch {
		return
	}
	// The stuck gate's own refusal and a user stop teach nothing.
	if strings.Contains(turnErr, stuckGuardMarker) || turnErr == "stopped" {
		return
	}
	evidence, turnLevel := collectLessonEvidence(steps, turnErr)
	if len(evidence) == 0 && turnLevel == "" {
		return
	}
	// Detach from the turn's (possibly cancelled) context but keep its values
	// (session id, call kind) for usage attribution and debug journaling.
	bg := context.WithoutCancel(ctx)
	// Register with the shutdown barrier (TSK759): reflectLessons writes a lesson to
	// the workspace store, and context.WithoutCancel means cancelAllSessions cannot
	// reach it. Unregistered, it kept writing while the workspace DB closed under it
	// — and in tests it outlived t.TempDir()'s RemoveAll, which Windows fails with
	// "directory not empty". A refusal means the workspace is already closing, so
	// there is nothing worth reflecting into.
	if !r.startBackgroundTurn(func() {
		defer func() {
			if p := recover(); p != nil {
				r.logger.Warn("lesson reflection panicked", "session", sessionID, "panic", p)
			}
		}()
		r.reflectLessons(bg, sessionID, evidence, turnLevel)
	}) {
		r.logger.Debug("lesson reflection skipped: workspace closing", "session", sessionID)
	}
}

// collectLessonEvidence pulls the reflectable failures out of a turn trace:
// real tool errors (policy denials and guardrail-blocked calls excluded — the
// guardrail already taught the model in-turn) plus the turn-level error.
func collectLessonEvidence(steps []TurnStep, turnErr string) (evidence []lessonEvidence, turnLevel string) {
	for _, st := range steps {
		if len(evidence) >= lessonEvidenceMax {
			break
		}
		if st.Kind != StepTool || !st.IsError || isPermissionDenyError(st) {
			continue
		}
		if strings.Contains(st.Output, "[loop guardrail]") || strings.HasPrefix(st.Output, "blocked by loop guardrail") {
			// Keep the raw failure but strip the appended guardrail coaching so
			// the reflector sees the error, not our own hint text.
			if i := strings.Index(st.Output, "\n\n[loop guardrail]"); i > 0 {
				st.Output = st.Output[:i]
			} else {
				continue
			}
		}
		evidence = append(evidence, lessonEvidence{
			tool:  st.Tool,
			input: truncateRunes(strings.Join(strings.Fields(string(st.Input)), " "), lessonSnippetRunes),
			errs:  truncateRunes(strings.Join(strings.Fields(st.Output), " "), lessonSnippetRunes),
		})
	}
	if e := strings.TrimSpace(turnErr); e != "" {
		turnLevel = truncateRunes(e, lessonSnippetRunes)
	}
	return evidence, turnLevel
}

// reflectLessons runs the reflection call and persists the resulting lesson.
// Runs on a background goroutine; every failure is logged, never propagated.
func (r *Runtime) reflectLessons(ctx context.Context, sessionID string, evidence []lessonEvidence, turnLevel string) {
	sess, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		r.logger.Warn("lesson session resolve failed", "session", sessionID, "error", err)
		return
	}
	system, err := r.isSystemAgentSession(ctx, sess)
	if err != nil {
		r.logger.Warn("lesson agent resolve failed", "session", sessionID, "error", err)
		return
	}
	if system {
		r.logger.Info("lesson reflection skipped for system-agent session", "session", sessionID, "agent", sess.AgentID)
		return
	}
	agent, err := r.db.GetAgent(ctx, sess.AgentID)
	if err != nil {
		r.logger.Warn("lesson agent load failed", "session", sessionID, "error", err)
		return
	}

	var b strings.Builder
	b.WriteString("A turn of this agent just failed. Evidence:\n")
	for _, ev := range evidence {
		fmt.Fprintf(&b, "\n- tool %s\n  input: %s\n  error: %s\n", ev.tool, ev.input, ev.errs)
	}
	if turnLevel != "" {
		fmt.Fprintf(&b, "\n- turn-level error: %s\n", turnLevel)
	}

	agentCfg, lessonPrompt, err := r.resolveLessonConfig(agent)
	if err != nil {
		r.logger.Warn("lesson system agent resolution failed", "session", sessionID, "error", err)
		return
	}
	resp, err := r.guardedComplete(WithPromptTrace(WithCallKind(ctx, KindReflect), "lesson", lessonPrompt), agentCfg, providers.Request{
		Model:     agentCfg.Model,
		System:    lessonPrompt,
		MaxTokens: 300,
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: b.String()},
		},
	}, false)
	if err != nil {
		r.logger.Warn("lesson reflection failed", "session", sessionID, "error", err)
		return
	}
	text := cleanLessonText(resp.Text)
	if text == "" || len(text) > 1200 {
		// Journal the deliberate skip so "reflection ran but stored nothing" is
		// distinguishable from "reflection never ran" in debug.jsonl.
		r.emitDebug(ctx, db.DebugEvent{Type: db.DebugLesson, AgentID: agent.ID, Name: "none", Detail: truncateRunes(text, 80)})
		return
	}

	tool := ""
	if len(evidence) > 0 {
		tool = evidence[0].tool
	}
	if r.notes == nil {
		r.logger.Warn("lesson not persisted: notes store unavailable", "session", sessionID)
		return
	}
	// The lesson lands in the workspace memory (_Docs/94) as a workspace-scoped
	// inferred note; its failure-shape signature dedupes repeats (Occurrences++).
	var tags []string
	if tool != "" {
		tags = []string{tool}
	}
	note, err := r.notes.Put(notes.Note{
		Kind:          notes.KindLesson,
		Title:         lessonTitle(text),
		Body:          text,
		Scope:         notes.ScopeWorkspace,
		Confidence:    notes.ConfidenceInferred,
		Source:        notes.SourceLessonExtractor,
		SourceSession: sessionID,
		SourceAgent:   agent.ID,
		Signature:     lessonSignature(tool, evidence, turnLevel),
		Tags:          tags,
	})
	if err != nil {
		r.logger.Warn("lesson persist failed", "session", sessionID, "error", err)
		return
	}
	r.logger.Info("lesson recorded", "session", sessionID, "tool", tool, "note", note.ID, "count", note.Occurrences)
	r.emitDebug(ctx, db.DebugEvent{Type: db.DebugLesson, AgentID: agent.ID, Name: tool, Detail: truncateRunes(text, 200)})
	r.publish(events.Event{Type: events.TypeNotes, Level: "info", Title: "Yeni ders notu", Body: note.Title, Target: map[string]string{"view": "notes", "noteId": note.ID}})
}

// lessonTitle derives a note title from the reflector's text: its first
// sentence, cut at a word boundary to a searchable length (no ellipsis — a
// title is a wikilink target and a search key, not a teaser).
func lessonTitle(text string) string {
	t := strings.Trim(notes.FirstSentence(text, 0), ".!? ")
	const maxRunes = 90
	if r := []rune(t); len(r) > maxRunes {
		cut := string(r[:maxRunes])
		if i := strings.LastIndex(cut, " "); i > maxRunes/2 {
			cut = cut[:i]
		}
		t = strings.TrimRight(cut, " ,;:-")
	}
	if t == "" {
		t = "Lesson from a failed turn"
	}
	return strings.ReplaceAll(strings.ReplaceAll(t, "[", "("), "]", ")")
}

// cleanLessonText normalizes the reflector's reply: a model may open with
// "NONE" and then reconsider mid-answer (seen live in the E2E), so leading
// NONE/empty lines are stripped; a reply that is nothing but NONE — the
// deliberate "not generalizable" verdict — collapses to "".
func cleanLessonText(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	start := 0
	for start < len(lines) {
		l := strings.TrimSpace(lines[start])
		if l == "" || strings.EqualFold(l, "NONE") {
			start++
			continue
		}
		break
	}
	return strings.TrimSpace(strings.Join(lines[start:], "\n"))
}

// lessonSignature builds the dedupe key for a failure shape: the failing tool
// plus a digest of the NORMALIZED error text, so the same recurring failure
// updates one lesson instead of accumulating near-duplicates.
func lessonSignature(tool string, evidence []lessonEvidence, turnLevel string) string {
	src := turnLevel
	if len(evidence) > 0 {
		src = evidence[0].errs
	}
	src = normalizeErrSig(src)
	if len(src) > 120 {
		src = src[:120]
	}
	sum := sha256.Sum256([]byte(src))
	return tool + ":" + hex.EncodeToString(sum[:8])
}

// normalizeErrSig collapses the VARIABLE parts of an error message — paths,
// numbers, ids — so two occurrences of the same failure shape hash identically
// even when the concrete file, line number or token count differs (e.g.
// "no such file: C:\a\b.txt" and "no such file: /tmp/c.txt" are one shape).
func normalizeErrSig(s string) string {
	s = strings.ToLower(s)
	fields := strings.Fields(s)
	for i, f := range fields {
		// A token containing a path separator is a path → collapse entirely.
		if strings.ContainsAny(f, `/\`) {
			fields[i] = "<path>"
			continue
		}
		// Digit runs (line numbers, token counts, ports, ids) → '#'.
		fields[i] = digitRunRe.ReplaceAllString(f, "#")
	}
	return strings.Join(fields, " ")
}

// digitRunRe matches runs of digits for signature normalization.
var digitRunRe = regexp.MustCompile(`\d+`)
