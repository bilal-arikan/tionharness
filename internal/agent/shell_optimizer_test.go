package agent

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestSqzShellFilter_Gate locks the opt-in gate: no filter without an sqz hook, and
// (when sqz is installed) a live filter once the hook is wired. The filter must also
// stay nil when sqz is opted-in but the binary is missing — never a broken filter.
func TestSqzShellFilter_Gate(t *testing.T) {
	r := lifecycleRuntime(t)

	// No sqz hook wired → no filter (feature is off).
	if f := r.sqzShellFilter(context.Background()); f != nil {
		t.Fatal("sqzShellFilter must be nil when no sqz hook is wired")
	}

	// Opt in by wiring the sqz hook (mirrors the real HOK1 "sqz hook claude").
	seedHook(t, r, db.HookPreToolUse, "Bash", "sqz hook claude")
	f := r.sqzShellFilter(context.Background())

	if _, err := exec.LookPath("sqz"); err != nil {
		if f != nil {
			t.Fatal("with sqz opted-in but not on PATH, the filter must be nil (never broken)")
		}
		t.Skip("sqz not on PATH; live-filter path not exercised")
	}
	if f == nil {
		t.Fatal("with the sqz hook wired and sqz on PATH, the filter must be non-nil")
	}

	// Smoke: highly repetitive input must come back SHORTER (sqz self-gates, so the
	// content has to be clearly compressible). The "[sqz] N/N tokens" stats line goes
	// to stderr (not returned); stdout carries the lossless legend + abbreviated body,
	// so proving len(out) < len(in) confirms the binary ran and compressed end-to-end.
	big := strings.Repeat("2026-07-25 INFO handler=chat session=S action=process status=ok\n", 400)
	out := f("cat big.log", big)
	if strings.TrimSpace(out) == "" {
		t.Fatal("filter returned empty output")
	}
	if len(out) >= len(big) {
		t.Errorf("expected sqz to compress repetitive input, got %d bytes from %d:\n%.200s",
			len(out), len(big), out)
	}

	// Per-workspace override (WSSettings.ShellOutputCompression): off disables even
	// with the hook wired; on forces it regardless of hooks (sqz is on PATH here).
	r.SetShellCompression("off")
	if r.sqzShellFilter(context.Background()) != nil {
		t.Fatal("shellOutputCompression=off must disable the filter despite the sqz hook")
	}
	r.SetShellCompression("on")
	if r.sqzShellFilter(context.Background()) == nil {
		t.Fatal("shellOutputCompression=on must enable the filter when sqz is on PATH")
	}
	r.SetShellCompression("") // restore auto
}
