package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/proc"
)

// ClaudeCLI drives the locally-installed `claude` (Claude Code) CLI in
// non-interactive print mode. It authenticates via the user's existing
// OAuth/subscription login — no API key required.
//
// This mirrors SwarmClaw's "CLI provider" approach (Claude Code, Codex, ...).
type ClaudeCLI struct {
	binPath string
	model   string // optional alias/name override, e.g. "sonnet"

	// MCP delegation (Phase 8): when mcpConfigPath is set, the CLI is launched
	// with that MCP config and restricted to allowedTools. The CLI then runs
	// the full agentic tool loop itself and returns the final text. This is the
	// keyless tool-use path (no ANTHROPIC_API_KEY required).
	mcpConfigPath   string
	allowedTools    []string
	disallowedTools []string // CLI built-ins to suppress (e.g. AskUserQuestion, TodoWrite)
	// permissionPromptTool, when set, is handed to --permission-prompt-tool so the
	// CLI routes tools needing approval through that MCP tool (used in "ask" mode
	// instead of acceptEdits). Empty → fall back to the --permission-mode flag.
	permissionPromptTool string
	// settingsPath, when set, is handed to --settings so the CLI loads a SwarmGo-
	// generated settings.json (permission deny-list + PreToolUse/PostToolUse hooks)
	// for this turn. Lifecycle mirrors mcpConfigPath: set fresh by ConfigureMCP each
	// MCP turn (possibly "") and only emitted on the MCP path.
	settingsPath string
}

// NewClaudeCLI creates a provider that invokes the given claude binary.
func NewClaudeCLI(binPath, model string) *ClaudeCLI {
	return &ClaudeCLI{binPath: binPath, model: model}
}

// ConfigureMCP enables MCP tool delegation for subsequent Complete calls.
// configPath points to a claude --mcp-config JSON file; allowedTools is the
// list of tool identifiers the CLI may use (e.g. "mcp__filesystem");
// disallowedTools suppresses conflicting CLI built-ins (e.g. AskUserQuestion,
// TodoWrite) so the SwarmGo Interaction MCP equivalents are used instead.
// settingsPath points to a --settings file (permission deny-list + hooks); pass
// "" for none. Its lifecycle is tied to configPath so it is reset every MCP turn.
func (c *ClaudeCLI) ConfigureMCP(configPath string, allowedTools, disallowedTools []string, permissionPromptTool, settingsPath string) {
	c.mcpConfigPath = configPath
	c.allowedTools = allowedTools
	c.disallowedTools = disallowedTools
	c.permissionPromptTool = permissionPromptTool
	c.settingsPath = settingsPath
}

// Name implements Provider.
func (c *ClaudeCLI) Name() string { return "claude-cli" }

// permissionModeArgs maps SwarmGo's permission mode onto the claude CLI's
// permission flags. In headless (-p) mode the default mode cannot prompt for
// approval, so Edit/Write/Bash are refused unless an explicit mode is set:
//   - "read-only" → --permission-mode plan       (no mutations)
//   - "ask"       → --permission-mode acceptEdits (edits auto-approved)
//   - "auto"/""   → --dangerously-skip-permissions (everything auto-approved)
func permissionModeArgs(mode string) []string {
	switch mode {
	case "read-only":
		return []string{"--permission-mode", "plan"}
	case "ask":
		return []string{"--permission-mode", "acceptEdits"}
	default: // "auto", "" and any unknown value
		return []string{"--dangerously-skip-permissions"}
	}
}

// interactionSystemNote tells the CLI to use the SwarmGo Interaction MCP tools
// (which surface in the SwarmGo UI) instead of its own built-ins, which can't be
// answered in non-interactive print mode.
const interactionSystemNote = "To ask the user a clarifying question, call the ask_user tool and wait for the reply. " +
	"To show or update a task checklist, call todo_write. " +
	"Do not use the built-in AskUserQuestion or TodoWrite tools."

// usesInteractionTools reports whether the SwarmGo Interaction MCP tools are in
// the allowlist for this call.
func (c *ClaudeCLI) usesInteractionTools() bool {
	for _, t := range c.allowedTools {
		if strings.Contains(t, "swarmgo_interaction") {
			return true
		}
	}
	return false
}

// --- stream-json event shapes (--output-format stream-json --verbose) ---
//
// The CLI emits one JSON object per line: system/init, assistant (content
// blocks: text/thinking/tool_use), user (tool_result blocks), and a final
// result envelope. We parse this stream to capture the CLI's own tool loop and
// thinking as an activity trace — keyless, no ANTHROPIC_API_KEY required.

type cliUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type cliBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"` // tool_use block identifier
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"` // tool_result → references a tool_use id
	Content   json.RawMessage `json:"content"`     // tool_result: string or block array
	IsError   bool            `json:"is_error"`
}

type cliMessage struct {
	Model   string     `json:"model"`
	Content []cliBlock `json:"content"`
	Usage   *cliUsage  `json:"usage"`
}

type cliEvent struct {
	Type       string                     `json:"type"`
	Subtype    string                     `json:"subtype"`
	Message    *cliMessage                `json:"message"`
	IsError    bool                       `json:"is_error"`
	Result     string                     `json:"result"`
	Usage      *cliUsage                  `json:"usage"`
	ModelUsage map[string]json.RawMessage `json:"modelUsage"`
}

// Complete implements Provider by shelling out to `claude -p` and parsing its
// streamed JSON event log line-by-line. When req.OnEvent is set, each activity
// step is delivered as soon as it completes (step-by-step streaming); the final
// Response carries the full text + trace regardless.
func (c *ClaudeCLI) Complete(ctx context.Context, req Request) (*Response, error) {
	// Note: do NOT use --bare here — it skips keychain reads and breaks the
	// OAuth/subscription login ("Not logged in"). stream-json needs --verbose.
	args := []string{"-p", "--output-format", "stream-json", "--verbose"}

	model := req.Model
	if model == "" {
		model = c.model
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	// Permission handling. In "ask" mode with a permission-prompt tool wired, route
	// tools that need approval through it (CLI default mode + --permission-prompt-
	// tool → real per-tool approval in the SwarmGo UI). Otherwise map the mode to a
	// CLI permission flag (plan / acceptEdits / bypass) so headless edits aren't
	// silently refused.
	if c.permissionPromptTool != "" {
		args = append(args, "--permission-prompt-tool", c.permissionPromptTool)
	} else {
		args = append(args, permissionModeArgs(req.PermissionMode)...)
	}
	// The CLI has no prompt-cache breakpoint, so the static prefix and dynamic
	// suffix are merged into one appended system prompt.
	sys := strings.TrimSpace(strings.TrimSpace(req.System) + "\n\n" + strings.TrimSpace(req.SystemDynamic))
	if c.usesInteractionTools() {
		sys = strings.TrimSpace(sys + "\n\n" + interactionSystemNote)
	}
	if sys != "" {
		args = append(args, "--append-system-prompt", sys)
	}

	// MCP delegation: load the config and restrict to the allowlist. The
	// single-value --mcp-config is terminated by the boolean --strict-mcp-config;
	// --disallowedTools (suppressing conflicting CLI built-ins) precedes the
	// trailing --allowedTools so neither variadic flag swallows the other.
	if c.mcpConfigPath != "" {
		args = append(args, "--mcp-config", c.mcpConfigPath, "--strict-mcp-config")
		// SwarmGo-generated settings (permission deny-list + PreToolUse/PostToolUse
		// hooks) for this turn. Scoped to the MCP path so a stale path can't leak
		// onto the plain (non-MCP) completion path.
		if c.settingsPath != "" {
			args = append(args, "--settings", c.settingsPath)
		}
		if len(c.disallowedTools) > 0 {
			args = append(args, "--disallowedTools")
			args = append(args, c.disallowedTools...)
		}
		if len(c.allowedTools) > 0 {
			args = append(args, "--allowedTools")
			args = append(args, c.allowedTools...)
		}
	}

	prompt := serializeTranscript(req.Messages)

	cmd := exec.CommandContext(ctx, c.binPath, args...)
	proc.Hide(cmd) // no console flash when launched from the windowless desktop app
	// Run inside the workspace sandbox so relative paths (e.g. an attachment's
	// "uploads/<sid>/<file>") resolve there rather than the backend's launch
	// directory. Only set when the dir exists; otherwise inherit the default cwd.
	if req.WorkDir != "" {
		if fi, statErr := os.Stat(req.WorkDir); statErr == nil && fi.IsDir() {
			cmd.Dir = req.WorkDir
		}
	}
	cmd.Stdin = strings.NewReader(prompt) // pass prompt via stdin to avoid arg limits
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Parse events as they stream so OnEvent fires step-by-step. ReadString
	// handles arbitrarily long lines (tool results / the init tool list).
	p := newCLIParser(model, req.OnEvent)
	rd := bufio.NewReader(stdout)
	for {
		line, rerr := rd.ReadString('\n')
		if line != "" {
			p.feed(line)
		}
		if rerr != nil {
			break
		}
	}
	runErr := cmd.Wait()

	resp, parseErr := p.finish()
	if parseErr != nil {
		if runErr != nil {
			return nil, fmt.Errorf("claude CLI failed: %v %s", runErr, strings.TrimSpace(stderr.String()))
		}
		return nil, parseErr
	}
	return resp, nil
}

// cliStreamParser incrementally consumes the stream-json event log, building a
// Response.Trace and (when onEvent is set) emitting each step the moment it is
// ready: thinking immediately, intermediate text on flush, a tool step once its
// result arrives. The trailing text is the final answer (not emitted as a step).
type cliStreamParser struct {
	resp      *Response
	onEvent   func(TraceStep)
	toolIdx   map[string]int // tool_use id → index in resp.Trace
	emitted   map[int]bool   // trace index → already delivered via onEvent
	pending   strings.Builder
	finalText string
	sawResult bool
	hadError  bool
	errText   string
}

func newCLIParser(model string, onEvent func(TraceStep)) *cliStreamParser {
	return &cliStreamParser{
		resp:    &Response{Model: model},
		onEvent: onEvent,
		toolIdx: map[string]int{},
		emitted: map[int]bool{},
	}
}

func (p *cliStreamParser) emit(i int) {
	if p.onEvent == nil || p.emitted[i] || i < 0 || i >= len(p.resp.Trace) {
		return
	}
	p.emitted[i] = true
	p.onEvent(p.resp.Trace[i])
}

func (p *cliStreamParser) flushText() {
	t := strings.TrimSpace(p.pending.String())
	p.pending.Reset()
	if t != "" {
		p.resp.Trace = append(p.resp.Trace, TraceStep{Kind: "text", Text: t})
		p.emit(len(p.resp.Trace) - 1)
	}
}

// feed processes one event line from the stream.
func (p *cliStreamParser) feed(line string) {
	line = strings.TrimSpace(line)
	if line == "" || line[0] != '{' {
		return
	}
	var ev cliEvent
	if json.Unmarshal([]byte(line), &ev) != nil {
		return
	}

	switch ev.Type {
	case "assistant":
		if ev.Message == nil {
			return
		}
		if ev.Message.Model != "" {
			p.resp.Model = ev.Message.Model
		}
		if ev.Message.Usage != nil {
			p.resp.Usage.OutputTokens += ev.Message.Usage.OutputTokens
			if ev.Message.Usage.InputTokens > p.resp.Usage.InputTokens {
				p.resp.Usage.InputTokens = ev.Message.Usage.InputTokens
			}
		}
		for _, b := range ev.Message.Content {
			switch b.Type {
			case "text":
				p.pending.WriteString(b.Text)
			case "thinking":
				p.flushText()
				if t := strings.TrimSpace(b.Thinking); t != "" {
					p.resp.Trace = append(p.resp.Trace, TraceStep{Kind: "thinking", Text: t})
					p.emit(len(p.resp.Trace) - 1)
				}
			case "tool_use":
				p.flushText()
				p.resp.Trace = append(p.resp.Trace, TraceStep{Kind: "tool", Tool: b.Name, Input: b.Input})
				if b.ID != "" {
					p.toolIdx[b.ID] = len(p.resp.Trace) - 1
				}
				// Not emitted yet — wait for its tool_result to fill the output.
			}
		}
	case "user":
		if ev.Message == nil {
			return
		}
		for _, b := range ev.Message.Content {
			if b.Type != "tool_result" {
				continue
			}
			if i, ok := p.toolIdx[b.ToolUseID]; ok {
				p.resp.Trace[i].Output = toolResultText(b.Content)
				p.resp.Trace[i].IsError = b.IsError
				p.emit(i)
			}
		}
	case "result":
		p.sawResult = true
		if ev.IsError {
			p.hadError = true
			p.errText = ev.Result
			return
		}
		p.finalText = ev.Result
		if ev.Usage != nil {
			if ev.Usage.InputTokens > 0 {
				p.resp.Usage.InputTokens = ev.Usage.InputTokens
			}
			if ev.Usage.OutputTokens > 0 {
				p.resp.Usage.OutputTokens = ev.Usage.OutputTokens
			}
		}
		for k := range ev.ModelUsage {
			p.resp.Model = k
			break
		}
	}
}

// finish resolves the final answer and emits any tool steps whose result never
// arrived (so the UI still sees them).
func (p *cliStreamParser) finish() (*Response, error) {
	if p.hadError {
		return nil, fmt.Errorf("claude CLI error: %s", p.errText)
	}
	if !p.sawResult {
		return nil, fmt.Errorf("claude CLI: no result in stream")
	}
	for i := range p.resp.Trace {
		if p.resp.Trace[i].Kind == "tool" {
			p.emit(i)
		}
	}
	if p.finalText == "" {
		p.finalText = strings.TrimSpace(p.pending.String())
	}
	// Output guard: if the CLI echoed a harness repair reminder instead of an
	// answer (a malformed replayed turn can trigger this), don't surface it as
	// the assistant's reply. Strip the artifact; if nothing genuine remains,
	// fail the turn so the caller can retry rather than persist the reminder.
	if isRepairArtifact(p.finalText) {
		if cleaned := sanitizeTranscriptText(p.finalText); cleaned != "" {
			p.finalText = cleaned
		} else {
			return nil, fmt.Errorf("claude CLI returned only a repair reminder, not an answer")
		}
	}
	p.resp.Text = p.finalText
	return p.resp, nil
}

// toolResultText extracts displayable text from a tool_result content field,
// which the CLI encodes either as a JSON string or an array of content blocks.
func toolResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []cliBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var b strings.Builder
		for _, blk := range blocks {
			if blk.Text != "" {
				b.WriteString(blk.Text)
			}
		}
		return b.String()
	}
	return string(raw)
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
		text := m.Text
		if m.Role == RoleAssistant {
			label = "Assistant"
			// Strip any leaked tool-call / harness markup from prior assistant
			// turns so the CLI never sees a malformed message and injects its own
			// repair <system-reminder> (which the model would then echo back).
			text = sanitizeTranscriptText(text)
		}
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}
