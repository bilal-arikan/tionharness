package providers

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestClaude55CLIModelAndThinking(t *testing.T) {
	t.Setenv("MAX_THINKING_TOKENS", "777")
	for _, model := range []string{"claude-opus-5-5", "claude-sonnet-5-5"} {
		t.Run(model, func(t *testing.T) {
			dir := t.TempDir()
			capture := filepath.Join(dir, "invocation.txt")
			bin := filepath.Join(dir, "claude")
			const result = `{"type":"result","subtype":"success","result":"ok","is_error":false,"session_id":"local-test"}`
			script := "#!/bin/sh\nif [ \"$1\" = '--version' ]; then echo 'Claude Code 2.1.284'; exit 0; fi\nprintf '%s\\n' \"$*\" \"thinking=$MAX_THINKING_TOKENS\" > '" + capture + "'\nprintf '%s\\n' '" + result + "'\n"
			if runtime.GOOS == "windows" {
				bin += ".cmd"
				script = "@echo off\r\nif \"%~1\"==\"--version\" (echo Claude Code 2.1.284 & exit /b 0)\r\n>\"" + capture + "\" echo %*\r\n>>\"" + capture + "\" echo thinking=%MAX_THINKING_TOKENS%\r\necho " + result + "\r\n"
			}
			if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			provider := NewClaudeCLI(bin, "", filepath.Join(dir, "config"), "", "")
			req := Request{Model: model, DisableThinking: true, WorkDir: dir, Messages: []Message{{Role: RoleUser, Text: "hello"}}}
			resp, err := provider.Complete(ctx, req)
			if err != nil || resp.Text != "ok" {
				t.Fatalf("local CLI result: response=%+v err=%v", resp, err)
			}
			data, err := os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			wantThinking := "0"
			if model == "claude-opus-5-5" {
				wantThinking = ""
			}
			lines := "\n" + strings.ReplaceAll(string(data), "\r", "")
			if !strings.Contains(lines, "--model "+model) || !strings.Contains(lines, "\nthinking="+wantThinking+"\n") {
				t.Fatalf("wrong model or thinking configuration: %s", data)
			}
			session, err := provider.startPersistent(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			if session.model != model || slices.Contains(session.cmd.Env, "MAX_THINKING_TOKENS=0") != (model == "claude-sonnet-5-5") {
				t.Fatal("persistent CLI model or thinking configuration differs")
			}
		})
	}
}
