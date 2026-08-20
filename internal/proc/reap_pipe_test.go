package proc

import (
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

// Regression: a child's own children (a Gradle daemon, an MCP server) inherit
// the command's stdout pipe. When the context is cancelled, killing only the
// direct child leaves them alive holding the write end, so the reader never sees
// EOF and the calling turn wedges — the SES953 hang. TreeKill must reap the
// whole tree, which closes the pipe and unblocks the read.
func TestTreeKillReapsGrandchildHoldingPipe(t *testing.T) {
	if os.Getenv("PROC_TEST_HELPER") != "" {
		return // helper re-exec; see the helper tests below
	}

	self, err := os.Executable()
	if err != nil {
		t.Skipf("no test binary path: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Direct child: re-exec of this test binary. It spawns a long-lived
	// grandchild that inherits stdout, then sleeps itself, so at cancel time the
	// whole tree is alive — exactly the shape taskkill /T has to walk.
	cmd := CommandContext(ctx, self, "-test.run=TestProcHelperSpawnsLingeringChild", "-test.v=false")
	cmd.Env = append(os.Environ(), "PROC_TEST_HELPER=spawn", "PROC_TEST_SELF="+self)
	TreeKill(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Give the child time to spawn the grandchild before tearing the tree down.
	time.Sleep(2 * time.Second)
	cancel()

	// The read must reach EOF: it only can if the grandchild was killed too.
	readDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, stdout)
		close(readDone)
	}()

	select {
	case <-readDone:
	case <-time.After(30 * time.Second):
		t.Fatal("stdout never reached EOF after cancel: a grandchild survived the tree kill and still holds the pipe")
	}
	_ = cmd.Wait()
}

// TestProcHelperSpawnsLingeringChild is not a real test: it is the helper the
// regression above re-execs. It starts a grandchild that outlives the parent's
// own work, handing it the inherited stdout, then sleeps.
func TestProcHelperSpawnsLingeringChild(t *testing.T) {
	if os.Getenv("PROC_TEST_HELPER") != "spawn" {
		t.Skip("helper: only runs under the re-exec in TestTreeKillReapsGrandchildHoldingPipe")
	}
	self := os.Getenv("PROC_TEST_SELF")
	child := exec.Command(self, "-test.run=TestProcHelperSleeps", "-test.v=false")
	child.Env = append(os.Environ(), "PROC_TEST_HELPER=sleep")
	child.Stdout = os.Stdout // inherit the pipe — this is what wedges the reader
	Hide(child)
	if err := child.Start(); err != nil {
		t.Fatalf("helper: start grandchild: %v", err)
	}
	time.Sleep(60 * time.Second)
}

// TestProcHelperSleeps is the grandchild body — see the helper above.
func TestProcHelperSleeps(t *testing.T) {
	if os.Getenv("PROC_TEST_HELPER") != "sleep" {
		t.Skip("helper: only runs under the re-exec in TestTreeKillReapsGrandchildHoldingPipe")
	}
	time.Sleep(60 * time.Second)
}
