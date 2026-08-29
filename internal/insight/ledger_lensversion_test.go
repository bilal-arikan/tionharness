package insight

import "testing"

// TestLedgerLensVersionTriggersRescan pins the second re-scan trigger: with the
// session content unchanged, editing the lens (a new Version) makes the pair due
// again, while an unchanged version keeps skipping it.
func TestLedgerLensVersionTriggersRescan(t *testing.T) {
	l, err := OpenLedger(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fp := Fingerprint(5, 0)
	if err := l.Record(LedgerEntry{
		LensID: "tool-errors", SessionID: "SES1",
		SeenUpdatedAt: 100, SeenFingerprint: fp, SeenLensVersion: "v1", Status: "clean",
	}); err != nil {
		t.Fatal(err)
	}

	if l.NeedsScan("tool-errors", "SES1", 100, fp, "v1") {
		t.Fatal("same content and same lens version must skip")
	}
	if !l.NeedsScan("tool-errors", "SES1", 100, fp, "v2") {
		t.Fatal("an edited lens (new version) must re-scan unchanged content")
	}
}

// TestLedgerPreVersionEntriesAreGrandfathered pins the backward-compatibility
// choice: rows written before the version field (empty SeenLensVersion) count as
// up to date, so upgrading does not invalidate the whole ledger.
func TestLedgerPreVersionEntriesAreGrandfathered(t *testing.T) {
	l, err := OpenLedger(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fp := Fingerprint(5, 0)
	if err := l.Record(LedgerEntry{
		LensID: "tool-errors", SessionID: "SES1",
		SeenUpdatedAt: 100, SeenFingerprint: fp, Status: "clean", // no SeenLensVersion
	}); err != nil {
		t.Fatal(err)
	}
	if l.NeedsScan("tool-errors", "SES1", 100, fp, "v9") {
		t.Fatal("a pre-version ledger row must not be re-scanned on upgrade")
	}
	// A real content change still wins over the grandfather rule.
	if !l.NeedsScan("tool-errors", "SES1", 100, Fingerprint(6, 0), "v9") {
		t.Fatal("content change must still trigger a re-scan on a pre-version row")
	}
}

// TestLensVersionSensitivity pins what Lens.Version() reacts to: the analysis
// body, prefilter, scope and model change it; cosmetic fields do not.
func TestLensVersionSensitivity(t *testing.T) {
	base := Lens{
		ID: "l1", Name: "One", Enabled: true, Prompt: "analyze errors",
		Scope: []string{"steps", "debug"}, Model: "claude-cli",
		Prefilter: Prefilter{RequiresAny: []string{"error", "tool"}, MinTokens: 500},
	}
	v := base.Version()

	same := base
	same.Name = "Renamed"
	same.Description = "new description"
	same.Enabled = false
	same.Path = "/elsewhere/l1.md"
	same.Scope = []string{"debug", "steps"}                                            // reordered set
	same.Prefilter = Prefilter{RequiresAny: []string{"tool", "error"}, MinTokens: 500} // reordered set
	if same.Version() != v {
		t.Fatal("cosmetic edits and reordered sets must not change the lens version")
	}

	for name, mutate := range map[string]func(*Lens){
		"prompt":    func(l *Lens) { l.Prompt = "analyze errors AND timeouts" },
		"model":     func(l *Lens) { l.Model = "claude-opus-5" },
		"scope":     func(l *Lens) { l.Scope = []string{"steps"} },
		"prefilter": func(l *Lens) { l.Prefilter.MinTokens = 1000 },
	} {
		changed := base
		mutate(&changed)
		if changed.Version() == v {
			t.Fatalf("changing the %s must change the lens version", name)
		}
	}
}
