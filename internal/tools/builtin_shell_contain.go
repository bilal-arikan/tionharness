package tools

import (
	"fmt"
	"os/exec"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

// confinedShellJobLimits caps a confined foreground command's process tree. The
// number is a fork-bomb brake, far above what a build or test pipeline keeps
// alive at once (git-bash itself costs two processes per command).
var confinedShellJobLimits = proc.JobLimits{ActiveProcesses: 128}

// runShellCmd runs cmd to completion. A confined command runs inside a process
// job (proc.Job): everything it spawns — including processes that detach from
// their parent — is killed when the call returns, so an autonomous turn cannot
// leave work running after its tool call ends, and its process count is capped.
//
// runErr is the command's own outcome (formatted into the tool output as before);
// setupErr means containment could not be established and the command did not
// run. A confined command never falls back to running uncontained.
//
// started, when non-nil, is called once the process exists and before the wait,
// so the process ledger can publish its pid while the command is still running
// (internal/procwatch). This is why the uncontained path spells Start+Wait out
// instead of calling cmd.Run: Run offers no point between the two.
//
// This is a process/resource boundary only. The command still runs as the same
// user with the same filesystem access; see the note above shellMaxOutputBytes.
func runShellCmd(cmd *exec.Cmd, confined bool, started func(*exec.Cmd)) (runErr, setupErr error) {
	if !confined {
		if err := cmd.Start(); err != nil {
			return err, nil
		}
		if started != nil {
			started(cmd)
		}
		return cmd.Wait(), nil
	}
	job, err := proc.NewJob(confinedShellJobLimits)
	if err != nil {
		return nil, fmt.Errorf("confined shell: cannot create a process job, command not run: %w", err)
	}
	defer job.Close()
	if err := job.Start(cmd); err != nil {
		return nil, fmt.Errorf("confined shell: cannot start the command inside a process job: %w", err)
	}
	if started != nil {
		started(cmd)
	}
	return cmd.Wait(), nil
}
