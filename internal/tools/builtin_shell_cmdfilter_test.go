package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// shellToolForTest returns a Bash tool bound to a temp sandbox, or skips.
func shellToolForTest(t *testing.T) ShellTool {
	t.Helper()
	sb := NewSandbox(t.TempDir())
	if !sb.Ready() {
		t.Skip("sandbox not ready in this environment")
	}
	tool := NewShellTool(sb)
	if !tool.Available() {
		t.Skip("no POSIX shell available on this host")
	}
	return tool
}

// TestShellCommandFilter_Rewrites proves the rewritten command is what actually
// runs — the filter must shape execution, not just be recorded.
func TestShellCommandFilter_Rewrites(t *testing.T) {
	var seen string
	tool := shellToolForTest(t).WithCommandFilter(func(cmd string) (string, *ShellOptimization) {
		seen = cmd
		return "echo REWRITTEN", &ShellOptimization{Kind: "rtk", Command: "echo REWRITTEN"}
	})

	ctx, sink := WithOptimizerSink(context.Background())
	out, err := tool.Call(ctx, json.RawMessage(`{"command":"echo ORIGINAL"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if seen != "echo ORIGINAL" {
		t.Errorf("filter should receive the command as typed, got %q", seen)
	}
	if !strings.Contains(out, "REWRITTEN") || strings.Contains(out, "ORIGINAL") {
		t.Errorf("the REWRITTEN command must be the one executed, got %q", out)
	}
	opt := sink.Take()
	if opt == nil || opt.Kind != "rtk" || opt.Command != "echo REWRITTEN" {
		t.Fatalf("the rewrite must be recorded so the UI can show what really ran, got %+v", opt)
	}
	if opt.Degraded {
		t.Error("a successful command must not be flagged degraded")
	}
}

// TestShellCommandFilter_DegradedOnFailure is the error-swallow guard. rtk can
// reduce a real failure to something uninformative ("No tests found" for a broken
// go.mod), so a FAILED rewritten command must carry an explicit recovery note
// rather than a confident summary of an error nobody can see.
func TestShellCommandFilter_DegradedOnFailure(t *testing.T) {
	tool := shellToolForTest(t).WithCommandFilter(func(cmd string) (string, *ShellOptimization) {
		// Stands in for rtk: prints a terse summary, exits non-zero.
		return "echo 'Go test: No tests found'; exit 1", &ShellOptimization{Kind: "rtk"}
	})

	ctx, sink := WithOptimizerSink(context.Background())
	out, err := tool.Call(ctx, json.RawMessage(`{"command":"go test ./..."}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(out, "optimizer note") || !strings.Contains(out, "no_compress") {
		t.Errorf("a failed rewritten command must tell the agent how to recover the raw output, got:\n%s", out)
	}
	if opt := sink.Take(); opt == nil || !opt.Degraded {
		t.Errorf("a failed rewritten command must be flagged degraded for the UI, got %+v", opt)
	}
}

// TestShellCommandFilter_NoCompressOptsOut: one flag, one concept. `no_compress`
// already means "give me this command untouched", so it must skip the command
// rewrite too — otherwise the escape hatch the degraded note points at would
// still hand back a rewritten command.
func TestShellCommandFilter_NoCompressOptsOut(t *testing.T) {
	called := false
	tool := shellToolForTest(t).WithCommandFilter(func(cmd string) (string, *ShellOptimization) {
		called = true
		return "echo REWRITTEN", &ShellOptimization{Kind: "rtk"}
	})

	out, err := tool.Call(context.Background(), json.RawMessage(`{"command":"echo ORIGINAL","no_compress":true}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if called {
		t.Error("no_compress must skip the command filter entirely")
	}
	if !strings.Contains(out, "ORIGINAL") {
		t.Errorf("no_compress must run the command as typed, got %q", out)
	}
}

// TestShellFilters_Compose: both optimizers acting on one call must produce ONE
// merged record. Recording twice would drop whichever half the second write did
// not carry — in particular the Degraded warning.
func TestShellFilters_Compose(t *testing.T) {
	tool := shellToolForTest(t).
		WithCommandFilter(func(cmd string) (string, *ShellOptimization) {
			// Emit enough bytes to clear the output filter's size threshold, then fail.
			return "yes abcdefghij | head -400; exit 1", &ShellOptimization{Kind: "rtk", Command: "rewritten"}
		}).
		WithOutputFilter(func(cmd, output string) (string, *ShellOptimization) {
			return "compressed", &ShellOptimization{Kind: "sqz", InTokens: 100, OutTokens: 10}
		})

	ctx, sink := WithOptimizerSink(context.Background())
	if _, err := tool.Call(ctx, json.RawMessage(`{"command":"go test ./..."}`)); err != nil {
		t.Fatalf("call: %v", err)
	}
	opt := sink.Take()
	if opt == nil {
		t.Fatal("expected a merged optimization record")
	}
	// sqz measured real tokens, so it owns the label and the counts...
	if opt.Kind != "sqz" || opt.Percent() != 90 {
		t.Errorf("the measured sqz record should win the label, got %+v (%d%%)", *opt, opt.Percent())
	}
	// ...but the command-layer facts must survive the merge.
	if opt.Command != "rewritten" {
		t.Errorf("the rewritten command must survive the merge, got %q", opt.Command)
	}
	if !opt.Degraded {
		t.Error("the Degraded flag must survive the merge — it is the one that warns about a hidden failure")
	}
}
