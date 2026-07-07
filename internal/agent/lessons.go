package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
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

// lessonEvidenceMax bounds how many failing steps feed one reflection prompt.
const lessonEvidenceMax = 3

// lessonSnippetRunes caps each evidence snippet (input/error) in the prompt.
const lessonSnippetRunes = 400

// lessonSystemPrompt keeps the reflector terse, honest and generalizable.
const lessonSystemPrompt = `You review a failed AI-agent turn and distill ONE reusable lesson for future turns.
Reply with 1-3 plain sentences: what failed, the likely root cause, and how to avoid it next time.
Generalize (a rule the agent can apply again), do not just restate the error.
If the failure is not generalizable (one-off cancellation, external outage, missing login), reply with exactly: NONE`

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
		return
	}
	agent, err := r.db.GetAgent(ctx, sess.AgentID)
	if err != nil {
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

	// A cheaper/faster model is preferable (same policy as utility summaries):
	// the title-model override when configured, else the agent's own model.
	model := agent.Model
	if override := r.tun.TitleModel(); override != "" {
		model = override
	}
	resp, err := r.guardedComplete(WithCallKind(ctx, KindReflect), agent, providers.Request{
		Model:     model,
		System:    lessonSystemPrompt,
		MaxTokens: 300,
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: b.String()},
		},
	}, false)
	if err != nil {
		r.logger.Warn("lesson reflection failed", "session", sessionID, "error", err)
		return
	}
	text := strings.TrimSpace(resp.Text)
	if text == "" || strings.EqualFold(text, "NONE") || len(text) > 1200 {
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

// lessonSignature builds the dedupe key for a failure shape: the failing tool
// plus a digest of the normalized error text, so the same recurring failure
// updates one lesson instead of accumulating near-duplicates.
func lessonSignature(tool string, evidence []lessonEvidence, turnLevel string) string {
	src := turnLevel
	if len(evidence) > 0 {
		src = evidence[0].errs
	}
	src = strings.ToLower(src)
	if len(src) > 120 {
		src = src[:120]
	}
	sum := sha256.Sum256([]byte(src))
	return tool + ":" + hex.EncodeToString(sum[:8])
}

// LessonsContextBlock renders the newest stored lessons as a dynamic-context
// block for chat + headless turns ("" when there are none or the feature is
// off). Volatile by nature (lessons accrue over time), so it must ride
// SystemDynamic — never the cached static prefix.
func (r *Runtime) LessonsContextBlock(ctx context.Context) string {
	if r == nil || r.db == nil || !r.tun.LessonReflect() {
		return ""
	}
	lessons, err := r.db.ListLessons(lessonsInjectCount)
	if err != nil || len(lessons) == 0 {
		return ""
	}
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
