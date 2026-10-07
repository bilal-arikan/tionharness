package proc

// Job contains one command's whole process tree: Close kills all of it, and
// JobLimits cap its size. Each platform supplies its own implementation:
//
//   - Windows (job_windows.go): a Job Object with kill-on-close.
//   - Linux (job_linux.go): a cgroup v2 leaf under this process's own cgroup,
//     killed with cgroup.kill and capped with pids.max; it degrades to a
//     process-group job when cgroup v2 is unavailable or not delegated.
//   - Other Unix (job_other.go): the command's process group only.
//
// ContainmentStatus reports whether the current platform and host give full
// containment, so a caller can log a degraded setup once.

// JobLimits bounds a contained process tree (see Job). Zero fields mean "no
// limit" for that resource.
type JobLimits struct {
	// ActiveProcesses caps how many processes of the tree may be alive at once.
	// Process creation beyond it fails inside the tree; it is a fork-bomb brake.
	// On Linux the cgroup pids controller counts threads too, so the cap is
	// applied as ActiveProcesses × linuxTasksPerProcess tasks (job_cgroup.go).
	ActiveProcesses uint32
}
