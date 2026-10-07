package proc

// Pure cgroup v2 helpers for the Linux Job (job_linux.go). They take file
// contents and paths instead of touching the filesystem, so they build and are
// unit-tested on every OS; only job_linux.go calls them in production.

import (
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
)

// jobLeafPrefix names the per-job cgroups: tionharness-job-<pid>-<seq>. The pid
// lets a later run remove empty leaves left behind by a crashed process without
// touching another live process's leaves.
const jobLeafPrefix = "tionharness-job-"

// linuxTasksPerProcess converts JobLimits.ActiveProcesses into a pids.max value.
// The cgroup pids controller counts TASKS — every thread is one — while the
// Windows job limit counts processes. A literal pids.max of 128 would starve
// ordinary multi-threaded tools (the Go toolchain runs one compiler per CPU with
// several threads each, a JVM starts dozens), and the Go runtime aborts outright
// when it cannot create a thread. The scaled cap still stops a fork bomb.
const linuxTasksPerProcess = 16

// cgroupV2Rel returns this process's path in the unified (v2) hierarchy from the
// contents of /proc/self/cgroup — the "0::<path>" line. Paths that cannot be
// resolved to a directory (a deleted cgroup, or one outside our cgroup
// namespace, shown with "/..") are errors.
func cgroupV2Rel(procSelfCgroup string) (string, error) {
	for _, line := range strings.Split(procSelfCgroup, "\n") {
		p, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "0::")
		if !ok {
			continue
		}
		if strings.HasSuffix(p, " (deleted)") {
			return "", fmt.Errorf("own cgroup %q was deleted", strings.TrimSuffix(p, " (deleted)"))
		}
		if !strings.HasPrefix(p, "/") {
			return "", fmt.Errorf("unexpected cgroup v2 path %q", p)
		}
		for _, seg := range strings.Split(p, "/") {
			if seg == ".." {
				return "", fmt.Errorf("own cgroup %q lies outside this cgroup namespace", p)
			}
		}
		return path.Clean(p), nil
	}
	return "", errors.New("no cgroup v2 (unified hierarchy) entry in /proc/self/cgroup")
}

// cgroup2Mount picks the cgroup2 mount from /proc/self/mountinfo contents that
// can reach rel (its mount root is rel or an ancestor of it), preferring the
// conventional /sys/fs/cgroup. It returns the mount point and the mount's root
// within the hierarchy.
func cgroup2Mount(mountinfo, rel string) (mountpoint, root string, ok bool) {
	for _, line := range strings.Split(mountinfo, "\n") {
		pre, post, found := strings.Cut(line, " - ")
		if !found {
			continue
		}
		if f := strings.Fields(post); len(f) == 0 || f[0] != "cgroup2" {
			continue
		}
		f := strings.Fields(pre)
		if len(f) < 5 {
			continue
		}
		r, mp := unescapeMountinfo(f[3]), unescapeMountinfo(f[4])
		if !pathWithin(rel, r) {
			continue
		}
		if !ok || mp == "/sys/fs/cgroup" {
			mountpoint, root, ok = mp, r, true
		}
	}
	return mountpoint, root, ok
}

// unescapeMountinfo decodes the octal escapes (\040 for space, ...) mountinfo
// uses in paths.
func unescapeMountinfo(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+4 <= len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// pathWithin reports whether p is base or below it (both slash-rooted, clean).
func pathWithin(p, base string) bool {
	if base == "/" || p == base {
		return true
	}
	return strings.HasPrefix(p, base+"/")
}

// cgroupDirFor maps a hierarchy path to its directory under a cgroup2 mount
// whose root is mountRoot.
func cgroupDirFor(mountpoint, mountRoot, rel string) (string, error) {
	if !pathWithin(rel, mountRoot) {
		return "", fmt.Errorf("cgroup %q is not visible under the cgroup2 mount at %s (root %q)", rel, mountpoint, mountRoot)
	}
	sub := rel
	if mountRoot != "/" {
		sub = strings.TrimPrefix(rel, mountRoot)
	}
	return path.Join(mountpoint, sub), nil
}

// hasController reports whether a cgroup.controllers / cgroup.subtree_control
// list (space separated) names ctrl.
func hasController(list, ctrl string) bool {
	for _, c := range strings.Fields(list) {
		if c == ctrl {
			return true
		}
	}
	return false
}

// pidsMaxValue is the pids.max text for limits: "max" when unlimited, else the
// process cap scaled to tasks (see linuxTasksPerProcess).
func pidsMaxValue(limits JobLimits) string {
	if limits.ActiveProcesses == 0 {
		return "max"
	}
	return strconv.FormatUint(uint64(limits.ActiveProcesses)*linuxTasksPerProcess, 10)
}

// jobLeafName is the cgroup directory name of job number seq of process pid.
func jobLeafName(pid int, seq uint64) string {
	return jobLeafPrefix + strconv.Itoa(pid) + "-" + strconv.FormatUint(seq, 10)
}

// jobLeafOwner returns the pid encoded in a jobLeafName, or false when name is
// not one.
func jobLeafOwner(name string) (int, bool) {
	rest, ok := strings.CutPrefix(name, jobLeafPrefix)
	if !ok {
		return 0, false
	}
	pidStr, seqStr, ok := strings.Cut(rest, "-")
	if !ok {
		return 0, false
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		return 0, false
	}
	if _, err := strconv.ParseUint(seqStr, 10, 64); err != nil {
		return 0, false
	}
	return pid, true
}

// parseCgroupProcs parses cgroup.procs: one pid per line.
func parseCgroupProcs(s string) []int {
	var pids []int
	for _, f := range strings.Fields(s) {
		if pid, err := strconv.Atoi(f); err == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}
	return pids
}

// cgroupEventsPopulated reads the "populated" key of a cgroup.events file; ok is
// false when the key is missing.
func cgroupEventsPopulated(events string) (populated, ok bool) {
	for _, line := range strings.Split(events, "\n") {
		k, v, found := strings.Cut(strings.TrimSpace(line), " ")
		if found && k == "populated" {
			return strings.TrimSpace(v) != "0", true
		}
	}
	return false, false
}
