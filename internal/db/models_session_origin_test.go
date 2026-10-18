package db

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDeriveOrigin pins the legacy-field → origin derivation, which is both the
// boot backfill for pre-v4 headers and the default at creation when a caller
// passes no explicit origin. The two must agree, so one table covers both.
func TestDeriveOrigin(t *testing.T) {
	cases := []struct {
		name string
		in   Session
		want SessionOrigin
	}{
		{"plain chat", Session{Kind: "chat"}, SessionOrigin{Kind: OriginUser}},
		{"empty kind is chat", Session{}, SessionOrigin{Kind: OriginUser}},
		{"detached spawn", Session{Kind: "spawned"}, SessionOrigin{Kind: OriginSpawn}},
		{
			"subagent child",
			Session{Kind: "spawned", ExecutionType: ExecutionSubagent, ParentSessionID: "SES7"},
			SessionOrigin{Kind: OriginSubagent, TriggerSessionID: "SES7", RootSessionID: "SES7"},
		},
		{"insight scan", Session{Kind: "insight", SourceID: "scan-1"}, SessionOrigin{Kind: OriginInsight, RunID: "scan-1"}},
		{"schedule thread", Session{Kind: "schedule"}, SessionOrigin{Kind: OriginSchedule}},
		{"schedule spawn", Session{Kind: "schedule-run"}, SessionOrigin{Kind: OriginSchedule}},
		{"automation thread", Session{Kind: "automation", SourceID: "AUT4"}, SessionOrigin{Kind: OriginAutomation, EntityID: "AUT4"}},
		{
			"automation one-shot keeps the tagged parent",
			Session{Kind: "automation-run", ParentSessionID: "SES5"},
			SessionOrigin{Kind: OriginAutomation, TriggerSessionID: "SES5"},
		},
		{
			"handoff continuation",
			Session{Kind: "chat", ParentSessionID: "SES2"},
			SessionOrigin{Kind: OriginHandoff, TriggerSessionID: "SES2", RootSessionID: "SES2"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveOrigin(tc.in); got != tc.want {
				t.Fatalf("deriveOrigin = %+v, want %+v", got, tc.want)
			}
			// Lineage() on a header without Origin must yield the same answer.
			tc.in.CreatedAt = 42
			want := tc.want
			want.At = 42
			if got := tc.in.Lineage(); got != want {
				t.Fatalf("Lineage = %+v, want %+v", got, want)
			}
		})
	}
}

// TestCreateSessionStampsOrigin: every creation path stamps an origin — the
// derived default when none is given, the caller's when one is — and an unknown
// kind is rejected rather than persisted.
func TestCreateSessionStampsOrigin(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	ag, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})

	plain, err := d.CreateSession(ctx, Session{AgentID: ag.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if plain.Origin == nil || plain.Origin.Kind != OriginUser || plain.Origin.At != plain.CreatedAt {
		t.Fatalf("default origin = %+v, want user@createdAt", plain.Origin)
	}
	if plain.SchemaVersion != SessionSchemaVersion || SessionSchemaVersion < 4 {
		t.Fatalf("schema version = %d, want %d (>=4 for origin)", plain.SchemaVersion, SessionSchemaVersion)
	}
	if plain.RootSession() != plain.ID {
		t.Fatalf("a root session's RootSession() = %q, want its own id %q", plain.RootSession(), plain.ID)
	}

	explicit, err := d.CreateSession(ctx, Session{
		AgentID: ag.ID, Kind: "automation-run",
		Origin: &SessionOrigin{Kind: OriginAutomation, EntityID: "AUT1", TriggerSessionID: plain.ID},
	})
	if err != nil {
		t.Fatalf("create explicit: %v", err)
	}
	if explicit.Origin.EntityID != "AUT1" || explicit.Origin.TriggerSessionID != plain.ID || explicit.Origin.At == 0 {
		t.Fatalf("explicit origin not preserved/stamped: %+v", explicit.Origin)
	}

	if _, err := d.CreateSession(ctx, Session{AgentID: ag.ID, Origin: &SessionOrigin{Kind: "martian"}}); err == nil {
		t.Fatal("an unknown origin kind must be rejected")
	}
}

// TestLegacyHeaderOriginBackfilledOnLoad: a header written before schema v4 has
// no origin on disk. The loader derives it in memory so Lineage()/RootSession()
// work, and it does NOT rewrite the file just for that.
func TestLegacyHeaderOriginBackfilledOnLoad(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "store")
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ag, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	parent, _ := d.CreateSession(ctx, Session{AgentID: ag.ID, Title: "parent"})
	child, _ := d.CreateSession(ctx, Session{AgentID: ag.ID, ParentSessionID: parent.ID})
	_ = d.Close()

	// Simulate the pre-v4 header: strip origin + downgrade the version on disk.
	headerPath := filepath.Join(dir, dirSessions, child.ID, sessionHeaderFile)
	raw, err := os.ReadFile(headerPath)
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	var hdr map[string]json.RawMessage
	if err := json.Unmarshal(raw, &hdr); err != nil {
		t.Fatalf("decode header: %v", err)
	}
	delete(hdr, "origin")
	hdr["v"] = json.RawMessage("3")
	legacy, _ := json.Marshal(hdr)
	if err := os.WriteFile(headerPath, legacy, 0o644); err != nil {
		t.Fatalf("write legacy header: %v", err)
	}

	d2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer d2.Close()
	got, err := d2.GetSession(ctx, child.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Origin == nil || got.Origin.Kind != OriginHandoff || got.Origin.TriggerSessionID != parent.ID {
		t.Fatalf("legacy handoff origin not backfilled: %+v", got.Origin)
	}
	if got.Origin.At != got.CreatedAt {
		t.Fatalf("backfilled At = %d, want CreatedAt %d", got.Origin.At, got.CreatedAt)
	}
	after, _ := os.ReadFile(headerPath)
	if string(after) != string(legacy) {
		t.Fatal("boot must not rewrite a legacy header just to add the origin")
	}
}
