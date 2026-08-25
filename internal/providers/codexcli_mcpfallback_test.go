package providers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCodexMCPStartupFailureRecognisesLiveWording(t *testing.T) {
	fail := []string{
		"codex CLI exited before producing any turn output (likely login, config, or MCP startup failure): exit status 1 required MCP servers failed to initialize",
		"unity-mcp: handshaking with MCP server failed",
		"ERROR: MCP client for `unity-mcp` failed to start: handshake timed out",
	}
	for _, msg := range fail {
		if !codexMCPStartupFailure(msg) {
			t.Errorf("codexMCPStartupFailure(%q) = false, want true", msg)
		}
	}
	pass := []string{
		"",
		"codex CLI failed: exit status 1 stream closed",
		// No "mcp" anywhere: an unrelated startup problem must not trigger the
		// fallback, which would silently strip the agent's tools.
		"sandbox failed to initialize",
	}
	for _, msg := range pass {
		if codexMCPStartupFailure(msg) {
			t.Errorf("codexMCPStartupFailure(%q) = true, want false", msg)
		}
	}
}

func TestCodexNamedMCPServersNarrowsToTheFailingServer(t *testing.T) {
	servers := map[string]CLIMCPServer{
		"unity-mcp":   {Transport: "http", URL: "http://127.0.0.1:8080/mcp"},
		"interaction": {Command: "tionharness", Args: []string{"mcp"}},
	}
	got := codexNamedMCPServers("unity-mcp: handshaking with MCP server failed", servers)
	if want := []string{"unity-mcp"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("named = %v, want %v", got, want)
	}
	// The aggregate message names nobody; the caller falls back to the remote set.
	if got := codexNamedMCPServers("required MCP servers failed to initialize", servers); len(got) != 0 {
		t.Fatalf("named = %v, want none", got)
	}
	if want := []string{"unity-mcp"}; !reflect.DeepEqual(codexRemoteServerKeys(servers), want) {
		t.Fatalf("remote keys = %v, want %v", codexRemoteServerKeys(servers), want)
	}
}

func TestCodexServersWithoutLeavesTheOriginalIntact(t *testing.T) {
	servers := map[string]CLIMCPServer{
		"unity-mcp":   {Transport: "http", URL: "http://127.0.0.1:8080/mcp"},
		"interaction": {Command: "tionharness"},
	}
	out := codexServersWithout(servers, []string{"unity-mcp"})
	if _, ok := out["unity-mcp"]; ok {
		t.Fatal("dropped server still present in the copy")
	}
	if _, ok := out["interaction"]; !ok {
		t.Fatal("stdio server was dropped too")
	}
	// The provider reuses its server map on every later turn, so a turn-local
	// removal must not mutate it.
	if _, ok := servers["unity-mcp"]; !ok {
		t.Fatal("codexServersWithout mutated the input map")
	}
}

// Regression (SES1066): a remote MCP server that answers the reachability probe
// but then fails the MCP handshake made codex abort the whole session with
// "required MCP servers failed to initialize" — the agent lost its turn over a
// tool it never asked for. Complete must drop that server and re-run the turn.
func TestCodexCompleteRetriesWithoutTheFailingMCPServer(t *testing.T) {
	if os.Getenv("CODEX_TEST_HELPER") != "" {
		return // helper re-exec
	}
	self, err := os.Executable()
	if err != nil {
		t.Skipf("no test binary path: %v", err)
	}

	home := t.TempDir()
	t.Setenv("CODEX_TEST_HELPER_CONFIG", "enabled")
	c := &CodexCLI{
		binPath:   self,
		configDir: home,
		model:     "gpt-test",
		mcpServers: map[string]CLIMCPServer{
			"unity-mcp": {Transport: "http", URL: "http://127.0.0.1:8080/mcp"},
		},
		// Keep the preflight offline and "healthy": this regression is precisely
		// the case where the probe passes and the handshake still fails.
		mcpProbe: func(context.Context, string) bool { return true },
	}
	args := []string{"-test.run=TestCodexHelperFailsWhileUnityMCPConfigured", "-test.v=false"}
	if _, _, _, err := writeCodexConfig(context.Background(), home, c.buildConfig(Request{}), c.mcpProbe); err != nil {
		t.Fatalf("write initial config: %v", err)
	}
	resp, _, runErr := c.runAttempt(context.Background(), args, "prompt", "gpt-test", Request{}, home)
	if runErr == nil {
		t.Fatal("helper did not fail while unity-mcp was configured; the fixture no longer reproduces the bug")
	}
	_ = resp

	// Now the full path: Complete writes the config, sees the failure, strips
	// unity-mcp and succeeds on the retry.
	c.binPath = self
	got, err := c.completeWithArgs(context.Background(), args, "prompt", "gpt-test", Request{})
	if err != nil {
		t.Fatalf("Complete did not recover from the MCP startup failure: %v", err)
	}
	if !strings.Contains(got.Text, "OK") {
		t.Fatalf("recovered turn text = %q, want the helper's answer", got.Text)
	}
	var noted bool
	for _, s := range got.Trace {
		if strings.Contains(s.Text, "failed to start and were disabled") && strings.Contains(s.Text, "unity-mcp") {
			noted = true
		}
	}
	if !noted {
		t.Fatal("the disabled MCP server was not reported in the trace: the capability loss is silent")
	}
}

// TestCodexHelperFailsWhileUnityMCPConfigured is not a real test: it is the fake
// codex binary the regression above runs. It fails exactly the way codex does
// while the broken server is still in config.toml, and produces a normal turn
// once it is gone.
func TestCodexHelperFailsWhileUnityMCPConfigured(t *testing.T) {
	if os.Getenv("CODEX_TEST_HELPER_CONFIG") == "" {
		t.Skip("helper: only runs under the re-exec in TestCodexCompleteRetriesWithoutTheFailingMCPServer")
	}
	home := os.Getenv("CODEX_HOME")
	path := filepath.Join(home, "config.toml")
	// The real codex reads config.toml some way into its startup, which is what
	// gives a concurrent turn time to overwrite the file. The concurrency
	// regression sets this to widen that window deterministically; the
	// single-turn test leaves it unset and pays nothing.
	if d, err := time.ParseDuration(os.Getenv("CODEX_TEST_HELPER_DELAY")); err == nil && d > 0 {
		time.Sleep(d)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "helper: read config: %v\n", err)
		os.Exit(1)
	}
	if strings.Contains(string(data), "unity-mcp") {
		fmt.Fprintln(os.Stderr, "ERROR: unity-mcp: handshaking with MCP server failed")
		fmt.Fprintln(os.Stderr, "required MCP servers failed to initialize")
		os.Exit(1)
	}
	fmt.Println(`{"type":"thread.started","thread_id":"t1"}`)
	fmt.Println(`{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"OK"}}`)
	fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":1,"cached_input_tokens":0,"cache_write_input_tokens":0,"output_tokens":1,"reasoning_output_tokens":0}}`)
	os.Exit(0)
}
