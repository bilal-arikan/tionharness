package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Hata→ders döngüsü (self-healing, external-context-agent background_review analogue).
// After a turn that ended badly, a background reflection call distills the
// failure into ONE short, generalizable lesson and persists it to the
// workspace-wide lessons store. Future turns (chat + headless) read the newest
// lessons back as a dynamic-context block, so the same failure shape is not
// repeated across sessions. The memory subsystem was removed (2026-07-05);
// lessons are its narrow, failure-focused successor.

// lessonsInjectCount bounds how many newest lessons ride each turn's dynamic
// context (a few high-signal lines, not a log dump).
const lessonsInjectCount = 5

// Lesson trust is derived at selection time; it is deliberately not persisted.
// Recurring reflector lessons lose trust, while insight lessons keep Count
// neutral because their Count measures repeated lens findings, not failed advice.
const (
	lessonTrustBase              = 1.0
	lessonTrustRepeatPenalty     = 0.10
	lessonTrustFreshBonus        = 0.05
	lessonTrustFreshAge          = 6 * time.Hour
	lessonInsightSignaturePrefix = "lesson:"
)

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
	go func() {
		defer func() {
			if p := recover(); p != nil {
				r.logger.Warn("lesson reflection panicked", "session", sessionID, "panic", p)
			}
		}()
		r.reflectLessons(bg, sessionID, evidence, turnLevel)
	}()
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
	lesson, err := r.db.AddLesson(db.Lesson{
		Time:      time.Now().Unix(),
		AgentID:   agent.ID,
		SessionID: sessionID,
		Tool:      tool,
		Signature: lessonSignature(tool, evidence, turnLevel),
		Text:      text,
	})
	if err != nil {
		r.logger.Warn("lesson persist failed", "session", sessionID, "error", err)
		return
	}
	r.logger.Info("lesson recorded", "session", sessionID, "tool", tool, "count", lesson.Count)
	r.emitDebug(ctx, db.DebugEvent{Type: db.DebugLesson, AgentID: agent.ID, Name: tool, Detail: truncateRunes(text, 200)})
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

// LessonsContextBlock renders the newest stored lessons as a dynamic-context
// block for chat + headless turns ("" when there are none or the feature is
// off). Lessons learned by THIS agent rank first (its own failure history is
// the most relevant), then the rest of the workspace's. Within each group,
// derived trust ranks lessons first and recency breaks equal scores. agentID
// may be "" (no prioritization). Volatile by nature
// (lessons accrue over time), so it must ride SystemDynamic — never the
// cached static prefix.
func (r *Runtime) LessonsContextBlock(ctx context.Context, agentID string) string {
	if r == nil || r.db == nil || !r.tun.LessonReflect() {
		return ""
	}
	// Overfetch so same-agent lessons beyond the newest-N window can still be
	// promoted into the injected set.
	all, err := r.db.ListLessons(lessonsInjectCount * 10)
	if err != nil || len(all) == 0 {
		return ""
	}
	matching := make([]db.Lesson, 0, len(all))
	remaining := make([]db.Lesson, 0, len(all))
	if agentID != "" {
		for _, l := range all {
			if l.AgentID == agentID {
				matching = append(matching, l)
			} else {
				remaining = append(remaining, l)
			}
		}
	} else {
		remaining = append(remaining, all...)
	}
	now := time.Now().Unix()
	sortLessonsByTrust(matching, now)
	sortLessonsByTrust(remaining, now)
	lessons := make([]db.Lesson, 0, lessonsInjectCount)
	lessons = appendLessonsUpTo(lessons, matching, lessonsInjectCount)
	lessons = appendLessonsUpTo(lessons, remaining, lessonsInjectCount)
	var b strings.Builder
	b.WriteString("## Lessons from past failures (auto-collected)\n")
	b.WriteString("Earlier turns failed in these ways; apply the lessons instead of repeating them:\n")
	for _, l := range lessons {
		line := strings.Join(strings.Fields(l.Text), " ")
		if l.Tool != "" {
			fmt.Fprintf(&b, "- [%s] %s", l.Tool, line)
		} else {
			fmt.Fprintf(&b, "- %s", line)
		}
		if l.Count > 1 {
			fmt.Fprintf(&b, " (seen %d times)", l.Count)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func lessonTrust(l db.Lesson, now int64) float64 {
	score := lessonTrustBase
	if !strings.HasPrefix(l.Signature, lessonInsightSignaturePrefix) && l.Count > 1 {
		score -= float64(l.Count-1) * lessonTrustRepeatPenalty
	}
	if age := now - l.Time; age >= 0 && age <= int64(lessonTrustFreshAge/time.Second) {
		score += lessonTrustFreshBonus
	}
	return score
}

// sortLessonsByTrust is stable, preserving ListLessons' recency order when
// scores tie. The candidate set is bounded by the small overfetch above.
func sortLessonsByTrust(lessons []db.Lesson, now int64) {
	for i := 1; i < len(lessons); i++ {
		for j := i; j > 0 && lessonTrust(lessons[j], now) > lessonTrust(lessons[j-1], now); j-- {
			lessons[j], lessons[j-1] = lessons[j-1], lessons[j]
		}
	}
}

func appendLessonsUpTo(dst, src []db.Lesson, limit int) []db.Lesson {
	remaining := limit - len(dst)
	if remaining <= 0 {
		return dst
	}
	if len(src) > remaining {
		src = src[:remaining]
	}
	return append(dst, src...)
}
