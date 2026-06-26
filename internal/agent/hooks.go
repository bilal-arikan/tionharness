package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"

	"github.com/bilal-arikan/swarmgo/internal/proc"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/providers"
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
// Code's hook contract so the same external scripts (e.g. sqz) work unchanged.
type hookPayload struct {
	SessionID     string          `json:"session_id,omitempty"`
	Cwd           string          `json:"cwd,omitempty"`
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input,omitempty"`
	ToolResponse  *hookToolResp   `json:"tool_response,omitempty"`
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
}

type hookSpecificOutput struct {
	PermissionDecision       string `json:"permissionDecision"` // allow | deny | ask
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

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
// matcher matches every tool; otherwise it is a shell-style glob (*, ?).
func hookMatches(matcher, tool string) bool {
	if matcher == "" {
		return true
	}
	ok, err := filepath.Match(matcher, tool)
	return err == nil && ok
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
			r.logger.Warn("pre hook failed (fail-open)", "hook", h.ID, "tool", call.Name, "error", derr)
			r.emitDebug(ctx, db.DebugEvent{Type: db.DebugHook, Name: db.HookPreToolUse, DurMs: time.Since(hookStart).Milliseconds(), Detail: call.Name + ":error", Err: true})
			continue
		}
		r.emitDebug(ctx, db.DebugEvent{Type: db.DebugHook, Name: db.HookPreToolUse, DurMs: time.Since(hookStart).Milliseconds(), Detail: call.Name + ":" + hookDecisionLabel(dec)})
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
			r.emitDebug(ctx, db.DebugEvent{Type: db.DebugHook, Name: db.HookPostToolUse, DurMs: time.Since(hookStart).Milliseconds(), Detail: call.Name + ":error", Err: true})
			continue
		}
		r.emitDebug(ctx, db.DebugEvent{Type: db.DebugHook, Name: db.HookPostToolUse, DurMs: time.Since(hookStart).Milliseconds(), Detail: call.Name + ":" + hookDecisionLabel(dec)})
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
			// Non-JSON stdout on a clean exit is treated as "allow, no change"
			// rather than an error — many hooks just print diagnostics.
			r.logger.Debug("hook stdout not JSON (ignored)", "hook", h.ID)
			return hookDecision{}, nil
		}
	}
	return dec, nil
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
