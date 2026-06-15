package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// ClaudeCLI drives the locally-installed `claude` (Claude Code) CLI in
// non-interactive print mode. It authenticates via the user's existing
// OAuth/subscription login — no API key required.
//
// This mirrors SwarmClaw's "CLI provider" approach (Claude Code, Codex, ...).
type ClaudeCLI struct {
	binPath string
	model   string // optional alias/name override, e.g. "sonnet"
}

// NewClaudeCLI creates a provider that invokes the given claude binary.
func NewClaudeCLI(binPath, model string) *ClaudeCLI {
	return &ClaudeCLI{binPath: binPath, model: model}
}

// Name implements Provider.
func (c *ClaudeCLI) Name() string { return "claude-cli" }

// cliResult mirrors the `--output-format json` result envelope.
type cliResult struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	IsError   bool   `json:"is_error"`
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
	Usage     struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	ModelUsage map[string]json.RawMessage `json:"modelUsage"`
}

// Complete implements Provider by shelling out to `claude -p`.
func (c *ClaudeCLI) Complete(ctx context.Context, req Request) (*Response, error) {
	// Note: do NOT use --bare here — it skips keychain reads and breaks the
	// OAuth/subscription login ("Not logged in"). Print mode + JSON output.
	args := []string{"-p", "--output-format", "json"}

	model := req.Model
	if model == "" {
		model = c.model
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if sys := strings.TrimSpace(req.System); sys != "" {
		args = append(args, "--append-system-prompt", sys)
	}

	prompt := serializeTranscript(req.Messages)

	cmd := exec.CommandContext(ctx, c.binPath, args...)
	cmd.Stdin = strings.NewReader(prompt) // pass prompt via stdin to avoid arg limits

	out, runErr := cmd.Output()

	// The CLI emits a JSON envelope on stdout even on non-zero exit, so try
	// to parse it first for a meaningful error (e.g. "Not logged in").
	var res cliResult
	if jsonErr := json.Unmarshal(out, &res); jsonErr != nil {
		if runErr != nil {
			stderr := ""
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				stderr = strings.TrimSpace(string(exitErr.Stderr))
			}
			return nil, fmt.Errorf("claude CLI failed: %v %s", runErr, stderr)
		}
		return nil, fmt.Errorf("claude CLI decode: %w (raw: %.200s)", jsonErr, string(out))
	}
	if res.IsError {
		return nil, fmt.Errorf("claude CLI error: %s", res.Result)
	}

	// Resolve the model name actually used (first key of modelUsage).
	usedModel := model
	for k := range res.ModelUsage {
		usedModel = k
		break
	}

	return &Response{
		Text:  res.Result,
		Model: usedModel,
		Usage: Usage{
			InputTokens:  res.Usage.InputTokens,
			OutputTokens: res.Usage.OutputTokens,
		},
	}, nil
}

// serializeTranscript turns a multi-turn history into a single prompt.
// For a single user turn it returns the text directly; otherwise it builds
// a labelled transcript so the CLI has prior context.
func serializeTranscript(msgs []Message) string {
	// Filter to user/assistant turns.
	turns := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == RoleUser || m.Role == RoleAssistant {
			turns = append(turns, m)
		}
	}
	if len(turns) == 0 {
		return ""
	}
	if len(turns) == 1 {
		return turns[0].Text
	}

	var b strings.Builder
	b.WriteString("Continue this conversation. Reply only as the assistant to the final user message.\n\n")
	for _, m := range turns {
		label := "User"
		if m.Role == RoleAssistant {
			label = "Assistant"
		}
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(m.Text)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}
