package insight

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestFindingUpsertDedupesBySignature(t *testing.T) {
	root := t.TempDir()
	s, err := OpenFindingStore(root)
	if err != nil {
		t.Fatal(err)
	}

	first, err := s.Upsert(Finding{
		LensID: "tool-errors", Channel: ChannelAppFix, Signature: "Bash:file-too-long",
		Title: "Windows cmdline limit", EvidenceSessionIDs: []string{"SES1"},
		FirstSeen: 100, LastSeen: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Occurrences != 1 || first.Status != StatusNew {
		t.Fatalf("new finding: occ=%d status=%s", first.Occurrences, first.Status)
	}

	// Same signature+lens -> merge, not a new row.
	merged, err := s.Upsert(Finding{
		LensID: "tool-errors", Channel: ChannelAppFix, Signature: "Bash:file-too-long",
		Title: "Windows cmdline limit (v2)", EvidenceSessionIDs: []string{"SES2"},
		LastSeen: 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Occurrences != 2 {
		t.Fatalf("expected occurrences=2 after repeat, got %d", merged.Occurrences)
	}
	if merged.LastSeen != 200 {
		t.Fatalf("expected LastSeen refreshed to 200, got %d", merged.LastSeen)
	}
	if len(merged.EvidenceSessionIDs) != 2 {
		t.Fatalf("expected 2 evidence sessions, got %v", merged.EvidenceSessionIDs)
	}
	if got := s.List("", ""); len(got) != 1 {
		t.Fatalf("expected 1 stored finding after dedupe, got %d", len(got))
	}
}

func TestFindingStorePersistsAndFilters(t *testing.T) {
	root := t.TempDir()
	s, err := OpenFindingStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upsert(Finding{LensID: "tool-errors", Channel: ChannelAppFix, Signature: "a", Title: "A", LastSeen: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upsert(Finding{LensID: "skill-usage-opt", Channel: ChannelWorkspaceOpt, Signature: "b", Title: "B", LastSeen: 2}); err != nil {
		t.Fatal(err)
	}

	if got := s.List("", ChannelAppFix); len(got) != 1 || got[0].LensID != "tool-errors" {
		t.Fatalf("channel filter failed: %+v", got)
	}

	// Reopen -> persisted.
	reopened, err := OpenFindingStore(root)
	if err != nil {
		t.Fatal(err)
	}
	all := reopened.List("", "")
	if len(all) != 2 {
		t.Fatalf("expected 2 persisted findings, got %d", len(all))
	}
	// List is newest-first by LastSeen.
	if all[0].LensID != "skill-usage-opt" {
		t.Fatalf("expected newest-first order, got %s first", all[0].LensID)
	}
}

func TestFindingSetStatus(t *testing.T) {
	root := t.TempDir()
	s, _ := OpenFindingStore(root)
	f, _ := s.Upsert(Finding{LensID: "l", Channel: ChannelWorkspaceOpt, Signature: "s", Title: "t", LastSeen: 1})

	ev := &AppliedEntity{EntityType: "skill", EntityID: "tionharness-tool-discovery"}
	ok, err := s.SetStatus(f.ID, StatusApplied, 999, ev)
	if err != nil || !ok {
		t.Fatalf("SetStatus: ok=%v err=%v", ok, err)
	}
	got := s.List("", "")[0]
	if got.Status != StatusApplied || got.AppliedAt != 999 {
		t.Fatalf("status not applied: %+v", got)
	}
	if !got.AppliedEntity.Valid() || got.AppliedEntity.EntityID != "tionharness-tool-discovery" {
		t.Fatalf("evidence not recorded: %+v", got.AppliedEntity)
	}
	if ok, _ := s.SetStatus("nope", StatusDismissed, 1, nil); ok {
		t.Fatal("SetStatus on missing id should return false")
	}
}

// The evidence gate: "applied" without a named entity is a claim nobody can
// check, so it must fail loudly instead of being downgraded to accepted.
func TestFindingAppliedRequiresEvidence(t *testing.T) {
	root := t.TempDir()
	s, _ := OpenFindingStore(root)
	f, _ := s.Upsert(Finding{LensID: "l", Channel: ChannelWorkspaceOpt, Signature: "s", Title: "t", LastSeen: 1})

	for _, ev := range []*AppliedEntity{nil, {EntityType: "skill"}, {EntityID: "x"}} {
		if _, err := s.SetStatus(f.ID, StatusApplied, 5, ev); !errors.Is(err, ErrAppliedNeedsEvidence) {
			t.Fatalf("evidence %+v: expected ErrAppliedNeedsEvidence, got %v", ev, err)
		}
	}
	if got := s.List("", "")[0]; got.Status != StatusNew {
		t.Fatalf("rejected transition must not change the status: %+v", got)
	}
	// accepted/dismissed stay free of the gate.
	if ok, err := s.SetStatus(f.ID, StatusAccepted, 6, nil); err != nil || !ok {
		t.Fatalf("accepted without evidence: ok=%v err=%v", ok, err)
	}
}

func TestFindingSetStatusMany(t *testing.T) {
	s, _ := OpenFindingStore(t.TempDir())
	a, _ := s.Upsert(Finding{LensID: "l", Signature: "a", Title: "a", LastSeen: 1})
	b, _ := s.Upsert(Finding{LensID: "l", Signature: "b", Title: "b", LastSeen: 2})

	updated, err := s.SetStatusMany([]string{a.ID, b.ID, "missing"}, StatusDismissed, 7, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 2 {
		t.Fatalf("expected 2 updated ids, got %v", updated)
	}
	for _, f := range s.List("", "") {
		if f.Status != StatusDismissed {
			t.Fatalf("finding %s not dismissed: %+v", f.ID, f)
		}
	}
	// Reload from disk: one rewrite must have persisted both.
	reloaded, _ := OpenFindingStore(filepath.Dir(filepath.Dir(s.path)))
	for _, f := range reloaded.List("", "") {
		if f.Status != StatusDismissed {
			t.Fatalf("not persisted: %+v", f)
		}
	}
}

func TestFindingUpsertMergesSameTopicAcrossLenses(t *testing.T) {
	s, err := OpenFindingStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Same root cause, two lenses, each with its own lens-prefixed slug.
	if _, err := s.Upsert(Finding{
		LensID: "tool-errors", Channel: ChannelAppFix,
		Signature: "tool-errors:bash_file_too_long", Title: "cmdline limit", LastSeen: 100,
	}); err != nil {
		t.Fatal(err)
	}
	merged, err := s.Upsert(Finding{
		LensID: "lessons-mining", Channel: ChannelAppFix,
		Signature: "lessons-mining:bash file too long", Title: "cmdline limit", LastSeen: 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Occurrences != 2 {
		t.Fatalf("cross-lens repeat should merge, got occurrences=%d", merged.Occurrences)
	}
	if got := s.List("", ""); len(got) != 1 {
		t.Fatalf("expected 1 card for one root cause, got %d", len(got))
	}
	// A different channel is never merged, even with the same topic.
	if _, err := s.Upsert(Finding{
		LensID: "skill-usage-opt", Channel: ChannelWorkspaceOpt,
		Signature: "skill-usage-opt:bash_file_too_long", Title: "cmdline limit", LastSeen: 300,
	}); err != nil {
		t.Fatal(err)
	}
	if got := s.List("", ""); len(got) != 2 {
		t.Fatalf("cross-channel merge must not happen, got %d cards", len(got))
	}
}

func TestCanonTopicDropsLensPrefixAndVolatileTokens(t *testing.T) {
	cases := []struct{ lens, sig, want string }{
		{"tool-errors", "tool-errors:read missing arg", "read missing arg"},
		{"lessons-mining", "lessons-mining:read_missing-arg", "read missing arg"},
		{"tool-errors", "tool-errors:timeout in SES2047 msg12", "timeout in"},
		{"l", "l:low", ""},                      // single short token: too generic to match on
		{"l", "l:a1f2b3c4d5e6", "a1f2b3c4d5e6"}, // hash fallback signature survives alone
	}
	for _, c := range cases {
		if got := canonTopic(c.lens, c.sig); got != c.want {
			t.Fatalf("canonTopic(%q, %q) = %q, want %q", c.lens, c.sig, got, c.want)
		}
	}
}
