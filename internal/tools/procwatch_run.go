package tools

import (
	"os/exec"

	"github.com/bilal-arikan/tionharness/internal/procwatch"
)

// runTrackedCmd is cmd.Run() with the pid published to the process ledger while
// the command is still running. Run itself offers no point between fork and
// wait, so the two halves are spelled out; the returned error is exactly what
// Run would have returned (a failed Start included).
//
// It does NOT finish the ledger entry: the caller owns that, because only the
// caller knows whether a non-zero exit is the interesting part of its result.
func runTrackedCmd(cmd *exec.Cmd, h *procwatch.Handle) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	h.Started(cmd)
	return cmd.Wait()
}
