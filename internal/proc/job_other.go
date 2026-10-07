//go:build !windows && !linux

package proc

import (
	"os/exec"
	"runtime"
)

// Job on Unix systems other than Linux (macOS, the BSDs) is the command's own
// process group: TreeKill already puts the command in a fresh group (Setpgid),
// and Close kills whatever of that group is still alive. Unlike the Windows job
// object and the Linux cgroup job (job_linux.go) this does not hold a process
// that calls setsid, and JobLimits are not enforced (there is no per-tree
// process cap without cgroups).
type Job struct {
	cmd *exec.Cmd
}

// ContainmentStatus reports that Jobs here are process-group jobs only; see the
// Linux implementation for the full contract.
func ContainmentStatus() (enforced bool, reason string) {
	return false, "process-group containment only on " + runtime.GOOS +
		" (a child that calls setsid escapes Close; no process cap)"
}

// NewJob returns a process-group job. It never fails; the error keeps the
// signature identical to the Windows implementation.
func NewJob(JobLimits) (*Job, error) { return &Job{}, nil }

// Start starts cmd in its own process group so Close can reach the whole tree.
func (j *Job) Start(cmd *exec.Cmd) error {
	TreeKill(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	j.cmd = cmd
	return nil
}

// Close kills every process still in the command's process group.
func (j *Job) Close() error {
	if j == nil || j.cmd == nil {
		return nil
	}
	KillTree(j.cmd)
	j.cmd = nil
	return nil
}
