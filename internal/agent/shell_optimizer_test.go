package agent

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

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
	// content has to be clearly compressible). stdout carries the lossless legend +
	// abbreviated body, so proving len(out) < len(in) confirms the binary ran and
	// compressed end-to-end.
	//
	// The payload MUST be unique per run: sqz keeps a PERSISTENT dedup cache, and on
	// a repeat of content it has seen before it returns a "§ref:…§" handle and logs
	// "[sqz] dedup hit" INSTEAD of the "N/M tokens" stats line. With a fixed payload
	// this test therefore passed once and failed on every later run of the suite.
	marker := t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	big := strings.Repeat(marker+" 2026-07-25 INFO handler=chat session=S action=process status=ok\n", 400)
	out, opt := f("cat big.log", big)
	if strings.TrimSpace(out) == "" {
		t.Fatal("filter returned empty output")
	}
	if len(out) >= len(big) {
		t.Errorf("expected sqz to compress repetitive input, got %d bytes from %d:\n%.200s",
			len(out), len(big), out)
	}
	// The "[sqz] OUT/IN tokens" stats line rides stderr; parsing it is what makes the
	// UI chip a real measurement, so a compressed run MUST surface one.
	if opt == nil {
		t.Fatal("sqz compressed the output but reported no token measurement (stderr stats line not parsed)")
	}
	if opt.Kind != "sqz" || !opt.Measured() || opt.Percent() <= 0 {
		t.Fatalf("expected a measured sqz saving, got %+v (%d%%)", *opt, opt.Percent())
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
