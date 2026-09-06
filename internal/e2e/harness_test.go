package e2e

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// scriptedProvider is a deterministic, network-free stand-in for an LLM. Each
// chat turn pops the next queued Response; a tool-use turn drives the native loop
// to execute real tools, and the loop's follow-up call pops the next entry. The
// rolling-summary compaction call (a single prompt the conversation manager
// issues out-of-band) is answered without consuming a scripted turn, so a test's
// script stays aligned regardless of when a fold happens.
type scriptedProvider struct {
	mu           sync.Mutex
	queue        []*providers.Response
	calls        int
	compactCalls int
	lastReq      providers.Request
	summaryReply string
}

func newScriptedProvider(turns ...*providers.Response) *scriptedProvider {
	return &scriptedProvider{queue: turns, summaryReply: "ROLLING SUMMARY OF PRIOR TURNS"}
}

func (p *scriptedProvider) Name() string { return "scripted" }

func (p *scriptedProvider) Complete(_ context.Context, req providers.Request) (*providers.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.lastReq = req
	if isCompactionRequest(req) {
		p.compactCalls++
		return &providers.Response{StopReason: providers.StopEndTurn, Text: p.summaryReply, Model: "scripted"}, nil
	}
	if len(p.queue) == 0 {
		// Script exhausted: end the turn cleanly rather than hang the loop.
		return &providers.Response{StopReason: providers.StopEndTurn, Text: "", Model: "scripted"}, nil
	}
	r := p.queue[0]
	p.queue = p.queue[1:]
	return r, nil
}

// isCompactionRequest recognises the conversation manager's summarize call by its
// signature: exactly one user message carrying the compaction prompt.
func isCompactionRequest(req providers.Request) bool {
	return len(req.Messages) == 1 &&
		strings.Contains(req.Messages[0].Text, "running, structured summary of a conversation")
}

// --- response builders -------------------------------------------------------

// sayText is a plain end-of-turn textual reply.
func sayText(s string) *providers.Response {
	return &providers.Response{StopReason: providers.StopEndTurn, Text: s, Model: "scripted"}
}

// callTools is a tool-use turn: optional narration plus one or more tool calls
// the native loop will execute before re-invoking the provider.
func callTools(narration string, calls ...providers.ToolCall) *providers.Response {
	return &providers.Response{StopReason: providers.StopToolUse, Text: narration, ToolCalls: calls, Model: "scripted"}
}

// tc builds a single tool call with JSON-encoded arguments.
func tc(id, name string, args map[string]any) providers.ToolCall {
	b, _ := json.Marshal(args)
	return providers.ToolCall{ID: id, Name: name, Input: b}
}

// --- harness -----------------------------------------------------------------

// harness wires a real runtime over throwaway storage. TIONHARNESS_DATA_DIR is
// redirected to a temp dir so the global skills/market seed never touches the
// real ~/.tionharness.
type harness struct {
	t        *testing.T
	rt       *agent.Runtime
	db       *db.DB
	tun      *agent.Tunables
	convo    *conversation.Manager
	provider *scriptedProvider
	workDir  string

	// decorate, when set, wraps the per-turn context before the tool loop runs —
	// the seam the chat API uses to wire interactive concerns (permission prompter,
	// session grants, self-wake scheduler). Tests set it to exercise those paths.
	decorate func(context.Context) context.Context
}

func newHarness(t *testing.T, provider *scriptedProvider) *harness {
	t.Helper()
	t.Setenv("TIONHARNESS_DATA_DIR", t.TempDir())

	root := t.TempDir()
	workDir := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	database, err := db.Open(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	tun := agent.NewTunables()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := agent.NewRuntime(database, providers.NewRegistry(), tun, workDir, filepath.Dir(workDir), nil, nil, "", "", nil, logger)

	return &harness{
		t:        t,
		rt:       rt,
		db:       database,
		tun:      tun,
		convo:    conversation.NewManager(),
		provider: provider,
		workDir:  workDir,
	}
}

// newAgent creates a native, tool-enabled agent (provider "anthropic" keeps it on
// TionHarness's own agentic loop rather than the claude-cli delegation path). Pass
// mutators to tweak fields (e.g. assigned Skills) before persisting.
func (h *harness) newAgent(name string, mut ...func(*db.Agent)) db.Agent {
	h.t.Helper()
	a := db.Agent{Name: name, Provider: "anthropic", Model: "test-model", MCPEnabled: true}
	for _, m := range mut {
		m(&a)
	}
	created, err := h.db.CreateAgent(context.Background(), a)
	if err != nil {
		h.t.Fatalf("create agent: %v", err)
	}
	return created
}

func (h *harness) newSession(a db.Agent) db.Session {
	h.t.Helper()
	s, err := h.db.CreateSession(context.Background(), db.Session{AgentID: a.ID})
	if err != nil {
		h.t.Fatalf("create session: %v", err)
	}
	return s
}

// turnResult bundles what one chat turn produced for assertions.
type turnResult struct {
	resp     *providers.Response   // final provider response (text stitched)
	steps    []agent.TurnStep      // full persisted activity trace
	streamed []agent.TurnStep      // steps delivered live via the onStep sink
	reply    db.Message            // persisted assistant message
	prep     conversation.Prepared // the budgeting result for this turn
}

// send drives ONE full chat turn the way api/chat_stream does: persist the user
// message, replay history through the conversation budgeter, compose the system +
// dynamic (memory) context, run the streaming tool loop, then persist the reply
// and journal it. Returns everything a test needs to assert on.
func (h *harness) send(a db.Agent, sess db.Session, userText string) turnResult {
	h.t.Helper()
	ctx := agent.WithSessionID(context.Background(), sess.ID)

	if _, err := h.db.AddMessage(ctx, db.Message{SessionID: sess.ID, Role: providers.RoleUser, Text: userText}); err != nil {
		h.t.Fatalf("persist user message: %v", err)
	}
	history, err := h.db.ListMessages(ctx, sess.ID)
	if err != nil {
		h.t.Fatalf("list messages: %v", err)
	}
	sess, _ = h.db.GetSession(ctx, sess.ID)

	prep, err := h.convo.Prepare(ctx, h.db, h.provider, sess, a, history)
	if err != nil {
		h.t.Fatalf("prepare: %v", err)
	}

	system := "You are " + a.Name + "."
	if sb := h.rt.SkillsCatalogBlockForAgent(a); sb != "" {
		system = strings.TrimSpace(system + "\n\n" + sb)
	}
	dynamic := h.composeDynamic(ctx, a, userText)
	if prep.Summary != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n# Conversation summary\n" + prep.Summary)
	}

	req := providers.Request{
		Model:         a.Model,
		System:        system,
		SystemDynamic: dynamic,
		Messages:      prep.Messages,
	}

	if h.decorate != nil {
		ctx = h.decorate(ctx)
	}

	var streamed []agent.TurnStep
	resp, steps, err := h.rt.CompleteWithToolsStream(ctx, a, h.provider, req, false, func(st agent.TurnStep) {
		streamed = append(streamed, st)
	})
	if err != nil {
		h.t.Fatalf("turn failed: %v", err)
	}

	stepsJSON, _ := json.Marshal(steps)
	reply, err := h.db.AddMessage(ctx, db.Message{
		SessionID: sess.ID,
		Role:      providers.RoleAssistant,
		AgentID:   a.ID,
		Text:      resp.Text,
		Steps:     string(stepsJSON),
	})
	if err != nil {
		h.t.Fatalf("persist assistant message: %v", err)
	}

	return turnResult{resp: resp, steps: steps, streamed: streamed, reply: reply, prep: prep}
}

// sendMulti drives ONE turn answered by several agents in order, the way
// api/chat_stream routes an "@mention" turn: the user message is persisted once,
// then each agent re-reads the growing history (so a later agent sees the earlier
// replies) and appends its own reply. Returns one turnResult per agent, in order.
func (h *harness) sendMulti(agents []db.Agent, sess db.Session, userText string) []turnResult {
	h.t.Helper()
	baseCtx := agent.WithSessionID(context.Background(), sess.ID)

	if _, err := h.db.AddMessage(baseCtx, db.Message{SessionID: sess.ID, Role: providers.RoleUser, Text: userText}); err != nil {
		h.t.Fatalf("persist user message: %v", err)
	}

	var out []turnResult
	for _, a := range agents {
		ctx := baseCtx
		history, err := h.db.ListMessages(ctx, sess.ID)
		if err != nil {
			h.t.Fatalf("list messages: %v", err)
		}
		s, _ := h.db.GetSession(ctx, sess.ID)
		prep, err := h.convo.Prepare(ctx, h.db, h.provider, s, a, history)
		if err != nil {
			h.t.Fatalf("prepare: %v", err)
		}

		req := providers.Request{
			Model:         a.Model,
			System:        "You are " + a.Name + ".",
			SystemDynamic: h.composeDynamic(ctx, a, userText),
			Messages:      prep.Messages,
		}
		if h.decorate != nil {
			ctx = h.decorate(ctx)
		}
		resp, steps, err := h.rt.CompleteWithToolsStream(ctx, a, h.provider, req, false, nil)
		if err != nil {
			h.t.Fatalf("turn failed for %s: %v", a.Name, err)
		}
		stepsJSON, _ := json.Marshal(steps)
		reply, err := h.db.AddMessage(ctx, db.Message{
			SessionID: sess.ID, Role: providers.RoleAssistant, AgentID: a.ID,
			Text: resp.Text, Steps: string(stepsJSON),
		})
		if err != nil {
			h.t.Fatalf("persist reply for %s: %v", a.Name, err)
		}
		out = append(out, turnResult{resp: resp, steps: steps, reply: reply, prep: prep})
	}
	return out
}

// composeDynamic previously mirrored the memory half of the per-turn dynamic
// context. The memory subsystem was removed, so there is no per-turn dynamic
// suffix to compose here; kept as a no-op so the turn drivers stay unchanged.
func (h *harness) composeDynamic(ctx context.Context, a db.Agent, query string) string {
	return ""
}

// --- assertion helpers -------------------------------------------------------

// findToolStep returns the first trace step recorded for the named tool, or nil.
func findToolStep(steps []agent.TurnStep, tool string) *agent.TurnStep {
	for i := range steps {
		if steps[i].Tool == tool {
			return &steps[i]
		}
	}
	return nil
}
