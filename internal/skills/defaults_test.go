package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	m := loadShippedManifest(dir)
	m.Bodies["tionswarm-guide/SKILL.md"] = sha256Hex([]byte(oldBody))
	if err := saveShippedManifest(dir, m); err != nil {
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
	if loadShippedManifest(dir).Bodies["tionswarm-guide/SKILL.md"] != sha256Hex([]byte(embedBody)) {
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
	legacy := map[string]string{"tionswarm-guide/SKILL.md": sha256Hex([]byte("some old whole-file ship"))}
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(dir, shippedManifestName), data, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(guide)
	if string(got) != string(content) {
		t.Errorf("frontmatter-tuned file with current body must be left untouched")
	}
	if loadShippedManifest(dir).Bodies["tionswarm-guide/SKILL.md"] != sha256Hex([]byte(embedBody)) {
		t.Errorf("current body under tuned frontmatter was not recorded as pristine")
	}
}

// TestLoadShippedManifestLegacyFlat: a pre-v2 flat {path: hash} manifest loads
// into Files with empty Bodies.
func TestLoadShippedManifestLegacyFlat(t *testing.T) {
	dir := t.TempDir()
	flat := `{"a/SKILL.md": "deadbeef"}`
	if err := os.WriteFile(filepath.Join(dir, shippedManifestName), []byte(flat), 0o644); err != nil {
		t.Fatal(err)
	}
	m := loadShippedManifest(dir)
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
