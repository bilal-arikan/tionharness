package skills

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/bilal-arikan/tionswarm/internal/seed"
	"strings"
	"testing"
)

// seedAndSplitGuide seeds the defaults into a temp dir and returns the dir plus
// the embedded tionswarm-guide frontmatter and body.
func seedAndSplitGuide(t *testing.T) (dir, guidePath, embedFM, embedBody string) {
	t.Helper()
	dir = t.TempDir()
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	guidePath = filepath.Join(dir, "tionswarm-guide", "SKILL.md")
	raw, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatal(err)
	}
	embedFM, embedBody = splitFrontmatter(string(raw))
	if embedFM == "" || embedBody == "" {
		t.Fatal("embedded guide skill has no frontmatter/body to split")
	}
	return dir, guidePath, embedFM, embedBody
}

// TestEnsureDefaultsBodyRefreshUnderUserFrontmatter covers the frontmatter-aware
// re-seed: the app tunes visibility frontmatter in place (access/group/…), so the
// whole-file hash never matches again. A pristine previously-shipped BODY under
// that tuned frontmatter must still be refreshed to the new embedded body, with
// the frontmatter preserved verbatim.
func TestEnsureDefaultsBodyRefreshUnderUserFrontmatter(t *testing.T) {
	dir, guide, embedFM, embedBody := seedAndSplitGuide(t)

	userFM := embedFM + "\naccess: shared\ngroup: TionSwarm"
	oldBody := "OLD SHIPPED BODY under a user-tuned frontmatter"
	if err := os.WriteFile(guide, rebuildSkillFile(userFM, oldBody), 0o644); err != nil {
		t.Fatal(err)
	}
	m := seed.LoadManifest(dir)
	m.Bodies["tionswarm-guide/SKILL.md"] = seed.SHA256Hex([]byte(oldBody))
	if err := seed.SaveManifest(dir, m); err != nil {
		t.Fatal(err)
	}

	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(guide)
	gotFM, gotBody := splitFrontmatter(string(got))
	if gotFM != userFM {
		t.Errorf("user frontmatter not preserved:\n got %q\nwant %q", gotFM, userFM)
	}
	if gotBody != embedBody {
		t.Errorf("pristine shipped body was NOT refreshed to the embedded body")
	}
	if seed.LoadManifest(dir).Bodies["tionswarm-guide/SKILL.md"] != seed.SHA256Hex([]byte(embedBody)) {
		t.Errorf("manifest body hash not updated after refresh")
	}
}

// TestEnsureDefaultsPreservesUserBodyEdit: a body that matches neither the
// embedded nor the last-shipped body is a user edit and must survive re-seeding,
// even under modified frontmatter.
func TestEnsureDefaultsPreservesUserBodyEdit(t *testing.T) {
	dir, guide, embedFM, _ := seedAndSplitGuide(t)

	userFM := embedFM + "\naccess: shared"
	userBody := "MY OWN body notes — hands off"
	content := rebuildSkillFile(userFM, userBody)
	if err := os.WriteFile(guide, content, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(guide)
	if string(got) != string(content) {
		t.Errorf("user body edit was clobbered:\n got %q", got)
	}
}

// TestEnsureDefaultsTracksBodyWhenOnlyFrontmatterTuned covers the unfreeze
// bootstrap: an install whose SKILL.md carries a tuned frontmatter but the
// CURRENT embedded body, under a legacy (flat, whole-hash-only) manifest. The
// file must be left alone but its body recorded as pristine, so the NEXT
// shipped body change can refresh it.
func TestEnsureDefaultsTracksBodyWhenOnlyFrontmatterTuned(t *testing.T) {
	dir, guide, embedFM, embedBody := seedAndSplitGuide(t)

	userFM := embedFM + "\naccess: shared"
	content := rebuildSkillFile(userFM, embedBody)
	if err := os.WriteFile(guide, content, 0o644); err != nil {
		t.Fatal(err)
	}
	// Legacy manifest: flat map with a stale whole-file hash (an old ship).
	legacy := map[string]string{"tionswarm-guide/SKILL.md": seed.SHA256Hex([]byte("some old whole-file ship"))}
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(dir, seed.ManifestName), data, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(guide)
	if string(got) != string(content) {
		t.Errorf("frontmatter-tuned file with current body must be left untouched")
	}
	if seed.LoadManifest(dir).Bodies["tionswarm-guide/SKILL.md"] != seed.SHA256Hex([]byte(embedBody)) {
		t.Errorf("current body under tuned frontmatter was not recorded as pristine")
	}
}

// TestLoadShippedManifestLegacyFlat: a pre-v2 flat {path: hash} manifest loads
// into Files with empty Bodies.
func TestLoadShippedManifestLegacyFlat(t *testing.T) {
	dir := t.TempDir()
	flat := `{"a/SKILL.md": "deadbeef"}`
	if err := os.WriteFile(filepath.Join(dir, seed.ManifestName), []byte(flat), 0o644); err != nil {
		t.Fatal(err)
	}
	m := seed.LoadManifest(dir)
	if m.Files["a/SKILL.md"] != "deadbeef" {
		t.Errorf("legacy flat manifest not folded into Files: %+v", m)
	}
	if len(m.Bodies) != 0 {
		t.Errorf("legacy manifest should have no body entries: %+v", m.Bodies)
	}
}

// TestRebuildSkillFileRoundTrip: rebuilding with split parts must produce a file
// whose split yields the same parts (hash stability across re-seeds).
func TestRebuildSkillFileRoundTrip(t *testing.T) {
	fm := "name: X\ndescription: d"
	body := "# Title\n\ncontent line\n"
	out := rebuildSkillFile(fm, body)
	gotFM, gotBody := splitFrontmatter(string(out))
	if gotFM != fm || gotBody != body {
		t.Errorf("round trip mismatch: fm %q body %q", gotFM, gotBody)
	}
	if !strings.HasPrefix(string(out), "---\n") {
		t.Errorf("rebuilt file must start with a frontmatter fence")
	}
}

// The Skills screen's badge/button depend on DefaultState being set for shipped
// GLOBAL skills and on RestoreDefault bringing a mangled one back.
func TestSkillDefaultStateAndRestore(t *testing.T) {
	dir, guide, embedFM, embedBody := seedAndSplitGuide(t)

	if got := DefaultState(dir, "tionswarm-guide"); got != seed.StateDefault {
		t.Errorf("freshly seeded skill = %q, want default", got)
	}
	// The app's own frontmatter edits (visibility/group) must NOT read as "edited":
	// they still auto-refresh, and badging them would warn about every toggled skill.
	if err := os.WriteFile(guide, rebuildSkillFile(embedFM+"\naccess: shared", embedBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := DefaultState(dir, "tionswarm-guide"); got != seed.StateTuned {
		t.Errorf("frontmatter-only change = %q, want tuned", got)
	}
	// A body edit is the case worth surfacing: this file stops receiving updates.
	if err := os.WriteFile(guide, rebuildSkillFile(embedFM, "MY OWN BODY"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := DefaultState(dir, "tionswarm-guide"); got != seed.StateEdited {
		t.Errorf("body edit = %q, want edited", got)
	}

	if err := RestoreDefault(dir, "tionswarm-guide"); err != nil {
		t.Fatal(err)
	}
	if got := DefaultState(dir, "tionswarm-guide"); got != seed.StateDefault {
		t.Errorf("after restore = %q, want default", got)
	}
	if _, gotBody := splitFrontmatter(string(mustRead(t, guide))); gotBody != embedBody {
		t.Error("restore did not bring back the shipped body")
	}
}

// A slug that is not shipped (or that could escape the tree) must report nothing
// and refuse to restore — the UI gates its button on exactly this.
func TestSkillHasDefaultAndRestoreGuards(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	if !HasDefault("tionswarm-guide") {
		t.Error("a shipped skill must report a default")
	}
	for _, slug := range []string{"", "my-own-skill", "../escape", "a/b"} {
		if HasDefault(slug) {
			t.Errorf("slug %q must not report a shipped default", slug)
		}
		if err := RestoreDefault(dir, slug); err == nil {
			t.Errorf("restoring %q must fail", slug)
		}
	}
	if got := DefaultState(dir, "my-own-skill"); got != seed.StateNone {
		t.Errorf("user-authored skill = %q, want none", got)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
