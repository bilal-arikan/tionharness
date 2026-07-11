package proc

import (
	"os/exec"
	"strings"
	"testing"
)

// lastValue returns the last value for key in an env slice, mirroring how
// os/exec resolves duplicates (last wins).
func lastValue(env []string, key string) (string, bool) {
	prefix := key + "="
	val, ok := "", false
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			val, ok = kv[len(prefix):], true
		}
	}
	return val, ok
}

func TestHardenedEnvAppendsGuards(t *testing.T) {
	got := HardenedEnv([]string{"FOO=bar"})
	for k, want := range map[string]string{
		"GIT_TERMINAL_PROMPT": "0",
		"GIT_EDITOR":          "true",
		"GIT_PAGER":           "cat",
		"GIT_OPTIONAL_LOCKS":  "0",
	} {
		if v, ok := lastValue(got, k); !ok || v != want {
			t.Fatalf("%s = %q,%v; want %q", k, v, ok, want)
		}
	}
	if v, _ := lastValue(got, "FOO"); v != "bar" {
		t.Fatalf("base var FOO lost: %q", v)
	}
}

// The guard must WIN over an inherited interactive editor (this is the notepad
// hang fix): the guard is appended last so os/exec's last-wins dedup picks it.
func TestHardenedEnvGuardOverridesInherited(t *testing.T) {
	got := HardenedEnv([]string{"GIT_EDITOR=notepad"})
	if v, _ := lastValue(got, "GIT_EDITOR"); v != "true" {
		t.Fatalf("GIT_EDITOR last value = %q; want \"true\" (guard must override notepad)", v)
	}
}

func TestTreeKillConfiguresCancel(t *testing.T) {
	cmd := exec.Command("someprog")
	TreeKill(cmd)
	if cmd.Cancel == nil {
		t.Fatal("TreeKill did not set cmd.Cancel")
	}
	if cmd.WaitDelay <= 0 {
		t.Fatal("TreeKill did not set a positive cmd.WaitDelay")
	}
}
