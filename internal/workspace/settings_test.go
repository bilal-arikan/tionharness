package workspace

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestDefaultWSSettings pins the always-on defaults (cross-session awareness is
// no longer configurable; codebase-memory defaults on).
func TestDefaultWSSettings(t *testing.T) {
	d := defaultWSSettings()
	if !d.CodebaseMemoryEnabled {
		t.Error("codebase-memory capability should default on")
	}
	if d.WorktreeBaseRef != "" || d.WorktreeRootDir != "" {
		t.Fatalf("worktree lifecycle defaults = base %q root %q, want empty fallbacks", d.WorktreeBaseRef, d.WorktreeRootDir)
	}
}

func TestWSSettingsWorktreeLifecycleJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	written := &Workspace{DataDir: dir}
	written.settings.cur = defaultWSSettings()
	written.settings.cur.WorktreeBaseRef = "origin/develop"
	written.settings.cur.WorktreeRootDir = `C:\worktrees\project`
	if err := written.saveSettings(); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	data, err := os.ReadFile(written.settingsPath())
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}
	for _, field := range []string{`"worktreeBaseRef": "origin/develop"`, `"worktreeRootDir": "C:\\worktrees\\project"`} {
		if !bytes.Contains(data, []byte(field)) {
			t.Fatalf("persisted settings missing %s: %s", field, data)
		}
	}

	loaded := &Workspace{DataDir: dir}
	loaded.loadSettings()
	got := loaded.Settings()
	if got.WorktreeBaseRef != "origin/develop" || got.WorktreeRootDir != `C:\worktrees\project` {
		t.Fatalf("reloaded worktree settings = base %q root %q", got.WorktreeBaseRef, got.WorktreeRootDir)
	}
}

// TestCheckDefaultAgentRejectsSystemAgent pins the gate every DefaultAgentId
// write goes through: a system agent is refused, a normal agent accepted, and an
// unknown id left to the caller's own fallback.
func TestCheckDefaultAgentRejectsSystemAgent(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sysAgent, err := store.CreateAgent(ctx, db.Agent{Name: "Titler", System: true, SystemKey: "titler"})
	if err != nil {
		t.Fatal(err)
	}
	normal, err := store.CreateAgent(ctx, db.Agent{Name: "PM"})
	if err != nil {
		t.Fatal(err)
	}
	w := &Workspace{DataDir: t.TempDir(), DB: store}

	if err := w.checkDefaultAgent(sysAgent.ID); !errors.Is(err, ErrDefaultAgentSystem) {
		t.Fatalf("system agent accepted as default: %v", err)
	}
	if err := w.checkDefaultAgent(normal.ID); err != nil {
		t.Fatalf("normal agent rejected: %v", err)
	}
	for _, id := range []string{"", "AGT-does-not-exist"} {
		if err := w.checkDefaultAgent(id); err != nil {
			t.Fatalf("checkDefaultAgent(%q) = %v, want nil", id, err)
		}
	}
}

// TestSanitizeDefaultAgentClearsSystemAgent covers the boot self-heal for a
// ws-settings.json written before the gate existed.
func TestSanitizeDefaultAgentClearsSystemAgent(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sysAgent, err := store.CreateAgent(ctx, db.Agent{Name: "Titler", System: true, SystemKey: "titler"})
	if err != nil {
		t.Fatal(err)
	}
	w := &Workspace{DataDir: t.TempDir(), DB: store}
	w.settings.cur = defaultWSSettings()
	w.settings.cur.DefaultAgentId = sysAgent.ID

	w.sanitizeDefaultAgent(slog.New(slog.NewTextHandler(io.Discard, nil)))

	if got := w.Settings().DefaultAgentId; got != "" {
		t.Fatalf("defaultAgentId = %q, want cleared", got)
	}
	data, err := os.ReadFile(w.settingsPath())
	if err != nil {
		t.Fatalf("settings not persisted: %v", err)
	}
	if !bytes.Contains(data, []byte(`"defaultAgentId": ""`)) {
		t.Fatalf("cleared defaultAgentId not persisted: %s", data)
	}
}
