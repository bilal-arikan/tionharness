package insight

import "testing"

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

	ok, err := s.SetStatus(f.ID, StatusApplied, 999)
	if err != nil || !ok {
		t.Fatalf("SetStatus: ok=%v err=%v", ok, err)
	}
	got := s.List("", "")[0]
	if got.Status != StatusApplied || got.AppliedAt != 999 {
		t.Fatalf("status not applied: %+v", got)
	}
	if ok, _ := s.SetStatus("nope", StatusDismissed, 1); ok {
		t.Fatal("SetStatus on missing id should return false")
	}
}
