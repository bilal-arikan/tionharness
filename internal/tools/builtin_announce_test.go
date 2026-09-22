package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/secrets"
)

// newAnnounceTool builds the tool over a temp workspace, optionally seeding
// release.json and vault entries.
func newAnnounceTool(t *testing.T, releaseJSON string, vaultEntries map[string]string) AnnounceReleaseTool {
	t.Helper()
	dir := t.TempDir()
	if releaseJSON != "" {
		if err := os.MkdirAll(filepath.Join(dir, "_Docs"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, releaseFilePath), []byte(releaseJSON), 0o644); err != nil {
			t.Fatalf("seed release.json: %v", err)
		}
	}
	vault, err := secrets.Open(t.TempDir(), stubCipher{})
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}
	for k, v := range vaultEntries {
		if _, err := vault.Set(k, v, "test"); err != nil {
			t.Fatalf("vault set %s: %v", k, err)
		}
	}
	return NewAnnounceReleaseTool(NewSandbox(dir), vault)
}

const sampleReleaseJSON = `{
  "version": "1.2.3",
  "tag": "v1.2.3",
  "date": "2026-09-22",
  "count": 2,
  "sections": [
    {"title":"Features","entries":[{"hash":"b1","scope":"monitor","subject":"watch files"}]},
    {"title":"Bug Fixes","entries":[{"hash":"c1","subject":"stop the leak"}]}
  ]
}`

// call invokes the tool with the given args.
func call(t *testing.T, tool AnnounceReleaseTool, args string) (string, error) {
	t.Helper()
	return tool.Call(context.Background(), json.RawMessage(args))
}

// TestAnnounceDryRunRendersWithoutPosting is the safe path an agent uses to
// check the message before it goes out.
func TestAnnounceDryRunRendersWithoutPosting(t *testing.T) {
	tool := newAnnounceTool(t, sampleReleaseJSON, map[string]string{
		discordSecret: "https://discord.example/webhook",
	})
	out, err := call(t, tool, `{"dry_run":true}`)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	for _, want := range []string{"Dry run", "discord", "v1.2.3", "watch files"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry run output is missing %q:\n%s", want, out)
		}
	}
}

// TestAnnounceRequiresReleaseFile: there is nothing to announce before the
// pipeline has written release.json, and the error must say so.
func TestAnnounceRequiresReleaseFile(t *testing.T) {
	tool := newAnnounceTool(t, "", map[string]string{discordSecret: "https://discord.example/webhook"})
	_, err := call(t, tool, `{"dry_run":true}`)
	if err == nil {
		t.Fatal("a missing release.json was accepted")
	}
	if !strings.Contains(err.Error(), releaseFilePath) {
		t.Fatalf("the error does not name the file: %v", err)
	}
}

// TestAnnounceRejectsAnUnnamedRelease: announcing a release with no tag would
// produce a message nobody can act on.
func TestAnnounceRejectsAnUnnamedRelease(t *testing.T) {
	tool := newAnnounceTool(t, `{"version":"","tag":"","sections":[]}`,
		map[string]string{discordSecret: "https://discord.example/webhook"})
	if _, err := call(t, tool, `{"dry_run":true}`); err == nil {
		t.Fatal("a release with no tag was accepted")
	}
}

// TestAnnounceRejectsMalformedReleaseJSON: a truncated file must fail loudly
// rather than announce an empty release.
func TestAnnounceRejectsMalformedReleaseJSON(t *testing.T) {
	tool := newAnnounceTool(t, `{"tag":"v1.0.0",`,
		map[string]string{discordSecret: "https://discord.example/webhook"})
	if _, err := call(t, tool, `{"dry_run":true}`); err == nil {
		t.Fatal("malformed release json was accepted")
	}
}

// TestAnnounceNeedsAConfiguredChannel: with an empty vault there is nowhere to
// post, and the error must name the secrets to set.
func TestAnnounceNeedsAConfiguredChannel(t *testing.T) {
	tool := newAnnounceTool(t, sampleReleaseJSON, nil)
	_, err := call(t, tool, `{"dry_run":true}`)
	if err == nil {
		t.Fatal("an unconfigured tool was accepted")
	}
	if !strings.Contains(err.Error(), discordSecret) {
		t.Fatalf("the error does not name the secret to set: %v", err)
	}
}

// TestAnnounceExplicitTargetWithoutEndpointIsAnError: silently announcing to
// fewer channels than asked is how a release goes unannounced unnoticed.
func TestAnnounceExplicitTargetWithoutEndpointIsAnError(t *testing.T) {
	tool := newAnnounceTool(t, sampleReleaseJSON, map[string]string{
		discordSecret: "https://discord.example/webhook",
	})
	_, err := call(t, tool, `{"targets":["telegram"],"dry_run":true}`)
	if err == nil {
		t.Fatal("an unconfigured explicit target was silently skipped")
	}
	if !strings.Contains(err.Error(), telegramSecret) {
		t.Fatalf("the error does not name the missing secret: %v", err)
	}
}

// TestAnnounceDefaultsToConfiguredChannelsOnly: omitting targets must pick up
// exactly the channels that have endpoints.
func TestAnnounceDefaultsToConfiguredChannelsOnly(t *testing.T) {
	tool := newAnnounceTool(t, sampleReleaseJSON, map[string]string{
		discordSecret:     "https://discord.example/webhook",
		telegramSecret:    "https://api.telegram.org/botX",
		telegramChatIDKey: "-100123",
	})
	out, err := call(t, tool, `{"dry_run":true}`)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !strings.Contains(out, "discord") || !strings.Contains(out, "telegram") {
		t.Fatalf("both configured channels should be listed:\n%s", out)
	}
}

// TestAnnounceRejectsUnknownTarget keeps the target set closed at the tool
// boundary too, not only inside the announce package.
func TestAnnounceRejectsUnknownTarget(t *testing.T) {
	tool := newAnnounceTool(t, sampleReleaseJSON, map[string]string{discordSecret: "https://x"})
	if _, err := call(t, tool, `{"targets":["slack"],"dry_run":true}`); err == nil {
		t.Fatal("an unknown target was accepted")
	}
}
