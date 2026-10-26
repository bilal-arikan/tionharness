package providers

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"testing"
	"time"
)

// TestPersistentSessionUsesTurnEnv: the warm process must authenticate exactly
// like a one-shot turn. It used to launch without CLAUDE_CONFIG_DIR or the
// configured token, so it ran on the ambient ~/.claude login and failed every
// turn once that login expired (WS3, 2026-10-25).
func TestPersistentSessionUsesTurnEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake CLI")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "claude-home")
	c := NewClaudeCLI(bin, "", home, "oauth", "tok-123")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := Request{Model: "claude-sonnet-5-5", CLIEffortLevel: "max", Messages: []Message{{Role: RoleUser, Text: "hi"}}}
	sess, err := c.startPersistent(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	for _, want := range []string{
		"CLAUDE_CONFIG_DIR=" + home,
		"CLAUDE_CODE_OAUTH_TOKEN=tok-123",
		"CLAUDE_CODE_EFFORT_LEVEL=max",
		"MCP_TOOL_TIMEOUT=" + strconv.Itoa(InteractionToolTimeoutSec*1000),
	} {
		if !slices.Contains(sess.cmd.Env, want) {
			t.Errorf("persistent env lacks %q", want)
		}
	}
	if !slices.Equal(sess.cmd.Env, c.turnEnv(req, req.Model)) {
		t.Error("persistent env must equal the one-shot turn env")
	}

	// A credential change must restart the process, not reuse it.
	other := NewClaudeCLI(bin, "", home, "oauth", "tok-456")
	if c.persistentFingerprint(req, "sys") == other.persistentFingerprint(req, "sys") {
		t.Error("fingerprint must change with the credential")
	}
}
