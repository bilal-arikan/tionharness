//go:build windows

package proc

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Job is a Windows Job Object holding one command's whole process tree. Every
// process the command creates — including ones that detach from their parent
// (`start`, `Start-Process`, `nohup … &`) and would escape taskkill /T once the
// parent exits — stays in the job, and closing the job kills all of them
// (JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE). The job also caps how many processes may
// be alive at once, which stops a fork bomb.
//
// This is a PROCESS and RESOURCE boundary, not a filesystem one: processes in
// the job still run as the same user with the same file access.
type Job struct {
	h windows.Handle
}

// ntResumeProcess resumes every thread of a process created CREATE_SUSPENDED.
// exec.Cmd does not expose the primary thread handle, so the per-process ntdll
// call is the only way to resume it.
var ntResumeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

// ContainmentStatus reports that Jobs fully enforce containment: a job object
// holds the whole tree and its process cap on every supported Windows version.
func ContainmentStatus() (enforced bool, reason string) { return true, "" }

// NewJob creates an anonymous job object with the given limits and
// kill-on-close set. The caller must Close it.
func NewJob(limits JobLimits) (*Job, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create job object: %w", err)
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if limits.ActiveProcesses > 0 {
		info.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
		info.BasicLimitInformation.ActiveProcessLimit = limits.ActiveProcesses
	}
	if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("set job object limits: %w", err)
	}
	return &Job{h: h}, nil
}

// Start starts cmd inside the job. The process is created suspended, assigned to
// the job, and only then resumed, so it cannot create a single child outside the
// job in between. On any failure after creation the process is killed: a command
// that was supposed to be contained must not keep running uncontained.
func (j *Job) Start(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	if err := cmd.Start(); err != nil {
		return err
	}
	fail := func(err error) error {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	ph, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME, false, uint32(cmd.Process.Pid))
	if err != nil {
		return fail(fmt.Errorf("open started process: %w", err))
	}
	defer windows.CloseHandle(ph)
	if err := windows.AssignProcessToJobObject(j.h, ph); err != nil {
		return fail(fmt.Errorf("assign process to job object: %w", err))
	}
	if status, _, _ := ntResumeProcess.Call(uintptr(ph)); status != 0 {
		return fail(fmt.Errorf("resume contained process: NTSTATUS 0x%08x", status))
	}
	return nil
}

// Close kills every process still in the job and releases it. Safe to call on a
// job whose processes have all exited.
func (j *Job) Close() error {
	if j == nil || j.h == 0 {
		return nil
	}
	err := windows.CloseHandle(j.h)
	j.h = 0
	return err
}
