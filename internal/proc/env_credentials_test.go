package proc

import (
	"strings"
	"testing"
)

// The core guarantee: a child environment derived from the real process env never
// carries the API key or the at-rest credential secret, while the variables a
// command actually needs to run survive.
func TestCredentialSafeEnvStripsSecretsKeepsPath(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "yyy")
	t.Setenv("CREDENTIAL_SECRET", "xxx")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIA000")
	t.Setenv("GITHUB_TOKEN", "ghp_000")
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("GOPATH", "/go")

	env := CredentialSafeEnv(nil)

	for _, k := range []string{"ANTHROPIC_API_KEY", "CREDENTIAL_SECRET", "AWS_ACCESS_KEY_ID", "GITHUB_TOKEN"} {
		if v, ok := lastValue(env, k); ok {
			t.Fatalf("%s leaked into child env with value %q", k, v)
		}
	}
	// Nor may a secret VALUE survive under some other name.
	for _, secret := range []string{"yyy", "xxx", "AKIA000", "ghp_000"} {
		for _, kv := range env {
			if strings.Contains(kv, secret) {
				t.Fatalf("secret value %q leaked via %q", secret, kv)
			}
		}
	}
	// Ordinary variables must survive — an over-broad filter breaks commands.
	for k, want := range map[string]string{"PATH": "/usr/bin", "GOPATH": "/go"} {
		if v, ok := lastValue(env, k); !ok || v != want {
			t.Fatalf("%s = %q,%v; want %q (required var must survive)", k, v, ok, want)
		}
	}
	// What was withheld is reported, so a broken command is diagnosable.
	marker, ok := lastValue(env, StrippedEnvVar)
	if !ok {
		t.Fatalf("%s marker missing", StrippedEnvVar)
	}
	for _, k := range []string{"ANTHROPIC_API_KEY", "CREDENTIAL_SECRET", "AWS_ACCESS_KEY_ID", "GITHUB_TOKEN"} {
		if !strings.Contains(marker, k) {
			t.Fatalf("%s = %q; missing %s", StrippedEnvVar, marker, k)
		}
	}
}

// A stale marker from a parent TionHarness process must not be inherited: the
// child's marker has to describe the child's own filtering.
func TestCredentialSafeEnvDropsInheritedMarker(t *testing.T) {
	env := CredentialSafeEnv([]string{StrippedEnvVar + "=BOGUS_FROM_PARENT", "PATH=/usr/bin"})
	v, _ := lastValue(env, StrippedEnvVar)
	if strings.Contains(v, "BOGUS_FROM_PARENT") {
		t.Fatalf("inherited marker survived: %q", v)
	}
}

func TestIsCredentialEnvName(t *testing.T) {
	for _, name := range []string{
		"ANTHROPIC_API_KEY", "anthropic_api_key", "CREDENTIAL_SECRET",
		"OPENAI_ORGANIZATION", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
		"GH_TOKEN", "MY_APP_PASSWORD", "SSH_PRIVATE_KEY", "GPG_PASSPHRASE",
	} {
		if !IsCredentialEnvName(name) {
			t.Errorf("%s classified as safe; want credential", name)
		}
	}
	for _, name := range []string{
		"PATH", "Path", "HOME", "TEMP", "GOPATH", "JAVA_HOME", "LANG",
		"HTTPS_PROXY", "SHELL", "USERPROFILE", "PROGRAMFILES",
	} {
		if IsCredentialEnvName(name) {
			t.Errorf("%s classified as credential; want safe", name)
		}
	}
}
