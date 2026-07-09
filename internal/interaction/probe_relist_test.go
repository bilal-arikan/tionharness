package interaction

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// probeBackend is a live-CLI harness backend that answers the ONE question the
// blocking PushToolsChangedAndWait fix hinges on: when activate_tools registers a
// deferred tool and pushes tools/list_changed, does claude-cli re-fetch tools/list
// for the extended tier WHILE the activate tools/call is still pending (Hypothesis
// A → a blocking wait is safe + effective), or only AFTER activate returns
// (Hypothesis B → blocking just adds latency and cannot help)?
//
// The failed racing call is NOT visible server-side: if the tool isn't in the CLI's
// registry yet, the CLI rejects the model's tool_use itself and never sends a
// tools/call. So the measurement is done entirely from the server: on activate we
// push, then HOLD the response for up to holdMs while watching whether a
// tools/list(extended) arrives during the hold (Tools(_, "extended") signals the
// live waiter). "A" = re-list observed during the hold; "B" = timed out.
type probeBackend struct {
	token  string
	srv    *Server
	holdMs int

	mu     sync.Mutex
	active map[string]bool  // extended tools turned on via activate_tools
	waiter chan time.Time   // non-nil only during an activate hold; signalled by Tools(extended)
	log    func(string)
}

func newProbeBackend(token string, holdMs int, log func(string)) *probeBackend {
	return &probeBackend{token: token, holdMs: holdMs, active: map[string]bool{}, log: log}
}

func (b *probeBackend) Valid(token string) bool { return token == b.token }

func (b *probeBackend) Tools(_, tier string) []ToolSpec {
	switch tier {
	case "core":
		return []ToolSpec{
			{Name: "ask_user", Description: "Ask the user a question and wait.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"question":{"type":"string"}},"required":["question"]}`)},
			{Name: "activate_tools", Description: "Load one or more on-demand tools into this session by name.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"tools":{"type":"array","items":{"type":"string"}}},"required":["tools"]}`)},
		}
	case "extended":
		// A re-fetch of the extended tier: signal any live activate hold that the
		// client re-listed (this is the exact event the fix would wait on).
		b.mu.Lock()
		if b.waiter != nil {
			select {
			case b.waiter <- time.Now():
			default:
			}
		}
		names := make([]string, 0, len(b.active))
		for n := range b.active {
			names = append(names, n)
		}
		b.mu.Unlock()
		out := make([]ToolSpec, 0, len(names))
		for _, n := range names {
			out = append(out, ToolSpec{Name: n, Description: "On-demand tool " + n + ": returns a short status string.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)})
		}
		return out
	default:
		return nil
	}
}

func (b *probeBackend) Call(_ context.Context, token, name string, args json.RawMessage) (CallResult, error) {
	if name != "activate_tools" {
		// A successful (post-re-list) call to the target tool, or any other call.
		return CallResult{Text: "ok:" + name}, nil
	}
	var in struct {
		Tools []string `json:"tools"`
	}
	_ = json.Unmarshal(args, &in)

	// Register the newly requested tools so the next tools/list(extended) advertises
	// them, exactly like the real gateway's activateExtended.
	wait := make(chan time.Time, 1)
	b.mu.Lock()
	for _, t := range in.Tools {
		b.active[strings.TrimPrefix(t, "mcp__gwext__")] = true
	}
	b.waiter = wait
	b.mu.Unlock()

	t0 := time.Now()
	pushed := b.srv.PushToolsChanged(token)

	verdict := ""
	if !pushed {
		verdict = "NO-STREAM (push had no open SSE stream)"
	} else {
		select {
		case ts := <-wait:
			verdict = fmt.Sprintf("A: re-list arrived while activate PENDING (+%v)", ts.Sub(t0).Round(time.Millisecond))
		case <-time.After(time.Duration(b.holdMs) * time.Millisecond):
			verdict = fmt.Sprintf("B: NO re-list during %dms hold", b.holdMs)
		}
	}
	b.mu.Lock()
	b.waiter = nil
	b.mu.Unlock()
	b.log(verdict)

	callable := make([]string, len(in.Tools))
	for i, t := range in.Tools {
		callable[i] = "mcp__gwext__" + strings.TrimPrefix(t, "mcp__gwext__")
	}
	return CallResult{Text: "activated: " + strings.Join(callable, ", ") + "\nCall each by this exact (namespaced) name."}, nil
}

// TestProbeRelistOrdering measures whether claude-cli re-fetches tools/list for the
// extended tier WHILE an activate_tools call is still pending — the precondition for
// the blocking PushToolsChangedAndWait fix. It runs the real CLI N times (each a
// fresh process forced to activate-then-call) and tallies A (concurrent re-list,
// fix is safe+effective) vs B (serialized, fix would only add latency). Spends
// tokens + needs a logged-in claude-home; gated behind TIONSWARM_LIVE_CLI=1.
//
//	TIONSWARM_LIVE_CLI=1 go test ./internal/interaction/ -run TestProbeRelistOrdering -v -timeout 20m
//
// Tunables: TIONSWARM_PROBE_ITERS (default 5), TIONSWARM_PROBE_HOLD_MS (default 1500),
// TIONSWARM_CLAUDE_BIN, TIONSWARM_CLAUDE_MODEL (default claude-fable-5).
func TestProbeRelistOrdering(t *testing.T) {
	if os.Getenv("TIONSWARM_LIVE_CLI") != "1" {
		t.Skip("set TIONSWARM_LIVE_CLI=1 to run the live re-list ordering probe")
	}
	bin := "claude"
	if p := os.Getenv("TIONSWARM_CLAUDE_BIN"); p != "" {
		bin = p
	}
	model := "claude-fable-5"
	if m := os.Getenv("TIONSWARM_CLAUDE_MODEL"); m != "" {
		model = m
	}
	iters := envInt("TIONSWARM_PROBE_ITERS", 5)
	holdMs := envInt("TIONSWARM_PROBE_HOLD_MS", 1500)

	const token = "probe-tok"
	var aCount, bCount, noStream, raceSeen int
	var deltas []time.Duration

	for i := 0; i < iters; i++ {
		var verdict string
		backend := newProbeBackend(token, holdMs, func(s string) { verdict = s })
		srv := NewServer(backend, nil)
		backend.srv = srv
		ts := httptest.NewServer(srv)

		cfg := map[string]any{"mcpServers": map[string]any{
			"gwcore": map[string]any{"type": "http", "url": ts.URL + "/core",
				"headers": map[string]string{"Authorization": "Bearer " + token}, "alwaysLoad": true},
			"gwext": map[string]any{"type": "http", "url": ts.URL + "/extended",
				"headers": map[string]string{"Authorization": "Bearer " + token}},
		}}
		cfgBytes, _ := json.MarshalIndent(cfg, "", "  ")
		cfgFile, _ := os.CreateTemp(t.TempDir(), "probe-*.json")
		cfgFile.Write(cfgBytes)
		cfgFile.Close()

		// Force the exact activate→call sequence that triggers the race. A unique marker
		// per iteration avoids a shared prompt-cache prefix.
		prompt := fmt.Sprintf("[probe:%d] Do EXACTLY this, with no commentary: "+
			"first call activate_tools with tools=[\"list_tasks\"]; "+
			"then immediately call mcp__gwext__list_tasks with {}. "+
			"If the second call errors with \"No such tool available\", call it once more. "+
			"Then reply DONE.", i)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		args := []string{"-p", prompt,
			"--mcp-config", cfgFile.Name(), "--strict-mcp-config",
			"--allowedTools", "mcp__gwcore__ask_user", "mcp__gwcore__activate_tools", "mcp__gwext",
			"--model", model, "--output-format", "stream-json", "--verbose"}
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(os.Environ(), "ENABLE_TOOL_SEARCH=auto")
		out, err := cmd.CombinedOutput()
		cancel()
		ts.Close()
		if err != nil {
			t.Logf("[probe:%d] claude err=%v (tail: %s)", i, err, tail(string(out), 400))
		}

		// The CLI rejects an un-registered tool with this text before ever calling the
		// server — its presence confirms the race actually fired this run.
		race := strings.Contains(out2(out), "no such tool available")
		if race {
			raceSeen++
		}
		switch {
		case strings.HasPrefix(verdict, "A:"):
			aCount++
			if d, ok := parseDelta(verdict); ok {
				deltas = append(deltas, d)
			}
		case strings.HasPrefix(verdict, "B:"):
			bCount++
		default:
			noStream++
		}
		t.Logf("[probe:%d] verdict=%q raceReproduced=%v", i, verdict, race)
	}

	fmt.Fprintf(os.Stdout, "\n=== RE-LIST ORDERING PROBE (iters=%d, hold=%dms, model=%s) ===\n", iters, holdMs, model)
	fmt.Fprintf(os.Stdout, "A (concurrent re-list, fix SAFE+EFFECTIVE): %d\n", aCount)
	fmt.Fprintf(os.Stdout, "B (serialized, fix ONLY ADDS LATENCY):      %d\n", bCount)
	fmt.Fprintf(os.Stdout, "no-stream / inconclusive:                   %d\n", noStream)
	fmt.Fprintf(os.Stdout, "race reproduced (CLI 'No such tool'):       %d/%d\n", raceSeen, iters)
	if len(deltas) > 0 {
		fmt.Fprintf(os.Stdout, "observed re-list delay during hold: min=%v med=%v max=%v\n",
			minDur(deltas), medDur(deltas), maxDur(deltas))
	}
	switch {
	case aCount >= iters-noStream && aCount > 0:
		fmt.Fprintf(os.Stdout, "DECISION → IMPLEMENT PushToolsChangedAndWait (timeout ≈ 2×max observed delay).\n")
	case bCount > 0 && aCount == 0:
		fmt.Fprintf(os.Stdout, "DECISION → DO NOT implement blocking (CLI serializes; keep benign retry).\n")
	default:
		fmt.Fprintf(os.Stdout, "DECISION → mixed/inconclusive; raise iters and re-run.\n")
	}
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func out2(b []byte) string { return strings.ToLower(string(b)) }

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

func parseDelta(v string) (time.Duration, bool) {
	i := strings.Index(v, "+")
	j := strings.Index(v, ")")
	if i < 0 || j < 0 || j <= i {
		return 0, false
	}
	d, err := time.ParseDuration(v[i+1 : j])
	return d, err == nil
}

func minDur(ds []time.Duration) time.Duration {
	m := ds[0]
	for _, d := range ds {
		if d < m {
			m = d
		}
	}
	return m
}
func maxDur(ds []time.Duration) time.Duration {
	m := ds[0]
	for _, d := range ds {
		if d > m {
			m = d
		}
	}
	return m
}
func medDur(ds []time.Duration) time.Duration {
	c := append([]time.Duration(nil), ds...)
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && c[j-1] > c[j]; j-- {
			c[j-1], c[j] = c[j], c[j-1]
		}
	}
	return c[len(c)/2]
}
