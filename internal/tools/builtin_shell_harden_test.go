package tools

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

// Every shell subprocess is built through hardenShellCmd, so this is the single
// place where credential leakage into an agent-run command can be proven absent.
func TestHardenShellCmdStripsCredentials(t *testing.T) {
	t.Setenv("CREDENTIAL_SECRET", "xxx")
	t.Setenv("ANTHROPIC_API_KEY", "yyy")
	t.Setenv("PATH", "/usr/bin")

	cmd := hardenShellCmd(exec.Command("someprog"), false)

	for _, kv := range cmd.Env {
		name := kv
		if eq := strings.IndexByte(kv, '='); eq > 0 {
			name = kv[:eq]
		}
		if proc.IsCredentialEnvName(name) {
			t.Fatalf("credential %s reached the shell subprocess env", name)
		}
	}
	if !hasEnv(cmd.Env, "PATH=/usr/bin") {
		t.Fatal("PATH missing from shell subprocess env")
	}
	// The non-interactive guards must still be there (they are appended after the
	// credential filter, so filtering must not have displaced them).
	if !hasEnv(cmd.Env, "GIT_EDITOR=true") {
		t.Fatal("GIT_EDITOR guard missing after credential filtering")
	}
}

func hasEnv(env []string, want string) bool {
	for _, kv := range env {
		if kv == want {
			return true
		}
	}
	return false
}
