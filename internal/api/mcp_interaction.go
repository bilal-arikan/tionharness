package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/bilal/swarmgo/internal/agent"
	"github.com/bilal/swarmgo/internal/interaction"
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
	ask := tools.NewAskUserTool().Def()
	todo := tools.NewTodoWriteTool().Def()
	return []interaction.ToolSpec{
		{Name: ask.Name, Description: ask.Description, InputSchema: ask.InputSchema},
		{Name: todo.Name, Description: todo.Description, InputSchema: todo.InputSchema},
	}
}

// Call implements interaction.Backend.
func (b *interactionBackend) Call(ctx context.Context, token, name string, args json.RawMessage) (interaction.CallResult, error) {
	run := b.runs.byToken(token)
	if run == nil {
		return interaction.CallResult{}, errors.New("no live turn for token")
	}
	switch name {
	case "mcp__swarmgo_interaction__ask_user", "ask_user":
		return b.callAsk(ctx, run, args)
	case "mcp__swarmgo_interaction__todo_write", "todo_write":
		return b.callTodo(run, args)
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

	run.emit("step", agent.TurnStep{Kind: agent.StepAsk, Text: in.Question, Options: in.Options})

	select {
	case ans := <-run.answer:
		return interaction.CallResult{Text: ans}, nil
	case <-run.done:
		return interaction.CallResult{Text: "the turn ended before the user answered; proceed without the answer", IsError: true}, nil
	case <-ctx.Done():
		return interaction.CallResult{}, ctx.Err()
	case <-time.After(askTimeout):
		return interaction.CallResult{Text: "no answer within the time limit; proceed on your own", IsError: true}, nil
	}
}

// callTodo validates the checklist (reusing the canonical tool), emits a live
// checklist card, and returns the confirmation text — non-blocking.
func (b *interactionBackend) callTodo(run *chatRun, args json.RawMessage) (interaction.CallResult, error) {
	text, err := tools.NewTodoWriteTool().Call(context.Background(), args)
	if err != nil {
		return interaction.CallResult{Text: err.Error(), IsError: true}, nil
	}
	if todos := parseTodoItems(args); len(todos) > 0 {
		run.emit("step", agent.TurnStep{Kind: agent.StepTodo, Todos: todos})
	}
	return interaction.CallResult{Text: text}, nil
}

// parseTodoItems extracts the checklist for the UI step from a todo_write input.
func parseTodoItems(args json.RawMessage) []agent.TodoItem {
	var in struct {
		Todos []agent.TodoItem `json:"todos"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil
	}
	return in.Todos
}

// interactionURL builds the loopback URL a CLI subprocess uses to reach this
// server's /mcp/interaction endpoint, or "" when the base URL is unknown.
func (s *Server) interactionURL() string {
	if s.selfURL == "" {
		return ""
	}
	return strings.TrimRight(s.selfURL, "/") + "/mcp/interaction"
}
