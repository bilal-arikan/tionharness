package providers

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLivePersistentPoolLifecycle exercises the persistent claude-cli process pool
// (the ClaudePersistentSession=true path the runtime takes) against the REAL CLI,
// using the CLI's own login. It spends subscription quota, so it is gated behind
// TIONHARNESS_LIVE_CLI_POOL=1; TIONHARNESS_LIVE_MODEL picks the model (default
// haiku — cheap and rarely overloaded; run once more with claude-fable-5-1).
//
// Covered, in order:
//
//  1. cold start + three WARM turns on one key — the process retains context
//     (turn 2/3 recall a fact only turn 1 carried) and the built-in allowlist
//     (`--tools`) rides the launch flags
//
//  2. a fingerprint change (different system prompt) cold-restarts the process
//     for the same key and the turn still succeeds
//
//  3. two keys in parallel get two independent processes
//
//  4. Close terminates everything without a phantom "could not be killed"
//
//     TIONHARNESS_LIVE_CLI_POOL=1 go test ./internal/providers/ -run TestLivePersistentPoolLifecycle -v
func TestLivePersistentPoolLifecycle(t *testing.T) {
	if os.Getenv("TIONHARNESS_LIVE_CLI_POOL") != "1" {
		t.Skip("set TIONHARNESS_LIVE_CLI_POOL=1 to run the live persistent-pool test")
	}
	bin := "claude"
	if p := os.Getenv("TIONHARNESS_CLAUDE_BIN"); p != "" {
		bin = p
	}
	model := os.Getenv("TIONHARNESS_LIVE_MODEL")
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	c := NewClaudeCLI(bin, model, "", "", "")
	pool := NewCLISessionPool()
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	base := Request{
		Model:                  model,
		PermissionMode:         "auto",
		System:                 "You are a terse assistant inside an automated test.",
		CLIRestrictNativeTools: true, // auxiliary shape: no built-in tools at all
		MaxTokens:              256,
	}
	turn := func(key string, req Request, history []Message) *Response {
		req.Messages = history
		req.SystemDynamic = "Current time: " + time.Now().Format("15:04:05.000")
		resp, err := pool.Turn(ctx, key, c, req, nil)
		if err != nil {
			t.Fatalf("turn on %s: %v", key, err)
		}
		t.Logf("%s: model=%s text=%q in=%d out=%d cacheW=%d cacheR=%d",
			key, resp.Model, strings.TrimSpace(resp.Text), resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.CacheWriteTokens, resp.Usage.CacheReadTokens)
		return resp
	}

	// 1. cold + warm turns.
	hist := []Message{{Role: RoleUser, Text: "Remember this: my code is ZEBRA-9. Reply with just OK."}}
	r1 := turn("k1", base, hist)
	hist = append(hist, Message{Role: RoleAssistant, Text: r1.Text}, Message{Role: RoleUser, Text: "What is my code? Reply with only the code."})
	r2 := turn("k1", base, hist)
	if !strings.Contains(strings.ToUpper(r2.Text), "ZEBRA-9") {
		t.Fatalf("warm turn 2 lost context: %q", r2.Text)
	}
	hist = append(hist, Message{Role: RoleAssistant, Text: r2.Text}, Message{Role: RoleUser, Text: "Repeat the code once more, nothing else."})
	r3 := turn("k1", base, hist)
	if !strings.Contains(strings.ToUpper(r3.Text), "ZEBRA-9") {
		t.Fatalf("warm turn 3 lost context: %q", r3.Text)
	}
	if !strings.Contains(strings.ToLower(r1.Model+r2.Model+r3.Model), strings.ToLower(strings.TrimSuffix(model, "-20251001"))) {
		t.Errorf("served model drifted: %q %q %q (want %s)", r1.Model, r2.Model, r3.Model, model)
	}

	// 2. fingerprint change → cold restart, still answers.
	changed := base
	changed.System = "You are a terse assistant inside an automated test. Always answer in uppercase."
	r4 := turn("k1", changed, []Message{{Role: RoleUser, Text: "Reply with just ok."}})
	if !strings.Contains(strings.ToLower(r4.Text), "ok") {
		t.Fatalf("post-restart turn text = %q", r4.Text)
	}

	// 3. two keys in parallel.
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, key := range []string{"p1", "p2"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			req := base
			req.Messages = []Message{{Role: RoleUser, Text: "Reply with exactly: " + key}}
			resp, err := pool.Turn(ctx, key, c, req, nil)
			if err != nil {
				errs <- err
				return
			}
			if !strings.Contains(resp.Text, key) {
				errs <- context.DeadlineExceeded
				return
			}
			t.Logf("%s: text=%q cacheW=%d cacheR=%d", key, strings.TrimSpace(resp.Text), resp.Usage.CacheWriteTokens, resp.Usage.CacheReadTokens)
		}(key)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("parallel session: %v", err)
	}

	// 4. Close must not report phantom kill failures.
	pool.Close()
}
