package api

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/interaction"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/sessionhub"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// captureRun installs a write capture on a run and returns the collected events.
func captureRun(run *chatRun) (*[]capturedStep, *sync.Mutex) {
	var mu sync.Mutex
	var steps []capturedStep
	run.setWrite(func(event string, data any) {
		mu.Lock()
		defer mu.Unlock()
		b, _ := json.Marshal(data)
		steps = append(steps, capturedStep{event: event, data: b})
	})
	return &steps, &mu
}

type capturedStep struct {
	event string
	data  json.RawMessage
}

func TestInteractionBackend_AskRoundTrip(t *testing.T) {
	// The CLI ask path now routes through the session interaction store (CAS) +
	// the hub (interaction_open / interaction_resolved), not the old run.answer
	// channel. So the round trip is: callAsk publishes interaction_open with a
	// fresh id → a window (here, the test) reads that id off the hub and resolves
	// it → the blocked call returns the answer.
	srv := &Server{
		runs:         newChatRuns(),
		hub:          sessionhub.New("test", 0),
		interactions: newInteractionStore(),
	}
	run := srv.runs.register("r1", "s-r1", "ws1", func() {})
	defer srv.runs.unregister("r1")

	b := &interactionBackend{runs: srv.runs, apiSrv: srv}
	if !b.Valid(run.token) {
		t.Fatal("token should be valid")
	}
	if b.Valid("nope") {
		t.Fatal("unknown token should be invalid")
	}

	// Subscribe BEFORE the call so the live interaction_open frame is not missed,
	// then resolve it with the answer as soon as it opens.
	_, ch, _ := srv.hub.Subscribe("ws1", "s-r1")
	gotOpen := make(chan struct{}, 1)
	go func() {
		for ev := range ch {
			if ev.Kind != sessionhub.KindInteractionOpen {
				continue
			}
			var p struct {
				ID       string   `json:"id"`
				Kind     string   `json:"kind"`
				Question string   `json:"question"`
				Options  []string `json:"options"`
			}
			if err := json.Unmarshal(ev.Payload, &p); err != nil {
				return
			}
			if p.Kind != "ask" || p.Question != "color?" || len(p.Options) != 2 {
				t.Errorf("unexpected interaction_open: %+v", p)
			}
			srv.resolveInteraction("ws1", "s-r1", p.ID, "BLUE", "test")
			gotOpen <- struct{}{}
			return
		}
	}()

	res, err := b.Call(context.Background(), run.token, "ask_user", json.RawMessage(`{"question":"color?","options":["RED","BLUE"]}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError || res.Text != "BLUE" {
		t.Fatalf("want answer BLUE, got %+v", res)
	}
	select {
	case <-gotOpen:
	case <-time.After(time.Second):
		t.Fatal("interaction_open was never observed on the hub")
	}
}

// TestInteractionAdvertisedNames locks the single-source invariant (CLI-2): the
// names handed to the CLI allowlist (InteractionEndpoint.ToolNames) are exactly
// the names the backend advertises via Tools(). Adding a tool to one therefore
// adds it to the other automatically. Also asserts the use_skill bridge and the
// always-on spawn_session are advertised.
func TestInteractionAdvertisedNames(t *testing.T) {
	b := &interactionBackend{runs: newChatRuns()}
	specs := b.Tools("", "") // no token → static set only (no per-run bridge); tier "" → full set
	want := make(map[string]bool, len(specs))
	for _, s := range specs {
		want[s.Name] = true
	}
	got := interactionAdvertisedNames(nil, false)
	if len(got) != len(specs) {
		t.Fatalf("advertised names (%d) must match Tools() specs (%d)", len(got), len(specs))
	}
	for _, n := range got {
		if !want[n] {
			t.Fatalf("advertised name %q is not in Tools()", n)
		}
	}
	names := strings.Join(got, ",")
	if !contains(got, "use_skill") {
		t.Fatalf("use_skill must be advertised (CLI skill bridge); got %s", names)
	}
	// spawn_session is always advertised now (the self-manage master toggle was
	// removed 2026-07-01; self-management is always on).
	if !contains(got, "spawn_session") {
		t.Fatalf("spawn_session must be advertised (self-management always on); got %s", names)
	}
	// Interactive (chat) turn advertises ask_user / request_confirmation.
	if !contains(got, "ask_user") || !contains(got, "request_confirmation") {
		t.Fatalf("interactive turn must advertise ask_user + request_confirmation; got %s", names)
	}

	// Autonomous turn must DROP the interactive tools (no live user to answer) so a
	// claude-cli flow child never calls them and crashes on the is_error result.
	auto := interactionAdvertisedNames(nil, true)
	if contains(auto, "ask_user") || contains(auto, "request_confirmation") {
		t.Fatalf("autonomous turn must NOT advertise ask_user/request_confirmation; got %s", strings.Join(auto, ","))
	}
	// Non-interactive bridged tools stay available autonomously (e.g. use_skill).
	if !contains(auto, "use_skill") {
		t.Fatalf("autonomous turn must keep non-interactive tools (use_skill); got %s", strings.Join(auto, ","))
	}
}

// TestInteractionTierSplit locks the two-tier CLI bridge (claude-cli 2.1.x+): the eager
// core set (alwaysLoad server) and the deferred extended set. The extended ADVERTISED
// surface is now dynamic (Doc 52 gateway): it starts EMPTY and grows via activate_tools,
// so the partition is verified by CLASSIFICATION (cliTier) rather than by the advertised
// extended list — every full tool is core or extended, and advertised core matches.
func TestInteractionTierSplit(t *testing.T) {
	b := &interactionBackend{runs: newChatRuns()} // tun nil → self-manage off
	full := b.Tools("", "")
	core := b.Tools("", "core")

	// Extended advertises only ACTIVATED tools → empty at the start of a session.
	if ext := b.Tools("", "extended"); len(ext) != 0 {
		t.Fatalf("extended tier must start empty (dynamic gateway surface), got %d", len(ext))
	}
	// Classification partitions the full set into core + extended (no hidden with nil visOf).
	var coreC, extC int
	for _, s := range full {
		switch cliTier(s.Name, nil) {
		case "core":
			coreC++
		case "extended":
			extC++
		}
	}
	if coreC+extC != len(full) {
		t.Fatalf("classification must partition full: core(%d)+extended(%d) != full(%d)", coreC, extC, len(full))
	}
	if len(core) != coreC {
		t.Fatalf("advertised core(%d) must equal core-classified(%d)", len(core), coreC)
	}
	// Pure classification: the eager shell/edit tools are core.
	for _, n := range []string{"Bash", "run_subagent"} {
		if interactionTier(n) != "core" {
			t.Errorf("interactionTier(%q) must be core", n)
		}
	}
	// Eager essentials present without a tun live in core.
	for _, n := range []string{"ask_user", "use_skill", "permission_prompt", "todo_write", "create_artifact"} {
		if !specHasTool(core, n) {
			t.Errorf("%q must be in the core (alwaysLoad) tier", n)
		}
	}
	// NameOnly session-lifecycle tools classify as extended (deferred), never core.
	for _, n := range []string{"update_session", "notify"} {
		if interactionTier(n) != "extended" {
			t.Errorf("%q must classify as extended (deferred)", n)
		}
		if specHasTool(core, n) {
			t.Errorf("%q must NOT be in the core tier", n)
		}
	}

	// splitInteractionTiers must place every bridged self-management def in the
	// extended tier and keep eager statics in core.
	bridge := []providers.ToolDef{{Name: "create_agent"}, {Name: "list_flows"}}
	gotCore, gotExt := splitInteractionTiers(interactionAdvertisedNames(nil, false), bridge, nil)
	if !contains(gotCore, "ask_user") || contains(gotCore, "update_session") {
		t.Errorf("split core tier wrong: %v", gotCore)
	}
	if !contains(gotExt, "create_agent") || !contains(gotExt, "list_flows") || !contains(gotExt, "update_session") {
		t.Errorf("split extended tier must carry bridged + NameOnly tools: %v", gotExt)
	}
	if contains(gotExt, "create_artifact") {
		t.Errorf("eager create_artifact must not leak into the extended tier: %v", gotExt)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// TestInteractionBridge covers CLI-3: a run with bridged self-management tools
// installed advertises them in Tools(token) and, once activated, dispatches them
// via Call's default case through the run's bridge dispatcher.
func TestInteractionBridge(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("rb", "s-rb", "ws1", func() {})
	defer runs.unregister("rb")

	var gotName string
	var gotArgs string
	run.setBridge(
		[]providers.ToolDef{{Name: "create_agent", Description: "make an agent", InputSchema: json.RawMessage(`{"type":"object"}`)}},
		func(_ context.Context, name string, args json.RawMessage) (string, error) {
			gotName, gotArgs = name, string(args)
			return "agent created", nil
		},
	)

	b := &interactionBackend{runs: runs}

	// Advertised for this token: the bridged tool appears alongside the static set.
	if !specHasTool(b.Tools(run.token, ""), "create_agent") {
		t.Fatal("bridged tool create_agent must be advertised for the run token")
	}
	// Not advertised for an unknown token (no run → static set only).
	if specHasTool(b.Tools("other", ""), "create_agent") {
		t.Fatal("bridged tool must not leak to a different token")
	}

	// Activated tools dispatch through the bridge (the extended namespace is stripped
	// before dispatch).
	b.activateExtended(run.token, []string{"create_agent"})
	res, err := b.Call(context.Background(), run.token, extendedNSPrefix+"create_agent", json.RawMessage(`{"name":"x"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError || res.Text != "agent created" {
		t.Fatalf("want bridged result, got %+v", res)
	}
	if gotName != "create_agent" || gotArgs != `{"name":"x"}` {
		t.Fatalf("bridge dispatch got name=%q args=%q", gotName, gotArgs)
	}
}

func TestGatewayToolCallErrorsAreActionable(t *testing.T) {
	runs := newChatRuns()
	b := &interactionBackend{runs: runs}
	run := runs.register("r1", "s1", "ws1", func() {})
	tok := runs.interactionToken("ws1", "s1", "a1")
	runs.bindActive(tok, run)
	run.setBridge(
		[]providers.ToolDef{{Name: "list_agents", Description: "list agents"}},
		func(_ context.Context, name string, _ json.RawMessage) (string, error) { return "called:" + name, nil },
	)

	for _, name := range []string{"list_agents", extendedNSPrefix + "list_agents"} {
		res, err := b.Call(context.Background(), tok, name, nil)
		if err != nil || !res.IsError {
			t.Fatalf("inactive %q must return a tool error, got res=%+v err=%v", name, res, err)
		}
		for _, want := range []string{
			extendedNSPrefix + "list_agents",
			`activate_tools({"tools":["mcp__tionharness_extended__list_agents"]})`,
			"next turn, not the current turn",
		} {
			if !strings.Contains(res.Text, want) {
				t.Errorf("inactive error %q must contain %q", res.Text, want)
			}
		}
	}

	run.setToolAllowed(func(name string) bool { return name != "list_agents" })
	blocked, _ := b.Call(context.Background(), tok, "list_agents", nil)
	if !blocked.IsError || !strings.Contains(blocked.Text, "blocked") || strings.Contains(blocked.Text, "Activate it") {
		t.Fatalf("policy-blocked tool must not recommend activation, got %q", blocked.Text)
	}

	unknown, _ := b.Call(context.Background(), tok, "list_agent", nil)
	if !unknown.IsError || !strings.Contains(unknown.Text, "No such tool available: list_agent") ||
		!strings.Contains(unknown.Text, extendedNSPrefix+"list_agents") || !strings.Contains(unknown.Text, "tool_search") {
		t.Fatalf("unknown tool error must suggest a close name and tool_search, got %q", unknown.Text)
	}
}

func specHasTool(specs []interaction.ToolSpec, name string) bool {
	for _, s := range specs {
		if s.Name == name {
			return true
		}
	}
	return false
}

func TestInteractionBackend_AskSurvivesTurnEnded(t *testing.T) {
	srv := &Server{
		runs:         newChatRuns(),
		hub:          sessionhub.New("test", 0),
		interactions: newInteractionStore(),
	}
	run := srv.runs.register("r2", "s-r2", "ws1", func() {})
	b := &interactionBackend{runs: srv.runs, apiSrv: srv}
	ctx, cancel := context.WithCancel(context.Background())

	// Codex can report turn.completed while the MCP request is still blocked.
	// Closing run.done must not cancel the interaction before the user answers.
	go func() {
		for {
			srv.interactions.mu.Lock()
			var id string
			for _, interactions := range srv.interactions.bySession {
				for id = range interactions {
					break
				}
			}
			srv.interactions.mu.Unlock()
			if id != "" {
				cancel() // Codex closes the MCP HTTP request just before run teardown.
				srv.runs.unregister("r2")
				time.Sleep(20 * time.Millisecond)
				srv.resolveInteraction("ws1", "s-r2", id, "answered after turn completion", "test")
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	res, err := b.Call(ctx, run.token, "ask_user", json.RawMessage(`{"question":"q"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError || res.Text != "answered after turn completion" {
		t.Fatalf("ask was cancelled by turn completion: %+v", res)
	}
}

func TestInteractionBackend_Todo(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("r3", "s-r3", "ws1", func() {})
	defer runs.unregister("r3")
	steps, mu := captureRun(run)
	b := &interactionBackend{runs: runs}

	res, err := b.Call(context.Background(), run.token, "todo_write",
		json.RawMessage(`{"todos":[{"content":"step one","status":"in_progress"},{"content":"step two","status":"pending"}]}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError || !strings.Contains(res.Text, "Checklist updated") {
		t.Fatalf("todo should succeed with confirmation text: %+v", res)
	}
	// No live emit: the CLI trace surfaces the checklist card.
	mu.Lock()
	defer mu.Unlock()
	if len(*steps) != 0 {
		t.Fatalf("todo should not emit a live step, got %d", len(*steps))
	}
}

func TestInteractionBackend_Confirm(t *testing.T) {
	srv := &Server{
		runs:         newChatRuns(),
		hub:          sessionhub.New("test", 0),
		interactions: newInteractionStore(),
	}
	run := srv.runs.register("rc", "s-rc", "ws1", func() {})
	defer srv.runs.unregister("rc")
	b := &interactionBackend{runs: srv.runs, apiSrv: srv}

	// Resolve the confirm interaction with "Onayla" as soon as it opens on the hub.
	_, ch, _ := srv.hub.Subscribe("ws1", "s-rc")
	go func() {
		for ev := range ch {
			if ev.Kind != sessionhub.KindInteractionOpen {
				continue
			}
			var p struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(ev.Payload, &p) != nil {
				return
			}
			srv.resolveInteraction("ws1", "s-rc", p.ID, "Onayla", "test")
			return
		}
	}()
	res, err := b.Call(context.Background(), run.token, "mcp__tionharness_interaction__request_confirmation",
		json.RawMessage(`{"question":"Delete the file?"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError || res.Text != "confirmed" {
		t.Fatalf("want confirmed, got %+v", res)
	}
}

// fakeSink is a minimal ArtifactSink for the artifact dispatch test.
type fakeSink struct{ created, updated int }

func (f *fakeSink) CreateArtifact(_ context.Context, spec tools.CreateArtifactSpec) (tools.ArtifactRef, error) {
	f.created++
	return tools.ArtifactRef{ID: "art-1", Title: spec.Title, Kind: spec.Kind}, nil
}
func (f *fakeSink) UpdateArtifact(_ context.Context, id, content string) (tools.ArtifactRef, error) {
	f.updated++
	return tools.ArtifactRef{ID: id, Title: "t", Kind: "markdown"}, nil
}

func TestInteractionBackend_Artifact(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("ra", "s-ra", "ws1", func() {})
	defer runs.unregister("ra")
	sink := &fakeSink{}
	run.setArtifacts(sink)
	b := &interactionBackend{runs: runs}

	res, err := b.Call(context.Background(), run.token, "create_artifact",
		json.RawMessage(`{"title":"Doc","kind":"markdown","content":"# Hi"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError || sink.created != 1 || !strings.Contains(res.Text, "art-1") {
		t.Fatalf("create_artifact failed: %+v created=%d", res, sink.created)
	}

	// No sink installed -> graceful error result.
	run.setArtifacts(nil)
	res2, _ := b.Call(context.Background(), run.token, "update_artifact",
		json.RawMessage(`{"id":"art-1","content":"x"}`))
	if !res2.IsError {
		t.Fatalf("want error when no sink, got %+v", res2)
	}
}

// TestInteractionBackend_AutonomousAskBails locks the headless-run guarantee: an
// autonomous run (scheduler/spawn) has no live client, so ask_user must
// return an error result at once instead of blocking until the 15-minute timeout.
func TestInteractionBackend_AutonomousAskBails(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("rauto", "s-rauto", "ws1", func() {})
	defer runs.unregister("rauto")
	run.autonomous = true
	b := &interactionBackend{runs: runs}

	res, err := b.Call(context.Background(), run.token, "ask_user", json.RawMessage(`{"question":"q"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError {
		t.Fatalf("autonomous ask_user must bail with an error result, got %+v", res)
	}
}

func TestInteractionBackend_UnknownToken(t *testing.T) {
	runs := newChatRuns()
	b := &interactionBackend{runs: runs}
	if _, err := b.Call(context.Background(), "ghost", "ask_user", json.RawMessage(`{"question":"q"}`)); err == nil {
		t.Fatal("want error for unknown token")
	}
}

// TestBridgedShellAdvertisesNoCompress: the bridged Bash/PowerShell schema must
// declare no_compress. The bridge builds its definition from a bare, filter-less
// tool while the real filter is installed per turn by NewShellRunner — so the flag
// works at call time, but the schema omitted it AND set additionalProperties:false,
// making it forbidden.
//
// That contradicted the optimizer's own degraded note, which tells the agent to
// "re-run the SAME command with no_compress: true" when a rewritten command fails.
// A live turn on 2026-07-31 followed that instruction, was rejected, and fell back
// to `> file 2>&1` — the recovery path advertised in one place and blocked in
// another.
func TestBridgedShellAdvertisesNoCompress(t *testing.T) {
	for _, def := range []providers.ToolDef{
		tools.NewShellTool(tools.Sandbox{}).AdvertiseOptimizerFlag().Def(),
		tools.NewPowerShellTool(tools.Sandbox{}).AdvertiseOptimizerFlag().Def(),
	} {
		schema := string(def.InputSchema)
		if !strings.Contains(schema, `"no_compress"`) {
			t.Errorf("%s: bridged schema must declare no_compress, got %s", def.Name, schema)
		}
		// Without the opt-in the flag stays hidden, so the native loop keeps its
		// minimal schema when no optimizer is wired.
	}
	bare := string(tools.NewShellTool(tools.Sandbox{}).Def().InputSchema)
	if strings.Contains(bare, `"no_compress"`) {
		t.Errorf("a bare tool with no filter and no opt-in must NOT advertise no_compress: %s", bare)
	}
}

// TestBareToolNameStripsRepeatedNamespace covers the doubled-prefix name a model
// produces when it re-namespaces an already namespaced tool: stripping must be
// idempotent, otherwise activation rejects the tool as unknown.
func TestBareToolNameStripsRepeatedNamespace(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"bare", "ask_user", "ask_user"},
		{"core", interactionNSPrefix + "ask_user", "ask_user"},
		{"extended", extendedNSPrefix + "create_agent", "create_agent"},
		{"doubled core", interactionNSPrefix + interactionNSPrefix + "ask_user", "ask_user"},
		{"doubled extended", extendedNSPrefix + extendedNSPrefix + "create_agent", "create_agent"},
		{"mixed tiers", interactionNSPrefix + extendedNSPrefix + "create_agent", "create_agent"},
		{"tripled", interactionNSPrefix + interactionNSPrefix + interactionNSPrefix + "ask_user", "ask_user"},
		{"foreign server untouched", "mcp__other_server__ask_user", "mcp__other_server__ask_user"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bareToolName(tc.in); got != tc.want {
				t.Errorf("bareToolName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// newGuidanceBackend builds a bound gateway backend + run for the wrong-loader tests.
func newGuidanceBackend(t *testing.T) (*interactionBackend, string, *chatRun) {
	t.Helper()
	runs := newChatRuns()
	b := &interactionBackend{runs: runs, tun: agent.NewTunables()}
	run := runs.register("r-guide", "s-guide", "ws-guide", func() {})
	tok := runs.interactionToken("ws-guide", "s-guide", "a-guide")
	runs.bindActive(tok, run)
	return b, tok, run
}

// TestGatewayActivateRedirectsExternalMCPName: an EXTERNAL mcp__<server>__ name handed
// to activate_tools must be pointed at the CLI's ToolSearch loader instead of vanishing
// into the generic "unknown name" list, which left the model retrying the wrong loader
// for three turns (WS20/SES79).
func TestGatewayActivateRedirectsExternalMCPName(t *testing.T) {
	b, tok, run := newGuidanceBackend(t)

	res, err := b.callActivateNames(tok, run, []string{"mcp__codebase-memory-mcp__search_graph"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "ToolSearch") {
		t.Fatalf("external MCP name must be redirected to ToolSearch, got:\n%s", res.Text)
	}
	if !strings.Contains(res.Text, "select:mcp__codebase-memory-mcp__search_graph") {
		t.Fatalf("redirect must spell out the select: query, got:\n%s", res.Text)
	}
	if !res.IsError {
		t.Fatalf("nothing activated + an unusable name must stay an error, got:\n%s", res.Text)
	}
}

// TestGatewayToolSearchRedirectsSelectQuery: the gateway tool_search must never answer a
// `select:` query over OUR namespace with an empty/blank result — that silence is the
// actual bug from SES79. It has to name activate_tools as the right loader.
func TestGatewayToolSearchRedirectsSelectQuery(t *testing.T) {
	b, _, run := newGuidanceBackend(t)

	res := b.callToolSearch(run, json.RawMessage(`{"query":"select:mcp__tionharness_extended__list_tasks"}`))
	if strings.TrimSpace(res.Text) == "" {
		t.Fatal("tool_search must not return an empty result for a select: query")
	}
	if !strings.Contains(res.Text, "activate_tools") {
		t.Fatalf("tool_search must redirect to activate_tools, got:\n%s", res.Text)
	}
}
