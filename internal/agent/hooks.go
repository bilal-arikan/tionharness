package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/bilal-arikan/tionswarm/internal/proc"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// Hook execution policy. Hooks run arbitrary shell commands, so they are bounded
// by a timeout and an output cap, and a hook crash/timeout fails open (the call
// proceeds) — a misconfigured hook must never wedge an agent turn.
const (
	defaultHookTimeoutSec = 30
	maxHookTimeoutSec     = 120
	hookOutputCap         = 64 * 1024
)

// hookPayload is the JSON written to a hook command's stdin. It mirrors Claude
// Code's hook contract so the same external scripts (e.g. sqz, caveman) work
// unchanged — both the tool-call fields (ToolName/ToolInput/ToolResponse) and
// the lifecycle fields (Prompt/Source/Reason/Trigger/StopHookActive).
type hookPayload struct {
	SessionID     string          `json:"session_id,omitempty"`
	Cwd           string          `json:"cwd,omitempty"`
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name,omitempty"`
	ToolInput     json.RawMessage `json:"tool_input,omitempty"`
	ToolResponse  *hookToolResp   `json:"tool_response,omitempty"`

	// Lifecycle-event fields (Claude Code parity):
	Prompt         string `json:"prompt,omitempty"`           // UserPromptSubmit: the submitted user prompt
	Source         string `json:"source,omitempty"`           // SessionStart: startup|resume|clear
	Trigger        string `json:"trigger,omitempty"`          // PreCompact: manual|auto ; SessionEnd: reason
	Message        string `json:"message,omitempty"`          // Notification: the notification text
	StopHookActive bool   `json:"stop_hook_active,omitempty"` // Stop/SubagentStop: already in a hook-forced continuation
}

type hookToolResp struct {
	Content string `json:"content"`
	IsError bool   `json:"isError"`
}

// hookDecision is the JSON a hook command may emit on stdout. Every field is
// optional: an empty stdout (and exit 0) means "allow, no change".
type hookDecision struct {
	Decision           string              `json:"decision"` // "block" | "approve" | ""
	Reason             string              `json:"reason"`
	HookSpecificOutput *hookSpecificOutput `json:"hookSpecificOutput"`
	UpdatedInput       json.RawMessage     `json:"updatedInput"`  // PreToolUse: replace tool_input
	UpdatedOutput      *string             `json:"updatedOutput"` // PostToolUse: replace tool output
	AdditionalContext  string              `json:"additionalContext"`

	// rawStdout holds a hook's plain (non-JSON) stdout — see the note on
	// hookSpecificOutput. Populated by execHook, consumed only by
	// RunLifecycleHooks for the two context-injecting events. Not serialised.
	rawStdout string
}

type hookSpecificOutput struct {
	PermissionDecision       string `json:"permissionDecision"` // allow | deny | ask
	PermissionDecisionReason string `json:"permissionDecisionReason"`
	// AdditionalContext is the Claude Code channel by which SessionStart /
	// UserPromptSubmit hooks inject text into the model's context. We accept it
	// here AND at the top level (dec.AdditionalContext) so scripts written either
	// way work.
	AdditionalContext string `json:"additionalContext"`
}

// rawStdout is set (on hookDecision, not serialised) when a hook exits cleanly
// with NON-JSON stdout. Claude Code treats a SessionStart / UserPromptSubmit
// hook's plain stdout AS injected context — many real hooks (e.g. caveman's
// activate script) print the ruleset as plain text rather than JSON. Tool hooks
// ignore this field, so their non-JSON stdout stays "allow, no change".

// preHookOutcome is the aggregate of every matching PreToolUse hook for a call.
type preHookOutcome struct {
	block     bool            // a hook denied the call
	denyMsg   string          // message fed back to the model
	input     json.RawMessage // replaced tool_input, nil = unchanged
	autoAllow bool            // a hook approved → bypass the permission gate
	steps     []TurnStep      // audit cards to record/emit
}

// postHookOutcome is the aggregate of every matching PostToolUse hook for a call.
type postHookOutcome struct {
	block   bool
	denyMsg string
	output  *string // replaced tool output, nil = unchanged
	extra   string  // additionalContext to fold back to the model
	steps   []TurnStep
}

// hookMatches reports whether a hook's matcher applies to a tool name. An empty
// matcher matches every tool; otherwise it is one or more comma-separated
// shell-style globs (*, ?) — the hook matches if ANY alternative matches. The
// comma form exists because Go's filepath.Match has no brace expansion, yet a
// single hook often needs to cover sibling tools (e.g. "Bash,PowerShell" after
// the shell tool was split into two on Windows/POSIX).
func hookMatches(matcher, tool string) bool {
	if matcher == "" {
		return true
	}
	for _, alt := range strings.Split(matcher, ",") {
		alt = strings.TrimSpace(alt)
		if alt == "" {
			continue
		}
		if ok, err := filepath.Match(alt, tool); err == nil && ok {
			return true
		}
	}
	return false
}

// runPreToolHooks runs every enabled PreToolUse hook matching call.Name, in
// creation order, threading any input rewrite through the chain. The first hook
// to block wins (later hooks are skipped). Hook errors fail open.
func (r *Runtime) runPreToolHooks(ctx context.Context, sessionID string, call providers.ToolCall) preHookOutcome {
	var out preHookOutcome
	hooks, err := r.db.ListEnabledHooksByEvent(ctx, db.HookPreToolUse)
	if err != nil || len(hooks) == 0 {
		return out
	}
	input := call.Input
	for _, h := range hooks {
		if !hookMatches(h.Matcher, call.Name) {
			continue
		}
		payload := hookPayload{
			SessionID:     sessionID,
			Cwd:           r.workDir,
			HookEventName: db.HookPreToolUse,
			ToolName:      call.Name,
			ToolInput:     input,
		}
		hookStart := time.Now()
		dec, derr := r.execHook(ctx, h, payload)
		if derr != nil {
			// FAIL-OPEN, LOUDLY: a broken hook must not disable the tool it matches.
			// The error rides the debug journal with its message (not just an ":error"
			// tag) so a mis-authored command is diagnosable without re-running the
			// turn — a silent skip is exactly how a dialect-mismatched hook stayed
			// invisible while blocking every Bash call.
			r.logger.Warn("pre hook failed (fail-open, tool allowed)", "hook", h.ID, "tool", call.Name, "error", derr)
			r.emitDebug(ctx, db.DebugEvent{Type: db.DebugHook, Name: db.HookPreToolUse, HookID: h.ID, DurMs: time.Since(hookStart).Milliseconds(), Detail: call.Name + ":error:" + derr.Error(), Err: true})
			continue
		}
		r.emitDebug(ctx, db.DebugEvent{Type: db.DebugHook, Name: db.HookPreToolUse, HookID: h.ID, DurMs: time.Since(hookStart).Milliseconds(), Detail: call.Name + ":" + hookDecisionLabel(dec)})
		if len(dec.UpdatedInput) > 0 {
			input = dec.UpdatedInput
			out.input = dec.UpdatedInput
			out.steps = append(out.steps, hookStep(h, call.Name, "hook_modify", "Girdi hook tarafından değiştirildi", dec.Reason, false))
		}
		perm := ""
		if dec.HookSpecificOutput != nil {
			perm = strings.ToLower(dec.HookSpecificOutput.PermissionDecision)
		}
		if dec.Decision == "block" || perm == "deny" {
			out.block = true
			out.denyMsg = firstNonEmpty(dec.Reason, "hook denied the tool call")
			out.steps = append(out.steps, hookStep(h, call.Name, "hook_block", "Araç çağrısı hook tarafından engellendi", out.denyMsg, true))
			return out
		}
		if dec.Decision == "approve" || perm == "allow" {
			out.autoAllow = true
			out.steps = append(out.steps, hookStep(h, call.Name, "hook_allow", "Araç çağrısı hook tarafından otomatik onaylandı", dec.Reason, false))
		}
	}
	return out
}

// runPostToolHooks runs every enabled PostToolUse hook matching call.Name, in
// creation order, threading any output rewrite through the chain. The first hook
// to block wins. Hook errors fail open.
func (r *Runtime) runPostToolHooks(ctx context.Context, sessionID string, call providers.ToolCall, res providers.ToolResult) postHookOutcome {
	var out postHookOutcome
	hooks, err := r.db.ListEnabledHooksByEvent(ctx, db.HookPostToolUse)
	if err != nil || len(hooks) == 0 {
		return out
	}
	content := res.Content
	for _, h := range hooks {
		if !hookMatches(h.Matcher, call.Name) {
			continue
		}
		payload := hookPayload{
			SessionID:     sessionID,
			Cwd:           r.workDir,
			HookEventName: db.HookPostToolUse,
			ToolName:      call.Name,
			ToolInput:     call.Input,
			ToolResponse:  &hookToolResp{Content: content, IsError: res.IsError},
		}
		hookStart := time.Now()
		dec, derr := r.execHook(ctx, h, payload)
		if derr != nil {
			r.logger.Warn("post hook failed (fail-open)", "hook", h.ID, "tool", call.Name, "error", derr)
			r.emitDebug(ctx, db.DebugEvent{Type: db.DebugHook, Name: db.HookPostToolUse, HookID: h.ID, DurMs: time.Since(hookStart).Milliseconds(), Detail: call.Name + ":error", Err: true})
			continue
		}
		r.emitDebug(ctx, db.DebugEvent{Type: db.DebugHook, Name: db.HookPostToolUse, HookID: h.ID, DurMs: time.Since(hookStart).Milliseconds(), Detail: call.Name + ":" + hookDecisionLabel(dec)})
		if dec.UpdatedOutput != nil {
			content = *dec.UpdatedOutput
			out.output = dec.UpdatedOutput
			out.steps = append(out.steps, hookStep(h, call.Name, "hook_modify", "Çıktı hook tarafından dönüştürüldü", dec.Reason, false))
		}
		if dec.AdditionalContext != "" {
			out.extra = strings.TrimSpace(out.extra + "\n" + dec.AdditionalContext)
			out.steps = append(out.steps, hookStep(h, call.Name, "hook_context", "Hook ek bağlam ekledi", dec.AdditionalContext, false))
		}
		if dec.Decision == "block" {
			out.block = true
			out.denyMsg = firstNonEmpty(dec.Reason, "hook blocked the tool result")
			out.steps = append(out.steps, hookStep(h, call.Name, "hook_block", "Araç sonucu hook tarafından engellendi", out.denyMsg, true))
			return out
		}
	}
	return out
}

// LifecycleExtras carries the event-specific fields for a lifecycle hook run.
// Only the fields relevant to Event are populated by the caller.
type LifecycleExtras struct {
	Prompt         string // UserPromptSubmit
	Source         string // SessionStart (startup|resume|clear)
	Trigger        string // PreCompact (manual|auto) / SessionEnd (reason)
	Message        string // Notification
	StopHookActive bool   // Stop / SubagentStop
}

// selector returns the string a lifecycle hook's matcher is tested against for
// this event (Claude Code semantics): SessionStart matches on source, PreCompact
// on trigger; the rest have no selector (an empty matcher matches all).
func (e LifecycleExtras) selector(event string) string {
	switch event {
	case db.HookSessionStart:
		return e.Source
	case db.HookPreCompact:
		return e.Trigger
	}
	return ""
}

// LifecycleOutcome aggregates the effect of every matching lifecycle hook for one
// event: injected context, a block decision, and audit cards. Context is the
// concatenation of every hook's additionalContext (folded into the turn's dynamic
// system prompt by the caller). Block short-circuits the turn (UserPromptSubmit)
// or forces continuation (Stop) depending on the event.
type LifecycleOutcome struct {
	Context string
	Block   bool
	Reason  string
	Steps   []TurnStep
}

// RunLifecycleHooks runs every enabled hook for a turn/session lifecycle event,
// in creation order, aggregating their injected context and honouring a block
// decision. Unlike the tool hooks it is EXPORTED because it fires from the API
// turn orchestrator (chat_stream / subagent / compaction), not the native tool
// loop. Hook errors fail open. Matching: an empty matcher matches every
// invocation; a non-empty matcher is tested against the event's selector
// (SessionStart→source, PreCompact→trigger).
func (r *Runtime) RunLifecycleHooks(ctx context.Context, sessionID, event string, extras LifecycleExtras) LifecycleOutcome {
	var out LifecycleOutcome
	if !db.IsLifecycleEvent(event) {
		return out
	}
	hooks, err := r.db.ListEnabledHooksByEvent(ctx, event)
	if err != nil || len(hooks) == 0 {
		return out
	}
	sel := extras.selector(event)
	for _, h := range hooks {
		if !hookMatches(h.Matcher, sel) {
			continue
		}
		payload := hookPayload{
			SessionID:      sessionID,
			Cwd:            r.workDir,
			HookEventName:  event,
			Prompt:         extras.Prompt,
			Source:         extras.Source,
			Trigger:        extras.Trigger,
			Message:        extras.Message,
			StopHookActive: extras.StopHookActive,
		}
		hookStart := time.Now()
		dec, derr := r.execHook(ctx, h, payload)
		if derr != nil {
			r.logger.Warn("lifecycle hook failed (fail-open)", "hook", h.ID, "event", event, "error", derr)
			r.emitDebug(ctx, db.DebugEvent{Type: db.DebugHook, Name: event, HookID: h.ID, DurMs: time.Since(hookStart).Milliseconds(), Detail: "error", Err: true})
			continue
		}
		r.emitDebug(ctx, db.DebugEvent{Type: db.DebugHook, Name: event, HookID: h.ID, DurMs: time.Since(hookStart).Milliseconds(), Detail: hookDecisionLabel(dec)})
		// Injected context: accept both the top-level and hookSpecificOutput channels.
		add := strings.TrimSpace(dec.AdditionalContext)
		if dec.HookSpecificOutput != nil && strings.TrimSpace(dec.HookSpecificOutput.AdditionalContext) != "" {
			add = strings.TrimSpace(add + "\n" + strings.TrimSpace(dec.HookSpecificOutput.AdditionalContext))
		}
		// Claude Code parity: a SessionStart / UserPromptSubmit hook's plain (non-JSON)
		// stdout IS injected context. Only these two events treat stdout this way.
		if add == "" && dec.rawStdout != "" && (event == db.HookSessionStart || event == db.HookUserPromptSubmit) {
			add = strings.TrimSpace(dec.rawStdout)
		}
		if add != "" {
			out.Context = strings.TrimSpace(out.Context + "\n\n" + add)
			out.Steps = append(out.Steps, hookStep(h, event, "hook_context", "Hook ek bağlam ekledi", add, false))
		}
		if dec.Decision == "block" {
			out.Block = true
			out.Reason = firstNonEmpty(dec.Reason, out.Reason, "hook blocked the turn")
			out.Steps = append(out.Steps, hookStep(h, event, "hook_block", "Tur hook tarafından engellendi", out.Reason, true))
			return out
		}
	}
	return out
}

// execHook runs a single hook command, writing the payload to stdin and parsing
// the JSON decision from stdout. Exit code 2 means "block" (stderr is the
// reason), matching the Claude Code convention. Output is capped and the run is
// bounded by the hook's timeout (clamped).
func (r *Runtime) execHook(ctx context.Context, h db.Hook, payload hookPayload) (hookDecision, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return hookDecision{}, err
	}
	timeout := h.TimeoutSec
	if timeout <= 0 {
		timeout = defaultHookTimeoutSec
	}
	if timeout > maxHookTimeoutSec {
		timeout = maxHookTimeoutSec
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = proc.CommandContext(runCtx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", h.Command)
	} else {
		cmd = proc.CommandContext(runCtx, "/bin/sh", "-c", h.Command)
	}
	// A hook is a shell line: killing the shell alone on timeout orphans whatever
	// it launched, and any survivor keeps the output pipes open so the run below
	// never completes. Reap the whole tree.
	proc.TreeKill(cmd)
	if r.workDir != "" {
		cmd.Dir = r.workDir
	}
	cmd.Stdin = bytes.NewReader(data)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	out := stdout.Bytes()
	if len(out) > hookOutputCap {
		out = out[:hookOutputCap]
	}

	// The INTERPRETER failing to parse or launch the command is checked before any
	// exit-code interpretation, because the two are otherwise indistinguishable:
	// POSIX sh/bash exit 2 on a syntax error — the very code the Claude Code
	// contract reserves for a deliberate "block" — so a hook authored in the wrong
	// dialect looked exactly like a deny and silently blocked its matched tool on
	// every call for the rest of the session. A parse failure is an ERROR, not a
	// decision: it fails open (the caller logs it and lets the call through) while a
	// real deny still blocks. The message names the fault and the expected dialect,
	// so debug.jsonl is diagnosable without re-running the turn.
	if runErr != nil {
		if msg := interpreterFailure(stderr.String()); msg != "" {
			return hookDecision{}, fmt.Errorf("hook interpreter failed (command is not valid %s syntax): %s", hookShellName(), msg)
		}
	}

	// Exit code 2 = block, with stderr carrying the reason (Claude Code contract).
	if ee, ok := runErr.(*exec.ExitError); ok && ee.ExitCode() == 2 {
		return hookDecision{Decision: "block", Reason: strings.TrimSpace(stderr.String())}, nil
	}
	if runErr != nil {
		return hookDecision{}, runErr
	}

	dec := hookDecision{}
	trimmed := bytes.TrimSpace(out)
	if len(trimmed) > 0 {
		if err := json.Unmarshal(trimmed, &dec); err != nil {
			// Non-JSON stdout on a clean exit is not a decision. For tool hooks it
			// means "allow, no change". For the two context-injecting lifecycle
			// events, RunLifecycleHooks treats it as injected context (Claude Code
			// parity) — so carry the raw text through in rawStdout.
			r.logger.Debug("hook stdout not JSON (raw)", "hook", h.ID)
			return hookDecision{rawStdout: string(trimmed)}, nil
		}
	}
	return dec, nil
}

// interpreterShellErrors are the stderr signatures a shell emits when it could not
// PARSE or LAUNCH the command at all, as opposed to running it to a non-zero exit.
// Matched case-insensitively against the hook's stderr.
var interpreterShellErrors = []string{
	"syntax error",                  // sh/bash parse failure (exits 2)
	"unexpected token",              // bash `near unexpected token '|'`; powershell uses the same wording
	"unexpected end of file",        // bash: unterminated construct
	"command not found",             // the interpreter could not resolve the program
	"is not recognized as the name", // powershell: unknown cmdlet/executable
	"parsererror",                   // powershell: ParserError from -Command
}

// interpreterFailure reports the stderr line proving the hook's INTERPRETER failed
// (parse/launch), or "" when stderr shows no such signature — in which case an
// exit-2 is honoured as the Claude Code "block" decision it is meant to be.
func interpreterFailure(stderr string) string {
	s := strings.TrimSpace(stderr)
	if s == "" {
		return ""
	}
	low := strings.ToLower(s)
	for _, sig := range interpreterShellErrors {
		if strings.Contains(low, sig) {
			// Report the first line only — enough to identify the fault, and it keeps
			// the warning readable in debug.jsonl.
			if i := strings.IndexAny(s, "\r\n"); i > 0 {
				return s[:i]
			}
			return s
		}
	}
	return ""
}

// hookShellName names the interpreter execHook spawns on this platform, so the
// error text tells the operator which dialect the command must be written in.
func hookShellName() string {
	if runtime.GOOS == "windows" {
		return "PowerShell"
	}
	return "POSIX sh"
}

// hookStep builds an audit TurnStep card for a hook action.
func hookStep(h db.Hook, tool, reason, text, detail string, isErr bool) TurnStep {
	return TurnStep{
		Kind:    StepHook,
		Tool:    tool,
		Text:    text,
		Reason:  reason,
		Output:  detail,
		IsError: isErr,
	}
}

// hookDecisionLabel reduces a hook decision to a short tag for the debug journal
// (allow | block | approve | modify | <perm>), so the per-session debug stream
// shows what each hook actually did without the full payload.
func hookDecisionLabel(dec hookDecision) string {
	if dec.Decision == "block" {
		return "block"
	}
	if dec.HookSpecificOutput != nil {
		if p := strings.ToLower(dec.HookSpecificOutput.PermissionDecision); p != "" {
			return p
		}
	}
	if dec.Decision == "approve" {
		return "approve"
	}
	if len(dec.UpdatedInput) > 0 || dec.UpdatedOutput != nil || dec.AdditionalContext != "" {
		return "modify"
	}
	return "allow"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
