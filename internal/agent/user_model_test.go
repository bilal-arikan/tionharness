package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestUserModelTunableDefault verifies HA-1 user modelling is on by default and
// can be toggled.
func TestUserModelTunableDefault(t *testing.T) {
	tun := NewTunables()
	if !tun.UserModel() {
		t.Fatal("UserModel should default to true")
	}
	tun.SetUserModel(false)
	if tun.UserModel() {
		t.Fatal("UserModel should be false after SetUserModel(false)")
	}
}

// TestWriteUserModelTruncates ensures the HA-1 writer honours the human block's
// character limit: an over-limit profile is truncated to fit rather than rejected,
// so the dream cycle always lands a (possibly clipped) update.
func TestWriteUserModelTruncates(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "ws"))
	ctx := context.Background()

	ag, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Modeled"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Shrink the human block's limit so a normal profile overruns it.
	if err := rt.mem.DefineCoreBlock(ctx, ag.ID, db.CoreBlock{Label: "human", CharLimit: 10}); err != nil {
		t.Fatalf("define human: %v", err)
	}

	if err := rt.writeUserModel(ctx, ag.ID, "this profile is far longer than ten characters"); err != nil {
		t.Fatalf("writeUserModel: %v", err)
	}
	got, _ := rt.mem.ReadCore(ctx, ag.ID, "human")
	if n := len([]rune(got)); n > 10 || n == 0 {
		t.Fatalf("human block = %q (%d runes), want 1..10 after truncation", got, n)
	}

	// A within-limit profile is written verbatim.
	if err := rt.writeUserModel(ctx, ag.ID, "name: Bil"); err != nil {
		t.Fatalf("writeUserModel small: %v", err)
	}
	got, _ = rt.mem.ReadCore(ctx, ag.ID, "human")
	if got != "name: Bil" {
		t.Fatalf("human block = %q, want verbatim", got)
	}
}
