package procwatch

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestBeginListsRunningEntryWithOwnerFromContext(t *testing.T) {
	r := New(10)
	ctx := WithOwner(context.Background(), Owner{WorkspaceID: "WS1", SessionID: "SES1"})
	h := r.Begin(ctx, Meta{Kind: KindShell, Label: "Bash", Command: "go test ./...", Owner: Owner{AgentID: "AGT1"}})

	got := r.List(Filter{})
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d", len(got))
	}
	e := got[0]
	if e.Status != StatusRunning {
		t.Errorf("status = %q, want running", e.Status)
	}
	if e.Owner.WorkspaceID != "WS1" || e.Owner.SessionID != "SES1" || e.Owner.AgentID != "AGT1" {
		t.Errorf("owner = %+v, want context owner merged with the declared agent", e.Owner)
	}
	if e.ID != h.ID() {
		t.Errorf("entry id %q != handle id %q", e.ID, h.ID())
	}
}

func TestFinishClassifiesOutcome(t *testing.T) {
	exitErr := runExitCode(t, 3)
	deadline, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	cases := []struct {
		name     string
		ctx      context.Context
		stop     bool
		err      error
		status   Status
		exitCode int
	}{
		{name: "success", ctx: context.Background(), status: StatusSucceeded},
		{name: "non-zero exit", ctx: context.Background(), err: exitErr, status: StatusFailed, exitCode: 3},
		{name: "never started", ctx: context.Background(), err: errors.New("exec: not found"), status: StatusFailed, exitCode: -1},
		{name: "expired context", ctx: deadline, err: exitErr, status: StatusTimedOut, exitCode: 3},
		{name: "stopped", ctx: context.Background(), stop: true, err: exitErr, status: StatusKilled, exitCode: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := New(10)
			h := r.Begin(tc.ctx, Meta{Kind: KindShell, Command: "x", Stop: func() {}})
			if tc.stop {
				if ok, err := r.Stop(h.ID()); err != nil || !ok {
					t.Fatalf("Stop = (%v, %v), want (true, nil)", ok, err)
				}
			}
			h.Finish(tc.err)

			e, ok := r.Get(h.ID())
			if !ok {
				t.Fatalf("entry %q missing after finish", h.ID())
			}
			if e.Status != tc.status {
				t.Errorf("status = %q, want %q", e.Status, tc.status)
			}
			if e.ExitCode != tc.exitCode {
				t.Errorf("exit code = %d, want %d", e.ExitCode, tc.exitCode)
			}
			if e.EndedAt == 0 {
				t.Error("EndedAt not stamped")
			}
			if e.Stoppable {
				t.Error("a finished entry must not advertise itself as stoppable")
			}
		})
	}
}

func TestFinishIsIdempotent(t *testing.T) {
	r := New(10)
	h := r.Begin(context.Background(), Meta{Kind: KindShell, Command: "x"})
	h.Finish(nil)
	h.Finish(errors.New("late failure"))

	e, _ := r.Get(h.ID())
	if e.Status != StatusSucceeded {
		t.Errorf("status = %q, want the first outcome (succeeded) to stand", e.Status)
	}
	if n := len(r.List(Filter{})); n != 1 {
		t.Errorf("history holds %d entries, want 1", n)
	}
}

func TestFinishCodeMapsToStatus(t *testing.T) {
	r := New(10)
	ok := r.Begin(context.Background(), Meta{Kind: KindShellBackground, Command: "a"})
	bad := r.Begin(context.Background(), Meta{Kind: KindShellBackground, Command: "b"})
	ok.FinishCode(0)
	bad.FinishCode(2)

	okEntry, _ := r.Get(ok.ID())
	badEntry, _ := r.Get(bad.ID())
	if okEntry.Status != StatusSucceeded {
		t.Errorf("code 0 → %q, want succeeded", okEntry.Status)
	}
	if badEntry.Status != StatusFailed || badEntry.ExitCode != 2 {
		t.Errorf("code 2 → (%q, %d), want (failed, 2)", badEntry.Status, badEntry.ExitCode)
	}
}

func TestHistoryIsBoundedButRunningEntriesAreKept(t *testing.T) {
	r := New(2)
	live := r.Begin(context.Background(), Meta{Kind: KindShell, Command: "live"})
	for i := range 5 {
		h := r.Begin(context.Background(), Meta{Kind: KindShell, Command: "done"})
		_ = i
		h.Finish(nil)
	}
	got := r.List(Filter{})
	if len(got) != 3 {
		t.Fatalf("want 1 running + 2 retained finished entries, got %d", len(got))
	}
	if _, ok := r.Get(live.ID()); !ok {
		t.Error("the running entry was pruned out of the ledger")
	}
}

func TestFilterFacets(t *testing.T) {
	r := New(10)
	shell := r.Begin(context.Background(), Meta{Kind: KindShell, Command: "a", Owner: Owner{SessionID: "SES1"}})
	r.Begin(context.Background(), Meta{Kind: KindMCP, Command: "b", Owner: Owner{SessionID: "SES2"}})
	shell.Finish(nil)

	if got := r.List(Filter{Kinds: []Kind{KindMCP}}); len(got) != 1 || got[0].Kind != KindMCP {
		t.Errorf("kind filter returned %d rows, want the single mcp entry", len(got))
	}
	if got := r.List(Filter{SessionID: "SES1"}); len(got) != 1 || got[0].Owner.SessionID != "SES1" {
		t.Errorf("session filter returned %d rows, want the single SES1 entry", len(got))
	}
	if got := r.List(Filter{Statuses: []Status{StatusRunning}}); len(got) != 1 || got[0].Status != StatusRunning {
		t.Errorf("status filter returned %d rows, want the single running entry", len(got))
	}
	if got := r.List(Filter{Limit: 1}); len(got) != 1 {
		t.Errorf("limit 1 returned %d rows", len(got))
	}
}

func TestStopUnknownAndFinishedEntries(t *testing.T) {
	r := New(10)
	if _, err := r.Stop("nope"); err == nil {
		t.Error("stopping an unknown id must error")
	}
	h := r.Begin(context.Background(), Meta{Kind: KindShell, Command: "x", Stop: func() {}})
	h.Finish(nil)
	ok, err := r.Stop(h.ID())
	if err != nil || ok {
		t.Errorf("Stop on a finished entry = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestNotifyFiresOnStartAndFinish(t *testing.T) {
	r := New(10)
	var seen []Status
	r.SetNotify(func(e Entry) { seen = append(seen, e.Status) })
	h := r.Begin(context.Background(), Meta{Kind: KindShell, Command: "x"})
	h.Finish(nil)

	if len(seen) != 2 || seen[0] != StatusRunning || seen[1] != StatusSucceeded {
		t.Errorf("notify saw %v, want [running succeeded]", seen)
	}
}

func TestOutputTailIsBounded(t *testing.T) {
	r := New(10)
	h := r.Begin(context.Background(), Meta{Kind: KindShell, Command: "x"})
	for range 10 {
		h.AppendOutput(string(make([]byte, 1024)))
	}
	h.AppendOutput("tail-marker")
	h.Finish(nil)

	e, _ := r.Get(h.ID())
	if len(e.OutputTail) > OutputTailBytes {
		t.Errorf("tail is %d bytes, want at most %d", len(e.OutputTail), OutputTailBytes)
	}
	if got := e.OutputTail[len(e.OutputTail)-len("tail-marker"):]; got != "tail-marker" {
		t.Errorf("tail ends with %q, want the newest output", got)
	}
}

func TestNilRegistryAndHandleAreInert(t *testing.T) {
	var r *Registry
	h := r.Begin(context.Background(), Meta{Kind: KindShell, Command: "x"})
	h.Started(nil)
	h.AppendOutput("out")
	h.Finish(nil)
	if h.ID() != "" {
		t.Errorf("nil handle id = %q, want empty", h.ID())
	}
	if got := r.List(Filter{}); got != nil {
		t.Errorf("nil registry listed %v", got)
	}
}

// runExitCode returns a real *exec.ExitError carrying the given exit code, so
// the classification test exercises the same error shape cmd.Wait produces.
func runExitCode(t *testing.T, code int) error {
	t.Helper()
	cmd := exec.Command("cmd.exe", "/c", "exit", itoaTest(code))
	if _, err := exec.LookPath("cmd.exe"); err != nil {
		cmd = exec.Command("sh", "-c", "exit "+itoaTest(code))
	}
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected an ExitError from the probe command, got %v", err)
	}
	return err
}

func itoaTest(n int) string { return string(rune('0' + n)) }
