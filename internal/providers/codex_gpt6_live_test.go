package providers

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// This optional test consumes three small model requests. It uses the production
// transport with disposable shadow homes and a temporary working directory;
// no repository content is sent and no tools are requested.
func TestCodexGPT6Live(t *testing.T) {
	home := os.Getenv("TIONHARNESS_CODEX_GPT6_LIVE_HOME")
	if home == "" {
		t.Skip("set TIONHARNESS_CODEX_GPT6_LIVE_HOME to an authenticated Codex home to opt in")
	}
	bin, err := exec.LookPath("codex")
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"gpt-6.1-sol", "gpt-6-astra", "gpt-6-sol"} {
		t.Run(model, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			p := NewCodexCLI(bin, "", home)
			response, err := p.Complete(ctx, Request{
				Model: model, CLIEffortLevel: "low", PermissionMode: "read-only",
				WorkDir:  t.TempDir(),
				System:   "Return exactly GPT6_OK. Do not call tools or access files.",
				Messages: []Message{{Role: "user", Text: "Reply with GPT6_OK only."}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(response.Text) != "GPT6_OK" {
				t.Fatalf("unexpected reply: %q", response.Text)
			}
			t.Logf("%s completed through TionHarness Codex transport", model)
		})
	}
}
