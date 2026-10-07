//go:build linux

package proc

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Job on Linux is a cgroup v2 leaf holding one command's whole process tree —
// the counterpart of the Windows Job Object. Membership is inherited by every
// descendant and cannot be shed by setsid, double forks or reparenting, so
// Close (cgroup.kill) reaches processes a process-group kill misses. The leaf's
// pids.max caps the tree's size (JobLimits.ActiveProcesses, scaled to tasks —
// see linuxTasksPerProcess).
//
// Placement. Leaves are created directly under this process's own cgroup:
// tionharness-job-<pid>-<seq>. That works wherever the own cgroup is writable —
// a systemd user-session scope (user@UID.service/app.slice/…, delegated to the
// user), a Delegate=yes service, root, or a container with a private cgroup
// namespace and a writable cgroupfs. A leaf needs no controllers to hold and
// kill processes, so the cgroup v2 "no internal processes" rule does not stand
// in the way (it only forbids DOMAIN controllers in subtree_control of a cgroup
// that has processes). For the process cap, pids — a threaded controller, which
// the kernel lets a cgroup with processes enable for its children — is enabled
// once in the own cgroup's cgroup.subtree_control. Nothing is ever migrated out
// of the own cgroup, so other processes sharing it are not disturbed.
//
// Degradation. When cgroup v2 is missing or the own cgroup is not writable, Job
// falls back to the process-group job of other Unix systems (setsid escapes, no
// cap); when only pids is unavailable the tree is still contained but uncapped.
// ContainmentStatus reports which, so the caller can log it once.
//
// Like the Windows job this is a PROCESS and RESOURCE boundary, not a
// filesystem one.
type Job struct {
	dir       string // leaf cgroup directory; "" = process-group job only
	cmd       *exec.Cmd
	closeOnce sync.Once
	closeErr  error
}

// cgroupSetup is the result of probing this process's cgroup once.
type cgroupSetup struct {
	parent    string // directory job leaves are created in; "" = no cgroup containment
	limits    bool   // leaves have pids.max
	cloneInto bool   // CLONE_INTO_CGROUP works (clone3, kernel 5.7+, not seccomp-blocked)
	kill      bool   // cgroup.kill exists (kernel 5.14+)
	freeze    bool   // cgroup.freeze exists (kernel 5.2+), used when kill does not
	reason    string // what is not enforced; "" = fully enforced
}

var (
	cgroupProbeOnce sync.Once
	cgroupState     cgroupSetup
	jobSeq          atomic.Uint64

	lateMu     sync.Mutex
	lateReason string // a per-job failure after a successful probe
)

const degradedPrefix = "process-group containment only (a child that calls setsid escapes Close; no process cap): "

// ContainmentStatus reports whether Jobs created by NewJob fully enforce
// containment — the whole tree killed on Close, setsid included, and the
// process cap applied — and, when not, why. Probing happens on first use.
func ContainmentStatus() (enforced bool, reason string) {
	s := probeCgroup()
	lateMu.Lock()
	late := lateReason
	lateMu.Unlock()
	if late != "" {
		return false, late
	}
	return s.reason == "", s.reason
}

// noteLateDegradation records a per-job shortfall the probe did not predict, so
// ContainmentStatus stops claiming full enforcement.
func noteLateDegradation(format string, a ...any) {
	lateMu.Lock()
	lateReason = fmt.Sprintf(format, a...)
	lateMu.Unlock()
}

func probeCgroup() *cgroupSetup {
	cgroupProbeOnce.Do(func() { cgroupState = detectCgroup() })
	return &cgroupState
}

// detectCgroup finds the own cgroup, checks it can hold job leaves, enables pids
// for them, and test-drives one leaf end to end (create, start a child in it,
// remove it) so a Job never discovers a missing capability mid-command.
func detectCgroup() cgroupSetup {
	degraded := func(format string, a ...any) cgroupSetup {
		return cgroupSetup{reason: degradedPrefix + fmt.Sprintf(format, a...)}
	}
	raw, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return degraded("read /proc/self/cgroup: %v", err)
	}
	rel, err := cgroupV2Rel(string(raw))
	if err != nil {
		return degraded("%v", err)
	}
	mount, root := "/sys/fs/cgroup", "/"
	if mi, err := os.ReadFile("/proc/self/mountinfo"); err == nil {
		if mp, r, ok := cgroup2Mount(string(mi), rel); ok {
			mount, root = mp, r
		}
	}
	dir, err := cgroupDirFor(mount, root, rel)
	if err != nil {
		return degraded("%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cgroup.procs")); err != nil {
		return degraded("cgroup v2 hierarchy not found at %s: %v", dir, err)
	}
	for _, p := range []string{dir, filepath.Join(dir, "cgroup.procs")} {
		if err := unix.Access(p, unix.W_OK); err != nil {
			return degraded("own cgroup %s is not writable (not delegated to this user): %v", dir, err)
		}
	}
	removeStaleLeaves(dir)

	var notes []string
	pidsNote := enablePidsController(dir)

	leaf, err := makeLeaf(dir)
	if err != nil {
		return degraded("create a job cgroup under %s: %v", dir, err)
	}
	s := cgroupSetup{parent: dir}
	s.kill = fileExists(filepath.Join(leaf, "cgroup.kill"))
	s.freeze = fileExists(filepath.Join(leaf, "cgroup.freeze"))
	switch {
	case pidsNote != "":
		notes = append(notes, pidsNote)
	case !fileExists(filepath.Join(leaf, "pids.max")):
		notes = append(notes, "process cap not enforced: pids.max missing in "+leaf)
	default:
		s.limits = true
	}
	s.cloneInto = probeCloneInto(leaf)
	if !s.cloneInto {
		if err := probeMigrate(leaf); err != nil {
			_ = removeLeaf(leaf, s)
			return degraded("cannot move a process into a job cgroup under %s: %v", dir, err)
		}
		notes = append(notes, "CLONE_INTO_CGROUP unavailable: a command joins its cgroup just after it starts, so a child it forks in that instant is not contained")
	}
	if err := removeLeaf(leaf, s); err != nil {
		return degraded("remove probe cgroup %s: %v", leaf, err)
	}
	if len(notes) > 0 {
		s.reason = "cgroup v2 containment in " + dir + ", partially degraded: " + strings.Join(notes, "; ")
	}
	return s
}

// enablePidsController makes pids.max appear in dir's children. It returns ""
// on success, else why the process cap will not be enforced.
func enablePidsController(dir string) string {
	sub, err := os.ReadFile(filepath.Join(dir, "cgroup.subtree_control"))
	if err != nil {
		return fmt.Sprintf("process cap not enforced: read %s/cgroup.subtree_control: %v", dir, err)
	}
	if hasController(string(sub), "pids") {
		return ""
	}
	avail, err := os.ReadFile(filepath.Join(dir, "cgroup.controllers"))
	if err != nil || !hasController(string(avail), "pids") {
		return "process cap not enforced: the pids controller is not delegated to " + dir
	}
	if err := os.WriteFile(filepath.Join(dir, "cgroup.subtree_control"), []byte("+pids"), 0); err != nil {
		return fmt.Sprintf("process cap not enforced: enable pids in %s/cgroup.subtree_control: %v", dir, err)
	}
	return ""
}

// probeCloneInto starts `sh -c 'exit 0'` straight inside leaf with
// CLONE_INTO_CGROUP and reports whether that worked. Old kernels lack clone3 and
// container seccomp profiles commonly answer it with ENOSYS.
func probeCloneInto(leaf string) bool {
	fd, err := unix.Open(leaf, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return false
	}
	defer unix.Close(fd)
	pid, err := syscall.ForkExec("/bin/sh", []string{"sh", "-c", "exit 0"}, &syscall.ProcAttr{
		Env: []string{},
		Sys: &syscall.SysProcAttr{UseCgroupFD: true, CgroupFD: fd},
	})
	if err != nil {
		return false
	}
	waitPid(pid)
	return true
}

// probeMigrate starts a shell blocked on a pipe, moves it into leaf through
// cgroup.procs (the fallback Start uses) and lets it exit.
func probeMigrate(leaf string) error {
	var p [2]int
	if err := unix.Pipe2(p[:], unix.O_CLOEXEC); err != nil {
		return err
	}
	pid, err := syscall.ForkExec("/bin/sh", []string{"sh", "-c", "read _"}, &syscall.ProcAttr{
		Env:   []string{},
		Files: []uintptr{uintptr(p[0])},
	})
	unix.Close(p[0])
	if err != nil {
		unix.Close(p[1])
		return fmt.Errorf("start probe process: %w", err)
	}
	err = os.WriteFile(filepath.Join(leaf, "cgroup.procs"), []byte(strconv.Itoa(pid)), 0)
	unix.Close(p[1]) // EOF: the probe shell exits
	waitPid(pid)
	return err
}

func waitPid(pid int) {
	var ws syscall.WaitStatus
	for {
		if _, err := syscall.Wait4(pid, &ws, 0, nil); err != syscall.EINTR {
			return
		}
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// removeStaleLeaves removes empty job leaves whose owning process is gone (a
// crashed run). Populated ones are left alone: rmdir fails on them.
func removeStaleLeaves(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	self := os.Getpid()
	for _, e := range entries {
		pid, ok := jobLeafOwner(e.Name())
		if !ok || !e.IsDir() || pid == self {
			continue
		}
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// makeLeaf creates a uniquely named job cgroup under parent.
func makeLeaf(parent string) (string, error) {
	leaf := filepath.Join(parent, jobLeafName(os.Getpid(), jobSeq.Add(1)))
	if err := os.Mkdir(leaf, 0o755); err != nil {
		return "", err
	}
	return leaf, nil
}

// killCgroup SIGKILLs every process in leaf. cgroup.kill does it atomically
// (forks racing the kill land in the cgroup and die too). Without it the cgroup
// is frozen first when possible so nothing forks while its pids are being read,
// then cgroup.procs is drained in a bounded loop.
func killCgroup(leaf string, s *cgroupSetup) {
	if s.kill {
		if err := os.WriteFile(filepath.Join(leaf, "cgroup.kill"), []byte("1"), 0); err == nil {
			return
		}
	}
	if s.freeze {
		_ = os.WriteFile(filepath.Join(leaf, "cgroup.freeze"), []byte("1"), 0) // SIGKILL still ends frozen tasks
	}
	for range 100 {
		raw, err := os.ReadFile(filepath.Join(leaf, "cgroup.procs"))
		if err != nil {
			return
		}
		pids := parseCgroupProcs(string(raw))
		if len(pids) == 0 {
			return
		}
		for _, pid := range pids {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// leafRemoveTimeout bounds how long Close waits for killed processes to leave
// the cgroup before giving up on removing it.
const leafRemoveTimeout = 3 * time.Second

// removeLeaf removes an emptied leaf, killing again and retrying while it is
// still populated (EBUSY): killed tasks take a moment to finish exiting.
func removeLeaf(leaf string, s cgroupSetup) error {
	deadline := time.Now().Add(leafRemoveTimeout)
	for {
		err := os.Remove(leaf)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if !errors.Is(err, syscall.EBUSY) || time.Now().After(deadline) {
			return fmt.Errorf("remove job cgroup %s: %w", leaf, err)
		}
		if ev, rerr := os.ReadFile(filepath.Join(leaf, "cgroup.events")); rerr == nil {
			if populated, ok := cgroupEventsPopulated(string(ev)); ok && populated {
				killCgroup(leaf, &s)
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// NewJob creates a job for one command. With cgroup v2 containment available it
// creates the command's leaf cgroup and sets its process cap; otherwise — or if
// creating the leaf fails — it returns a process-group job; a cap that cannot
// be set leaves the tree contained but uncapped (ContainmentStatus says which).
// It does not fail; the error keeps the signature identical to the
// Windows implementation. The caller must Close it.
func NewJob(limits JobLimits) (*Job, error) {
	s := probeCgroup()
	if s.parent == "" {
		return &Job{}, nil
	}
	leaf, err := makeLeaf(s.parent)
	if err != nil {
		noteLateDegradation(degradedPrefix+"create a job cgroup under %s: %v", s.parent, err)
		return &Job{}, nil
	}
	if s.limits {
		// The tree is still contained without its cap, so a failure here (e.g. the
		// service manager disabled pids in the parent since the probe) keeps the leaf.
		if err := os.WriteFile(filepath.Join(leaf, "pids.max"), []byte(pidsMaxValue(limits)), 0); err != nil {
			noteLateDegradation("cgroup v2 containment in %s, but process cap not enforced: set pids.max: %v", s.parent, err)
		}
	}
	return &Job{dir: leaf}, nil
}

// Start starts cmd inside the job: in its own process group (TreeKill) and, with
// a leaf cgroup, inside it. With CLONE_INTO_CGROUP the child is created in the
// leaf, so not one instruction of it runs outside; otherwise it is moved there
// right after the fork through cgroup.procs, and a grandchild forked in that
// window stays outside (still in the process group). Context cancellation kills
// the cgroup as well as the group. If the command cannot be placed in the leaf
// it is killed and an error returned: a command meant to be contained must not
// run uncontained.
func (j *Job) Start(cmd *exec.Cmd) error {
	TreeKill(cmd)
	if j.dir == "" {
		if err := cmd.Start(); err != nil {
			return err
		}
		j.cmd = cmd
		return nil
	}
	s := probeCgroup()
	leaf := j.dir
	if groupCancel := cmd.Cancel; groupCancel != nil {
		cmd.Cancel = func() error {
			killCgroup(leaf, s)
			return groupCancel()
		}
	}
	if s.cloneInto {
		fd, err := unix.Open(leaf, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		if err != nil {
			return fmt.Errorf("open job cgroup: %w", err)
		}
		cmd.SysProcAttr.UseCgroupFD = true
		cmd.SysProcAttr.CgroupFD = fd
		err = cmd.Start()
		unix.Close(fd)
		if err != nil {
			return err
		}
		j.cmd = cmd
		return nil
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(leaf, "cgroup.procs"), []byte(strconv.Itoa(cmd.Process.Pid)), 0); err != nil {
		KillTree(cmd)
		_ = cmd.Wait()
		return fmt.Errorf("move process into job cgroup: %w", err)
	}
	j.cmd = cmd
	return nil
}

// Close kills every process still in the job — the whole cgroup, then the
// process group for good measure — and removes the leaf cgroup. Safe to call on
// a job whose processes have all exited, and more than once.
func (j *Job) Close() error {
	if j == nil {
		return nil
	}
	j.closeOnce.Do(func() {
		if j.dir != "" {
			s := probeCgroup()
			killCgroup(j.dir, s)
			j.closeErr = removeLeaf(j.dir, *s)
		}
		if j.cmd != nil {
			KillTree(j.cmd)
			j.cmd = nil
		}
	})
	return j.closeErr
}
