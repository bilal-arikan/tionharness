package providers

import (
	"errors"
	"strings"
	"testing"
)

func TestCodexAuthAndModelFailuresArePermanent(t *testing.T) {
	for _, class := range []codexFailureClass{codexFailureAuth, codexFailureModel} {
		if err := newCodexFailureError(class, "rejected", "home"); !errors.Is(err, ErrPermanentProviderFailure) {
			t.Fatalf("class %v error = %v, want permanent marker", class, err)
		}
	}
	if err := newCodexFailureError(codexFailureQuota, "limited", "home"); errors.Is(err, ErrPermanentProviderFailure) {
		t.Fatalf("quota error unexpectedly permanent: %v", err)
	}
}

func TestClassifyCodexError(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want codexFailureClass
	}{
		{
			// The exact 401 body captured live. Codex retries this internally 5×
			// over websocket then 5× over https (~35 s), so classifying it from the
			// FIRST error event is what makes the early kill possible.
			name: "live 401 body",
			msg:  "unexpected status 401 Unauthorized: Missing bearer or basic authentication in header, please check your credentials",
			want: codexFailureAuth,
		},
		{name: "not logged in", msg: "Not logged in. Please run `codex login`.", want: codexFailureAuth},
		{
			// The exact body of a revoked OAuth credential, captured live. It carries
			// no 401/bearer wording, so before the refresh-token signatures it fell
			// through to codexFailureNone and the user never got the message naming
			// which CODEX_HOME needs `codex login`.
			name: "revoked refresh token",
			msg:  "Your access token could not be refreshed because your refresh token was revoked. Please log out and sign in again.",
			want: codexFailureAuth,
		},
		{name: "at capacity", msg: "Selected model is at capacity. Please try a different model.", want: codexFailureQuota},
		{name: "rate limit", msg: "You have hit your rate limit for this hour", want: codexFailureQuota},
		{name: "usage limit", msg: "weekly usage limit reached", want: codexFailureQuota},
		{
			// Must NOT be reported as a quota problem: the user has to change the
			// model, not wait for a window to reset.
			name: "chatgpt account model rejection",
			msg:  "The 'gpt-5.4' model is not supported when using Codex with a ChatGPT account.",
			want: codexFailureModel,
		},
		{name: "unknown", msg: "stream disconnected before completion", want: codexFailureNone},
		{name: "empty", msg: "", want: codexFailureNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyCodexError(tc.msg); got != tc.want {
				t.Fatalf("classifyCodexError(%q) = %v, want %v", tc.msg, got, tc.want)
			}
		})
	}
}

func TestCodexFailureClassRetryable(t *testing.T) {
	// Every classified failure is terminal — that is the point of classifying.
	for _, c := range []codexFailureClass{codexFailureAuth, codexFailureQuota, codexFailureModel} {
		if c.retryable() {
			t.Fatalf("class %v must not be retryable", c)
		}
	}
	if !codexFailureNone.retryable() {
		t.Fatal("codexFailureNone must leave the retry decision to the caller")
	}
}

func TestDescribeCodexFailureNamesConfigDir(t *testing.T) {
	got := describeCodexFailure(codexFailureAuth, "401 Unauthorized", `C:\ws\codex-home`)
	if !strings.Contains(got, "codex login") {
		t.Fatalf("auth message must tell the user to run codex login: %q", got)
	}
	if !strings.Contains(got, `CODEX_HOME=C:\ws\codex-home`) {
		t.Fatalf("auth message must name the config dir to log into: %q", got)
	}
	if got := describeCodexFailure(codexFailureAuth, "401", ""); !strings.Contains(got, "~/.codex") {
		t.Fatalf("empty config dir must fall back to the ambient home: %q", got)
	}
}

func TestCodexSandboxArgs(t *testing.T) {
	cases := map[string][]string{
		"read-only": {"-s", "read-only"},
		// "ask" is workspace-write, NOT per-tool approval: `codex exec` rejects
		// every approval request.
		"ask":      {"-s", "workspace-write"},
		"auto":     {"--dangerously-bypass-approvals-and-sandbox"},
		"":         {"--dangerously-bypass-approvals-and-sandbox"},
		"nonsense": {"--dangerously-bypass-approvals-and-sandbox"},
	}
	for mode, want := range cases {
		got := codexSandboxArgs(mode)
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Fatalf("codexSandboxArgs(%q) = %v, want %v", mode, got, want)
		}
	}
}

// TestCodexBuildArgsFlagOrder guards the single most breakage-prone fact about
// the codex command line: every option belongs to the `exec` subcommand and
// MUST precede `resume`. `codex exec resume <id> -s read-only` fails outright
// with "unexpected argument '-s' found".
func TestCodexBuildArgsFlagOrder(t *testing.T) {
	c := NewCodexCLI("codex", "", "")
	args := c.buildArgs(Request{PermissionMode: "read-only", CLIResumeScope: "SES1/AGT1", ResumeSessionID: "01a01457-0c11-71d0-b495-615d96b8b913"}, "gpt-5.4-mini")

	idx := func(want string) int {
		for i, a := range args {
			if a == want {
				return i
			}
		}
		return -1
	}
	resumeAt := idx("resume")
	if resumeAt < 0 {
		t.Fatalf("resume missing from %v", args)
	}
	for _, flag := range []string{"--json", "--strict-config", "-s", "-m"} {
		at := idx(flag)
		if at < 0 {
			t.Fatalf("%s missing from %v", flag, args)
		}
		if at > resumeAt {
			t.Fatalf("%s at %d must precede resume at %d: %v", flag, at, resumeAt, args)
		}
	}
	if args[0] != "exec" {
		t.Fatalf("first arg must be exec, got %v", args)
	}
	if args[resumeAt+1] != "01a01457-0c11-71d0-b495-615d96b8b913" {
		t.Fatalf("thread id must follow resume: %v", args)
	}
}

// TestCodexBuildArgsNeverIgnoresUserConfig guards against reintroducing
// --ignore-user-config: that flag skips the "user" config layer, which is
// CODEX_HOME/config.toml itself — the exact file writeCodexConfig renders MCP
// servers and developer_instructions into. With the flag set, codex never sees
// our MCP servers and every tool call fails with "not in tool registry" (live
// A/B verified). Isolation from the invoking user's ambient ~/.codex is already
// achieved via CODEX_HOME, so this flag is both redundant and destructive.
func TestCodexBuildArgsNeverIgnoresUserConfig(t *testing.T) {
	c := NewCodexCLI("codex", "", "")
	args := c.buildArgs(Request{PermissionMode: "auto"}, "")
	for _, a := range args {
		if a == "--ignore-user-config" {
			t.Fatalf("--ignore-user-config must not be emitted: it skips CODEX_HOME/config.toml, killing the MCP bridge: %v", args)
		}
	}
}

func TestCodexBuildArgsNoResumeWhenFresh(t *testing.T) {
	c := NewCodexCLI("codex", "", "")
	args := c.buildArgs(Request{PermissionMode: "auto"}, "")
	for _, a := range args {
		if a == "resume" {
			t.Fatalf("fresh turn must not resume: %v", args)
		}
		if a == "-m" {
			t.Fatalf("empty model must not emit -m: %v", args)
		}
	}
}

func TestCodexBuildArgsRejectsUnscopedResume(t *testing.T) {
	c := NewCodexCLI("codex", "", "")
	args := c.buildArgs(Request{ResumeSessionID: "thread-without-safe-home"}, "")
	for _, arg := range args {
		if arg == "resume" {
			t.Fatalf("unscoped resume escaped home safety gate: %v", args)
		}
	}
}

func TestCodexReasoningEffort(t *testing.T) {
	// DisableThinking is the explicit off switch and outranks the effort level.
	if got := codexReasoningEffort(Request{DisableThinking: true, CLIEffortLevel: "high"}); got != "none" {
		t.Fatalf("DisableThinking must map to none, got %q", got)
	}
	if got := codexReasoningEffort(Request{CLIEffortLevel: "MAX"}); got != "max" {
		t.Fatalf("max effort must pass through, got %q", got)
	}
	// An effort level codex does not accept must yield "" (use the CLI default)
	// rather than being passed on — --strict-config would reject it.
	if got := codexReasoningEffort(Request{CLIEffortLevel: "ultra-turbo"}); got != "" {
		t.Fatalf("unknown effort must fall back to the CLI default, got %q", got)
	}
}

// TestCodexCLIImplementsCLIProvider is the interface contract W5 wires against.
func TestCodexCLIImplementsCLIProvider(t *testing.T) {
	var _ CLIProvider = (*CodexCLI)(nil)
}

func TestCodexCLISetConfigDirIgnoresEmpty(t *testing.T) {
	c := NewCodexCLI("codex", "", `C:\ws\codex-home`)
	c.SetConfigDir("")
	if c.configDir != `C:\ws\codex-home` {
		t.Fatalf("empty dir must not clear the constructed default, got %q", c.configDir)
	}
	c.SetConfigDir(`C:\other`)
	if c.configDir != `C:\other` {
		t.Fatalf("non-empty dir must override, got %q", c.configDir)
	}
}
