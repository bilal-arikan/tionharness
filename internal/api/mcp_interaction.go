package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/bilal/swarmgo/internal/agent"
	"github.com/bilal/swarmgo/internal/interaction"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/tools"
)

// askTimeout bounds a blocking ask_user call so a never-answering user can't pin
// the CLI tool call forever; on timeout the tool returns an error and the model
// proceeds on its own.
const askTimeout = 15 * time.Minute

// interactionBackend adapts the in-flight chat-run registry to the Interaction
// MCP server: it resolves a per-run Bearer token to its chatRun and dispatches
// ask_user / todo_write against that turn (emitting the same UI steps the native
// tool path emits). See _Docs/11-INTERACTION-MCP.md.
type interactionBackend struct {
	runs *chatRuns
}

// Valid implements interaction.Backend.
func (b *interactionBackend) Valid(token string) bool {
	return b.runs.byToken(token) != nil
}

// Tools implements interaction.Backend. The specs come from the single tool
// definitions in the tools package — the schema is never re-declared here, so the
// native and CLI paths advertise the identical contract.
func (b *interactionBackend) Tools() []interaction.ToolSpec {
	defs := []providers.ToolDef{
		tools.NewAskUserTool().Def(),
		tools.NewTodoWriteTool().Def(),
		tools.NewRequestConfirmationTool().Def(),
		tools.NewCreateArtifactTool().Def(),
		tools.NewUpdateArtifactTool().Def(),
	}
	specs := make([]interaction.ToolSpec, 0, len(defs))
	for _, d := range defs {
		specs = append(specs, interaction.ToolSpec{Name: d.Name, Description: d.Description, InputSchema: d.InputSchema})
	}
	return specs
}

// bareToolName strips the Interaction MCP namespace so dispatch matches whether
// the CLI sends the namespaced (mcp__swarmgo_interaction__ask_user) or bare name.
func bareToolName(name string) string {
	return strings.TrimPrefix(name, "mcp__swarmgo_interaction__")
}

// Call implements interaction.Backend.
func (b *interactionBackend) Call(ctx context.Context, token, name string, args json.RawMessage) (interaction.CallResult, error) {
	run := b.runs.byToken(token)
	if run == nil {
		return interaction.CallResult{}, errors.New("no live turn for token")
	}
	switch bareToolName(name) {
	case "ask_user":
		return b.callAsk(ctx, run, args)
	case "request_confirmation":
		return b.callConfirm(ctx, run, args)
	case "todo_write":
		return b.callTodo(args)
	case "create_artifact", "update_artifact":
		return b.callArtifact(run, bareToolName(name), args)
	default:
		return interaction.CallResult{Text: "unknown tool: " + name, IsError: true}, nil
	}
}

// callAsk emits a transient ask step and blocks until the user answers (via
// POST /api/chat/control {action:"answer"}), the turn ends, or the timeout fires.
func (b *interactionBackend) callAsk(ctx context.Context, run *chatRun, args json.RawMessage) (interaction.CallResult, error) {
	var in struct {
		Question string   `json:"question"`
		Options  []string `json:"options"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return interaction.CallResult{Text: "invalid ask_user input: " + err.Error(), IsError: true}, nil
	}
	if strings.TrimSpace(in.Question) == "" {
		return interaction.CallResult{Text: "question is required", IsError: true}, nil
	}
	return b.blockForAnswer(ctx, run, in.Question, in.Options, func(a string) string { return a })
}

// callConfirm blocks for a yes/no decision on a risky action and normalises it.
func (b *interactionBackend) callConfirm(ctx context.Context, run *chatRun, args json.RawMessage) (interaction.CallResult, error) {
	var in struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return interaction.CallResult{Text: "invalid request_confirmation input: " + err.Error(), IsError: true}, nil
	}
	if strings.TrimSpace(in.Question) == "" {
		return interaction.CallResult{Text: "question is required", IsError: true}, nil
	}
	return b.blockForAnswer(ctx, run, in.Question, tools.ConfirmOptions, tools.NormalizeConfirmation)
}

// blockForAnswer emits an ask step (question + clickable options) and blocks until
// the user answers, the turn ends, the request is cancelled, or the timeout fires.
// normalize maps the raw answer to the tool's result text.
func (b *interactionBackend) blockForAnswer(ctx context.Context, run *chatRun, question string, options []string, normalize func(string) string) (interaction.CallResult, error) {
	run.emit("step", agent.TurnStep{Kind: agent.StepAsk, Text: question, Options: options})
	select {
	case ans := <-run.answer:
		return interaction.CallResult{Text: normalize(ans)}, nil
	case <-run.done:
		return interaction.CallResult{Text: "the turn ended before the user answered; proceed without the answer", IsError: true}, nil
	case <-ctx.Done():
		return interaction.CallResult{}, ctx.Err()
	case <-time.After(askTimeout):
		return interaction.CallResult{Text: "no answer within the time limit; proceed on your own", IsError: true}, nil
	}
}

// callTodo validates the checklist (reusing the canonical tool) and returns the
// confirmation text. No live emit — the CLI's stream-json trace surfaces the
// todo_write call, which traceStepToTurnStep promotes to a checklist card.
func (b *interactionBackend) callTodo(args json.RawMessage) (interaction.CallResult, error) {
	text, err := tools.NewTodoWriteTool().Call(context.Background(), args)
	if err != nil {
		return interaction.CallResult{Text: err.Error(), IsError: true}, nil
	}
	return interaction.CallResult{Text: text}, nil
}

// callArtifact creates or updates a versioned artifact through the run's sink. The
// CLI's stream-json trace surfaces the call as an artifact card (no live emit).
func (b *interactionBackend) callArtifact(run *chatRun, name string, args json.RawMessage) (interaction.CallResult, error) {
	sink := run.artifactSink()
	if sink == nil {
		return interaction.CallResult{Text: "artifacts are not available for this turn", IsError: true}, nil
	}
	actx := tools.WithArtifacts(context.Background(), sink)
	var (
		text string
		err  error
	)
	if name == "create_artifact" {
		text, err = tools.NewCreateArtifactTool().Call(actx, args)
	} else {
		text, err = tools.NewUpdateArtifactTool().Call(actx, args)
	}
	if err != nil {
		return interaction.CallResult{Text: err.Error(), IsError: true}, nil
	}
	return interaction.CallResult{Text: text}, nil
}

// interactionURL builds the loopback URL a CLI subprocess uses to reach this
// server's /mcp/interaction endpoint, or "" when the base URL is unknown.
func (s *Server) interactionURL() string {
	if s.selfURL == "" {
		return ""
	}
	return strings.TrimRight(s.selfURL, "/") + "/mcp/interaction"
}
