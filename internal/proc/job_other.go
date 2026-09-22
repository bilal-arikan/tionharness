//go:build !windows

package proc

import "os/exec"

// Job outside Windows is the command's own process group: TreeKill already puts
// the command in a fresh group (Setpgid), and Close kills whatever of that group
// is still alive. Unlike a Windows job this does not hold a process that calls
// setsid, and JobLimits are not enforced (there is no per-tree process cap short
// of cgroups).
type Job struct {
	cmd *exec.Cmd
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
