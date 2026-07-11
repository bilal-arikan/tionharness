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

// Confined turns must disable commit/tag signing so an unattended git op can't
// hang on a GPG pinentry prompt.
func TestDisableGitSigningEnv(t *testing.T) {
	env := DisableGitSigningEnv()
	if v, _ := lastValue(env, "GIT_CONFIG_COUNT"); v != "2" {
		t.Fatalf("GIT_CONFIG_COUNT = %q; want 2", v)
	}
	// Both keys must map to false; order-independent scan of key→value pairs.
	seen := map[string]string{}
	for i := 0; i < 2; i++ {
		k, _ := lastValue(env, "GIT_CONFIG_KEY_"+string(rune('0'+i)))
		val, _ := lastValue(env, "GIT_CONFIG_VALUE_"+string(rune('0'+i)))
		seen[k] = val
	}
	for _, k := range []string{"commit.gpgsign", "tag.gpgsign"} {
		if seen[k] != "false" {
			t.Fatalf("%s = %q; want false (got pairs %v)", k, seen[k], seen)
		}
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
