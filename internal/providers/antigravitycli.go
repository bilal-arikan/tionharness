package providers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/proc"
)

// AntigravityCLI drives the locally-installed `agy` (Google Antigravity CLI) in
// non-interactive print mode. It mirrors the ClaudeCLI provider as a local
// coding CLI driven as a SwarmGo provider — but the agy interface is simpler:
//
//   - Output is PLAIN TEXT, not stream-json: `agy --print "<prompt>"` prints the
//     model's answer to stdout. There is no --output-format / stream-json flag in
//     agy v1.x, so there is no per-step activity trace, no token usage, and no
//     resumable session id to capture from print mode.
//   - There is NO --append-system-prompt flag, so the system prompt is folded
//     into the single --print argument together with the prior transcript.
//   - The model is auto-selected by agy (defaults to a Flash tier) when --model
//     is omitted; --dangerously-skip-permissions auto-approves tool use.
//
// Auth is delegated to the user's agy configuration exactly as claude-cli
// delegates to the user's claude login: agy authenticates via an interactive
// Google Sign-In on first run (or ANTIGRAVITY_API_KEY when supported). The
// provider injects ANTIGRAVITY_API_KEY into the subprocess env when supplied,
// else inherits the ambient environment unchanged.
//
// MCP tool delegation (agy's .agents/mcp_config.json model) is NOT wired here
// yet — Phase 1 is chat only.
type AntigravityCLI struct {
	binPath string
	model   string // optional model override; "" lets agy auto-select
	apiKey  string // ANTIGRAVITY_API_KEY injected into the subprocess env ("" = inherit ambient)
}

// errAgyNonTTY explains the dominant failure mode: agy's known non-TTY stdout
// bug (google-antigravity/antigravity-cli#76). agy drops its answer (exit 0,
// zero bytes, ignores --print-timeout) when stdout is not a terminal — exactly
// how SwarmGo spawns it. There is no env-var workaround and --output-format is
// rejected in agy v1.x; the only unblock is a pseudo-terminal (Unix `script
// -qec`, Windows ConPTY) or an upstream fix. This provider is EXPERIMENTAL until
// then. (An unauthenticated agy also hangs on interactive Google Sign-In: run
// `agy` once and sign in, or set ANTIGRAVITY_API_KEY.)
const errAgyNonTTY = "agy produced no output: known non-TTY stdout bug (antigravity-cli#76) — agy drops the answer when stdout is not a terminal, which is how SwarmGo runs it. This provider is experimental until agy supports headless stdout / --output-format, or is wrapped in a pseudo-terminal. (Also ensure agy is signed in: run `agy` once and complete Google Sign-In, or set ANTIGRAVITY_API_KEY.)"

// NewAntigravityCLI creates a provider that invokes the given agy binary. model
// is the default model applied when a request omits one ("" = agy auto-selects);
// apiKey, when non-empty, is injected as ANTIGRAVITY_API_KEY into every
// subprocess (else the ambient env is used).
func NewAntigravityCLI(binPath, model, apiKey string) *AntigravityCLI {
	return &AntigravityCLI{binPath: binPath, model: model, apiKey: apiKey}
}

// Name implements Provider.
func (a *AntigravityCLI) Name() string { return "antigravity-cli" }

// Complete implements Provider by shelling out to `agy --print` and capturing
// its plain-text answer from stdout. agy runs its own agentic loop internally;
// in print mode it emits only the final answer (no intermediate trace), so the
// Response carries the text with an empty Trace and zero Usage.
func (a *AntigravityCLI) Complete(ctx context.Context, req Request) (*Response, error) {
	model := req.Model
	if model == "" {
		model = a.model
	}

	prompt := a.buildPrompt(req)

	args := []string{"--print", prompt}
	if model != "" {
		args = append(args, "--model", model)
	}
	// Permission handling: agy has no plan/ask flag, only --dangerously-skip-
	// permissions (auto-approve all tool use). In "auto" (and the empty default)
	// we skip prompts so headless tool use doesn't hang on an approval the print
	// run can't answer. In "ask"/"read-only" we leave prompts on; a pure chat turn
	// (Phase 1, no tools) never triggers one, so it completes either way.
	switch req.PermissionMode {
	case "ask", "read-only":
		// no skip flag — approvals stay interactive (chat turns don't trigger them)
	default: // "auto", "" and any unknown value
		args = append(args, "--dangerously-skip-permissions")
	}
	// Resume a prior agy conversation when the caller supplies its id. agy print
	// mode does not echo the conversation id back, so SwarmGo cannot capture one to
	// store — this only honours an id the caller already holds.
	if req.ResumeSessionID != "" {
		args = append(args, "--conversation", req.ResumeSessionID)
	}

	cmd := proc.CommandContext(ctx, a.binPath, args...)
	env := os.Environ()
	if a.apiKey != "" {
		env = append(env, "ANTIGRAVITY_API_KEY="+a.apiKey)
	}
	cmd.Env = env
	if req.WorkDir != "" {
		if fi, statErr := os.Stat(req.WorkDir); statErr == nil && fi.IsDir() {
			cmd.Dir = req.WorkDir
		}
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	text := strings.TrimSpace(stdout.String())

	if runErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if text == "" {
			if detail == "" {
				detail = errAgyNonTTY
			}
			return nil, fmt.Errorf("antigravity CLI failed: %v: %s", runErr, antigravityTail(detail))
		}
		// Non-zero exit but we DID get text: salvage the answer (agy can exit
		// non-zero on a trailing non-fatal warning) rather than discard real work.
	}
	if text == "" {
		// The dominant cause of an exit-0-but-empty run: agy's non-TTY stdout bug
		// (upstream #76). SwarmGo spawns agy with a piped stdout (not a terminal),
		// so agy drops the model answer. Surface the real cause + the unblock paths.
		return nil, fmt.Errorf("antigravity CLI returned no output — %s", errAgyNonTTY)
	}

	resp := &Response{
		Text:       text,
		Model:      model,
		StopReason: StopEndTurn,
		Trace:      []TraceStep{{Kind: "text", Text: text}},
	}
	// Print mode emits no token accounting; Usage stays zero.
	if req.OnEvent != nil {
		req.OnEvent(resp.Trace[0])
	}
	return resp, nil
}

// buildPrompt folds the system prompt + transcript + final user message into the
// single string agy takes via --print (it has no separate system-prompt flag and
// no documented stdin contract in print mode). A multi-turn history is rendered
// as a labelled transcript so agy has prior context.
func (a *AntigravityCLI) buildPrompt(req Request) string {
	sys := strings.TrimSpace(strings.TrimSpace(req.System) + "\n\n" + strings.TrimSpace(req.SystemDynamic))

	turns := make([]Message, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == RoleUser || m.Role == RoleAssistant {
			turns = append(turns, m)
		}
	}

	var b strings.Builder
	if sys != "" {
		b.WriteString(sys)
		b.WriteString("\n\n")
	}
	if len(turns) == 1 {
		b.WriteString(turns[0].Text)
		return strings.TrimSpace(b.String())
	}
	if len(turns) > 1 {
		b.WriteString("Continue this conversation. Reply only as the assistant to the final user message.\n\n")
		for _, m := range turns {
			label := "User"
			text := m.Text
			if m.Role == RoleAssistant {
				label = "Assistant"
				text = sanitizeTranscriptText(text)
			}
			b.WriteString(label)
			b.WriteString(": ")
			b.WriteString(text)
			b.WriteString("\n\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// antigravityTail trims a noisy multi-line stderr down to a short tail for the
// failure message.
func antigravityTail(s string) string {
	s = strings.TrimSpace(s)
	const max = 400
	if len(s) > max {
		s = "…" + s[len(s)-max:]
	}
	return s
}
