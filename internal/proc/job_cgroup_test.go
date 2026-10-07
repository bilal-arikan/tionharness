package proc

import (
	"slices"
	"testing"
)

func TestCgroupV2Rel(t *testing.T) {
	cases := []struct {
		name, in, want string
		wantErr        bool
	}{
		{"unified only", "0::/user.slice/user-1000.slice/user@1000.service/app.slice/app-x.scope\n",
			"/user.slice/user-1000.slice/user@1000.service/app.slice/app-x.scope", false},
		{"hybrid", "12:pids:/user.slice\n1:name=systemd:/user.slice/s.scope\n0::/user.slice/s.scope\n", "/user.slice/s.scope", false},
		{"namespace root", "0::/\n", "/", false},
		{"crlf", "0::/a/b\r\n", "/a/b", false},
		{"v1 only", "12:pids:/user.slice\n1:name=systemd:/x\n", "", true},
		{"outside namespace", "0::/../../system.slice/x.service\n", "", true},
		{"deleted", "0::/a/b (deleted)\n", "", true},
		{"relative", "0::a/b\n", "", true},
	}
	for _, c := range cases {
		got, err := cgroupV2Rel(c.in)
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("%s: cgroupV2Rel = %q, %v; want %q, err=%v", c.name, got, err, c.want, c.wantErr)
		}
	}
}

func TestCgroup2Mount(t *testing.T) {
	const mi = `22 28 0:21 / /proc rw,nosuid,nodev,noexec,relatime shared:12 - proc proc rw
30 25 0:26 / /sys/fs/cgroup rw,nosuid,nodev,noexec,relatime shared:4 - cgroup2 cgroup2 rw,nsdelegate
31 25 0:27 / /sys/fs/cgroup/pids rw,nosuid shared:5 - cgroup cgroup rw,pids
40 25 0:26 /user.slice /mnt/my\040cg rw shared:4 - cgroup2 cgroup2 rw
`
	mp, root, ok := cgroup2Mount(mi, "/user.slice/x.scope")
	if !ok || mp != "/sys/fs/cgroup" || root != "/" {
		t.Fatalf("prefer /sys/fs/cgroup: got %q %q %v", mp, root, ok)
	}
	// Only the bind mount below /user.slice is present.
	const bind = `40 25 0:26 /user.slice /mnt/my\040cg rw shared:4 - cgroup2 cgroup2 rw
`
	mp, root, ok = cgroup2Mount(bind, "/user.slice/x.scope")
	if !ok || mp != "/mnt/my cg" || root != "/user.slice" {
		t.Fatalf("bind mount: got %q %q %v", mp, root, ok)
	}
	if _, _, ok := cgroup2Mount(bind, "/system.slice/y.service"); ok {
		t.Fatal("a mount whose root does not contain rel must not be picked")
	}
	if _, _, ok := cgroup2Mount("31 25 0:27 / /sys/fs/cgroup/pids rw - cgroup cgroup rw,pids\n", "/"); ok {
		t.Fatal("cgroup v1 mount picked as cgroup2")
	}
}

func TestCgroupDirFor(t *testing.T) {
	cases := []struct {
		mp, root, rel, want string
		wantErr             bool
	}{
		{"/sys/fs/cgroup", "/", "/a/b", "/sys/fs/cgroup/a/b", false},
		{"/sys/fs/cgroup", "/", "/", "/sys/fs/cgroup", false},
		{"/mnt/cg", "/a", "/a/b", "/mnt/cg/b", false},
		{"/mnt/cg", "/a", "/a", "/mnt/cg", false},
		{"/mnt/cg", "/a", "/ab", "", true},
		{"/mnt/cg", "/a", "/b", "", true},
	}
	for _, c := range cases {
		got, err := cgroupDirFor(c.mp, c.root, c.rel)
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("cgroupDirFor(%q,%q,%q) = %q, %v; want %q, err=%v", c.mp, c.root, c.rel, got, err, c.want, c.wantErr)
		}
	}
}

func TestPidsMaxValue(t *testing.T) {
	if got := pidsMaxValue(JobLimits{}); got != "max" {
		t.Errorf("unlimited = %q, want max", got)
	}
	if got := pidsMaxValue(JobLimits{ActiveProcesses: 128}); got != "2048" {
		t.Errorf("128 processes = %q, want 2048 tasks", got)
	}
	if got := pidsMaxValue(JobLimits{ActiveProcesses: ^uint32(0)}); got != "68719476720" {
		t.Errorf("max uint32 must not overflow: %q", got)
	}
}

func TestJobLeafNameRoundTrip(t *testing.T) {
	name := jobLeafName(4242, 7)
	if name != "tionharness-job-4242-7" {
		t.Fatalf("jobLeafName = %q", name)
	}
	if pid, ok := jobLeafOwner(name); !ok || pid != 4242 {
		t.Fatalf("jobLeafOwner(%q) = %d, %v", name, pid, ok)
	}
	for _, bad := range []string{"tionharness-job-", "tionharness-job-12", "tionharness-job-x-1", "tionharness-job-0-1", "tionharness-job-5-x", "other-5-1"} {
		if _, ok := jobLeafOwner(bad); ok {
			t.Errorf("jobLeafOwner(%q) accepted", bad)
		}
	}
}

func TestCgroupFileParsers(t *testing.T) {
	if got := parseCgroupProcs("12\n345\n\n"); !slices.Equal(got, []int{12, 345}) {
		t.Errorf("parseCgroupProcs = %v", got)
	}
	if got := parseCgroupProcs(""); len(got) != 0 {
		t.Errorf("empty cgroup.procs = %v", got)
	}
	if p, ok := cgroupEventsPopulated("populated 1\nfrozen 0\n"); !p || !ok {
		t.Errorf("populated 1 -> %v %v", p, ok)
	}
	if p, ok := cgroupEventsPopulated("populated 0\nfrozen 0\n"); p || !ok {
		t.Errorf("populated 0 -> %v %v", p, ok)
	}
	if _, ok := cgroupEventsPopulated("frozen 0\n"); ok {
		t.Error("missing key reported ok")
	}
	if !hasController("cpuset cpu io memory pids", "pids") || hasController("cpuset cpu io memory", "pids") || hasController("pidsx", "pids") {
		t.Error("hasController")
	}
}
