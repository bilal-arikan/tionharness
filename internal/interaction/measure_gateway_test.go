package interaction

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// measBackend advertises a fixed, realistic extended surface (extCount tools with
// moderate schemas) so we can measure the CLI token cost of the extended tier being
// FULL (historical) vs EMPTY (gateway dynamic surface). Core is a tiny always-loaded set.
type measBackend struct {
	token    string
	extCount int
}

func (b *measBackend) Valid(token string) bool { return token == b.token }

func (b *measBackend) Tools(_, tier string) []ToolSpec {
	switch tier {
	case "core":
		// A couple of always-loaded tools + the gateway meta-tool, like the real core.
		return []ToolSpec{
			{Name: "ask_user", Description: "Ask the user a question and wait for the answer.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"question":{"type":"string"}},"required":["question"]}`)},
			{Name: "activate_tools", Description: "Load an on-demand tool into this session.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"tools":{"type":"array","items":{"type":"string"}}},"required":["tools"]}`)},
		}
	case "extended":
		return measExtendedTools(b.extCount)
	default:
		return nil
	}
}

func (b *measBackend) Call(_ context.Context, _, name string, _ json.RawMessage) (CallResult, error) {
	return CallResult{Text: "ok:" + name}, nil
}

// measExtendedTools builds n plausible self-management-style tool specs, each with a
// short description and a small object schema — representative of TionSwarm's real
// extended tier (session lifecycle, config, secrets, cross-session, ...).
func measExtendedTools(n int) []ToolSpec {
	if n == 0 {
		return nil
	}
	verbs := []string{"set", "get", "list", "update", "create", "delete", "search", "toggle", "read", "archive"}
	nouns := []string{"session_goal", "session_title", "working_dir", "tags", "config", "secret", "server", "skill", "schedule", "hook", "flow", "agent", "workspace", "label", "status"}
	out := make([]ToolSpec, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("%s_%s_%d", verbs[i%len(verbs)], nouns[i%len(nouns)], i)
		desc := fmt.Sprintf("Self-management tool %d: %s the %s for this workspace/session. "+
			"Use when the user asks to %s their %s. Returns a short status string.",
			i, verbs[i%len(verbs)], nouns[i%len(nouns)], verbs[i%len(verbs)], nouns[i%len(nouns)])
		schema := fmt.Sprintf(`{"type":"object","properties":{`+
			`"id":{"type":"string","description":"Target id for %s"},`+
			`"value":{"type":"string","description":"New value to apply"},`+
			`"dry_run":{"type":"boolean","description":"Preview only, do not mutate"}},"required":["id"]}`, nouns[i%len(nouns)])
		out = append(out, ToolSpec{Name: name, Description: desc, InputSchema: json.RawMessage(schema)})
	}
	return out
}

type measUsage struct {
	Input       int `json:"input_tokens"`
	CacheCreate int `json:"cache_creation_input_tokens"`
	CacheRead   int `json:"cache_read_input_tokens"`
	Output      int `json:"output_tokens"`
}

// TestMeasureGatewaySavings measures the CLI token cost of the extended tier being FULL
// (N advertised tools, historical) vs EMPTY (gateway dynamic surface). A trivial,
// tool-free prompt isolates the schema/catalog cost: the only difference between the two
// runs is how many extended tools the server advertises. Spends tokens; gated behind
// TIONSWARM_LIVE_CLI=1.
//
//	TIONSWARM_LIVE_CLI=1 go test ./internal/interaction/ -run TestMeasureGatewaySavings -v
func TestMeasureGatewaySavings(t *testing.T) {
	if os.Getenv("TIONSWARM_LIVE_CLI") != "1" {
		t.Skip("set TIONSWARM_LIVE_CLI=1 to run the gateway token measurement")
	}
	bin := "claude"
	if p := os.Getenv("TIONSWARM_CLAUDE_BIN"); p != "" {
		bin = p
	}
	model := "claude-fable-5"
	if m := os.Getenv("TIONSWARM_CLAUDE_MODEL"); m != "" {
		model = m
	}
	extN := 30 // representative extended surface size

	run := func(label string, extCount int) measUsage {
		backend := &measBackend{token: "meas-tok", extCount: extCount}
		srv := NewServer(backend, nil)
		ts := httptest.NewServer(srv)
		defer ts.Close()

		cfg := map[string]any{"mcpServers": map[string]any{
			"gwcore": map[string]any{"type": "http", "url": ts.URL + "/core",
				"headers": map[string]string{"Authorization": "Bearer meas-tok"}, "alwaysLoad": true},
			"gwext": map[string]any{"type": "http", "url": ts.URL + "/extended",
				"headers": map[string]string{"Authorization": "Bearer meas-tok"}},
		}}
		cfgBytes, _ := json.MarshalIndent(cfg, "", "  ")
		cfgFile, _ := os.CreateTemp(t.TempDir(), "meas-*.json")
		cfgFile.Write(cfgBytes)
		cfgFile.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		// Unique marker per label so the two runs do NOT share a prompt cache prefix.
		prompt := fmt.Sprintf("[run:%s] Reply with exactly the word DONE and nothing else. Do not use any tools.", label)
		args := []string{"-p", prompt,
			"--mcp-config", cfgFile.Name(), "--strict-mcp-config",
			"--allowedTools", "mcp__gwcore__ask_user", "mcp__gwcore__activate_tools", "mcp__gwext",
			"--model", model, "--output-format", "stream-json", "--verbose"}
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(os.Environ(), "ENABLE_TOOL_SEARCH=auto")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("[%s] claude err=%v", label, err)
		}
		u := parseFinalUsage(string(out))
		t.Logf("[%s] extCount=%d  input=%d cacheCreate=%d cacheRead=%d output=%d (prefix≈input+cacheCreate=%d)",
			label, extCount, u.Input, u.CacheCreate, u.CacheRead, u.Output, u.Input+u.CacheCreate)
		return u
	}

	full := run("FULL", extN)
	time.Sleep(2 * time.Second)
	empty := run("EMPTY", 0)

	fullPrefix := full.Input + full.CacheCreate
	emptyPrefix := empty.Input + empty.CacheCreate
	saved := fullPrefix - emptyPrefix
	fmt.Fprintf(os.Stdout, "\n=== GATEWAY TOKEN MEASUREMENT (extN=%d, model=%s) ===\n", extN, model)
	fmt.Fprintf(os.Stdout, "FULL  extended: prefix(input+cacheCreate)=%d  (input=%d cacheCreate=%d cacheRead=%d)\n",
		fullPrefix, full.Input, full.CacheCreate, full.CacheRead)
	fmt.Fprintf(os.Stdout, "EMPTY extended: prefix(input+cacheCreate)=%d  (input=%d cacheCreate=%d cacheRead=%d)\n",
		emptyPrefix, empty.Input, empty.CacheCreate, empty.CacheRead)
	fmt.Fprintf(os.Stdout, "SAVED (FULL-EMPTY) fresh prefix tokens: %d\n", saved)
}

// parseFinalUsage extracts the usage block from the stream-json "result" envelope.
func parseFinalUsage(streamJSON string) measUsage {
	var last measUsage
	for _, ln := range strings.Split(streamJSON, "\n") {
		ln = strings.TrimSpace(ln)
		if !strings.Contains(ln, `"type":"result"`) {
			continue
		}
		var env struct {
			Usage measUsage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(ln), &env); err == nil {
			last = env.Usage
		}
	}
	return last
}
