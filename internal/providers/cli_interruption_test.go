package providers

import (
	"strings"
	"testing"
	"time"
)

func TestCodexPartialReplyStallIsInterrupted(t *testing.T) {
	_, retryable, err := runCodexStallHelper(t, 150*time.Millisecond,
		`{"type":"item.completed","item":{"id":"a1","type":"agent_message","text":"I still need to write the required files."}}`)
	partial, interrupted := InterruptedResponse(err)
	if partial == nil || interrupted == nil {
		t.Fatalf("partial work was not preserved: %v", err)
	}
	if retryable {
		t.Fatal("partial turn must not silently restart")
	}
	if partial.StopReason != StopInterrupted || interrupted.Reason != "cli_idle_timeout" || interrupted.Window != 150*time.Millisecond {
		t.Fatalf("interruption = %+v, response = %+v", interrupted, partial)
	}
	if !strings.Contains(partial.Text, "required files") {
		t.Fatalf("partial reply lost: %+v", partial)
	}
}

func TestCLIStartupDiagnosticRedactsCredentialsAndBoundsUnicode(t *testing.T) {
	detail := safeCLIStartupDetail(`MCP handshake http://user:pass@localhost:8090/mcp?token=url-secret#private Authorization: Bearer bearer-secret api_key="key-secret" token=token-secret password=pass-secret`)
	for _, secret := range []string{"url-secret", "bearer-secret", "key-secret", "token-secret", "pass-secret", "user:pass", "#private"} {
		if strings.Contains(detail, secret) {
			t.Fatalf("credential %q leaked: %s", secret, detail)
		}
	}
	if !strings.Contains(detail, "localhost:8090/mcp") {
		t.Fatalf("diagnostic lost endpoint: %s", detail)
	}
	if len([]rune(safeCLIStartupDetail(strings.Repeat("ş", 2000)))) != 1600 {
		t.Fatal("diagnostic is not bounded by runes")
	}
}
