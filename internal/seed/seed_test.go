package seed

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// The tree under test: one body-aware file (config + body) and one opaque file.
// `cfg:` is the user-owned key; the body is ours.
func testCfg(dir, body string) Config {
	fsys := fstest.MapFS{
		"defaults/a.md":  &fstest.MapFile{Data: []byte("---\ncfg: shipped\n---\n" + body)},
		"defaults/b.txt": &fstest.MapFile{Data: []byte("opaque " + body)},
	}
	return Config{
		FS: fsys, Root: "defaults", Dir: dir,
		BodyAware: func(name string) bool { return filepath.Ext(name) == ".md" },
		Body:      func(c []byte) string { return splitTestBody(string(c)) },
		// Policy under test: adopt the shipped file, carry the user's cfg back.
		Merge: func(onDisk, embedded []byte) []byte {
			return []byte("---\ncfg: " + splitTestCfg(string(onDisk)) + "\n---\n" + splitTestBody(string(embedded)))
		},
	}
}

func splitTestBody(s string) string {
	if i := indexAfterFence(s); i >= 0 {
		return s[i:]
	}
	return s
}

func splitTestCfg(s string) string {
	const k = "cfg: "
	i := indexOf(s, k)
	if i < 0 {
		return ""
	}
	rest := s[i+len(k):]
	if j := indexOf(rest, "\n"); j >= 0 {
		return rest[:j]
	}
	return rest
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func indexAfterFence(s string) int {
	if i := indexOf(s, "\n---\n"); i >= 0 {
		return i + len("\n---\n")
	}
	return -1
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// An untouched prior ship must be REFRESHED when the shipped content changes —
// this is the whole point: an improvement has to reach installs that already have
// the file. A naive missing-only seed leaves it frozen forever.
func TestEnsureRefreshesUntouchedPriorShip(t *testing.T) {
	dir := t.TempDir()
	if err := Ensure(testCfg(dir, "V1")); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(dir, "a.md")); splitTestBody(got) != "V1" {
		t.Fatalf("initial seed body = %q", splitTestBody(got))
	}

	// Ship V2 over an install that never touched the file.
	if err := Ensure(testCfg(dir, "V2")); err != nil {
		t.Fatal(err)
	}
	if got := splitTestBody(read(t, filepath.Join(dir, "a.md"))); got != "V2" {
		t.Errorf("untouched prior ship not refreshed: body = %q, want V2", got)
	}
	if got := read(t, filepath.Join(dir, "b.txt")); got != "opaque V2" {
		t.Errorf("opaque file not refreshed: %q", got)
	}
}

// A real user edit must survive every future ship, body-aware or not.
func TestEnsurePreservesUserEdit(t *testing.T) {
	dir := t.TempDir()
	if err := Ensure(testCfg(dir, "V1")); err != nil {
		t.Fatal(err)
	}
	mine := "---\ncfg: mine\n---\nMY OWN BODY"
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("my opaque"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Ensure(testCfg(dir, "V2")); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(dir, "a.md")); got != mine {
		t.Errorf("user body edit clobbered: %q", got)
	}
	if got := read(t, filepath.Join(dir, "b.txt")); got != "my opaque" {
		t.Errorf("user edit to an opaque file clobbered: %q", got)
	}
}

// The case the Bodies ledger exists for: the APP rewrites config in place (a
// toggle), so the whole-file hash never matches again. The body is still a
// pristine prior ship, so a shipped body update must land through Merge — and the
// user's config must survive it.
func TestEnsureMergesShippedBodyUnderChangedConfig(t *testing.T) {
	dir := t.TempDir()
	if err := Ensure(testCfg(dir, "V1")); err != nil {
		t.Fatal(err)
	}
	// Simulate the UI toggle: config changed, body untouched.
	toggled := "---\ncfg: mine\n---\nV1"
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte(toggled), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Ensure(testCfg(dir, "V2")); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(dir, "a.md"))
	if splitTestBody(got) != "V2" {
		t.Errorf("shipped body did not land under changed config: %q", got)
	}
	if splitTestCfg(got) != "mine" {
		t.Errorf("user config not carried over: %q", got)
	}

	// And the merge must not freeze the file: a THIRD ship still refreshes it.
	if err := Ensure(testCfg(dir, "V3")); err != nil {
		t.Fatal(err)
	}
	got3 := read(t, filepath.Join(dir, "a.md"))
	if splitTestBody(got3) != "V3" || splitTestCfg(got3) != "mine" {
		t.Errorf("merged file froze after one refresh: %q", got3)
	}
}

// Bootstrapping without a manifest: nothing is overwritten, but a file that
// already matches the embedded tree is RECORDED, so the next ship can refresh it.
func TestEnsureBootstrapsHashesWithoutManifest(t *testing.T) {
	dir := t.TempDir()
	if err := Ensure(testCfg(dir, "V1")); err != nil {
		t.Fatal(err)
	}
	// Wipe the ledger: an install that predates it.
	if err := os.Remove(filepath.Join(dir, ManifestName)); err != nil {
		t.Fatal(err)
	}
	// Re-running against the SAME version must re-record it as pristine...
	if err := Ensure(testCfg(dir, "V1")); err != nil {
		t.Fatal(err)
	}
	if m := LoadManifest(dir); m.Files["a.md"] == "" || m.Bodies["a.md"] == "" {
		t.Fatalf("hashes did not self-seed for a pristine file: %+v", m)
	}
	// ...so the NEXT ship refreshes it.
	if err := Ensure(testCfg(dir, "V2")); err != nil {
		t.Fatal(err)
	}
	if got := splitTestBody(read(t, filepath.Join(dir, "a.md"))); got != "V2" {
		t.Errorf("self-seeded file not refreshed by the next ship: %q", got)
	}
}

// The honest limitation, pinned so nobody assumes otherwise: a file shipped
// BEFORE the ledger existed and since superseded is indistinguishable from a user
// edit, so Ensure leaves it. Restore is the deliberate way out, and it also
// re-arms the automatic refresh.
func TestRestoreRecoversPreLedgerFileAndReArmsRefresh(t *testing.T) {
	dir := t.TempDir()
	// An old install: V1 on disk, no ledger at all.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := "---\ncfg: shipped\n---\nV1"
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Ensure(testCfg(dir, "V2")); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(dir, "a.md")); got != old {
		t.Fatalf("a file of unknown provenance must NOT be overwritten, got %q", got)
	}

	// The user asks for the default back.
	if err := Restore(testCfg(dir, "V2"), "a.md"); err != nil {
		t.Fatal(err)
	}
	if got := splitTestBody(read(t, filepath.Join(dir, "a.md"))); got != "V2" {
		t.Fatalf("restore did not write the shipped default: %q", got)
	}
	// Restored => recorded => the NEXT ship refreshes it without asking again.
	if err := Ensure(testCfg(dir, "V3")); err != nil {
		t.Fatal(err)
	}
	if got := splitTestBody(read(t, filepath.Join(dir, "a.md"))); got != "V3" {
		t.Errorf("restore did not re-arm the automatic refresh: %q", got)
	}
}

func TestRestoreUnknownFileAndHasDefault(t *testing.T) {
	dir := t.TempDir()
	cfg := testCfg(dir, "V1")
	if err := Restore(cfg, "nope.md"); err == nil {
		t.Error("restoring a file with no embedded default must fail")
	}
	if !HasDefault(cfg, "a.md") {
		t.Error("a.md is shipped")
	}
	if HasDefault(cfg, "nope.md") {
		t.Error("nope.md is not shipped")
	}
}

// A blank Dir is a no-op (callers pass one before a workspace exists).
func TestEnsureBlankDirIsNoOp(t *testing.T) {
	if err := Ensure(testCfg("", "V1")); err != nil {
		t.Fatalf("blank dir should be a no-op, got %v", err)
	}
}

// Status is what makes the ledger legible in a UI. The distinction that matters:
// 'tuned' STILL auto-refreshes (only config differs), 'edited' does NOT — so a UI
// must be able to warn about the second without crying wolf about the first.
func TestStatus(t *testing.T) {
	dir := t.TempDir()
	cfg := testCfg(dir, "V1")
	if err := Ensure(cfg); err != nil {
		t.Fatal(err)
	}

	if got := Status(cfg, "a.md"); got != StateDefault {
		t.Errorf("freshly seeded = %q, want %q", got, StateDefault)
	}
	if got := Status(cfg, "b.txt"); got != StateDefault {
		t.Errorf("opaque freshly seeded = %q, want %q", got, StateDefault)
	}

	// Config-only change (a UI toggle) → tuned, and it must still refresh.
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("---\ncfg: mine\n---\nV1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Status(cfg, "a.md"); got != StateTuned {
		t.Errorf("config-only change = %q, want %q", got, StateTuned)
	}

	// Body change → edited (frozen).
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("---\ncfg: shipped\n---\nMINE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Status(cfg, "a.md"); got != StateEdited {
		t.Errorf("body change = %q, want %q", got, StateEdited)
	}
	// An opaque file has no body split — any change is an edit.
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Status(cfg, "b.txt"); got != StateEdited {
		t.Errorf("opaque change = %q, want %q", got, StateEdited)
	}

	// No shipped default, or nothing on disk → no state to report.
	if got := Status(cfg, "nope.md"); got != StateNone {
		t.Errorf("unshipped = %q, want none", got)
	}
	if err := os.Remove(filepath.Join(dir, "a.md")); err != nil {
		t.Fatal(err)
	}
	if got := Status(cfg, "a.md"); got != StateNone {
		t.Errorf("missing on disk = %q, want none", got)
	}
}

// The states Status reports must line up with what Ensure actually DOES, or the
// badge lies: 'tuned' promises a future refresh and 'edited' promises none.
func TestStatusMatchesRefreshBehaviour(t *testing.T) {
	dir := t.TempDir()
	if err := Ensure(testCfg(dir, "V1")); err != nil {
		t.Fatal(err)
	}
	tunedPath := filepath.Join(dir, "a.md")
	if err := os.WriteFile(tunedPath, []byte("---\ncfg: mine\n---\nV1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Status(testCfg(dir, "V1"), "a.md"); got != StateTuned {
		t.Fatalf("setup: want tuned, got %q", got)
	}
	if err := Ensure(testCfg(dir, "V2")); err != nil {
		t.Fatal(err)
	}
	if got := splitTestBody(read(t, tunedPath)); got != "V2" {
		t.Errorf("'tuned' must keep auto-refreshing, body = %q", got)
	}

	// Now edit the body: the badge says frozen, so Ensure must leave it.
	edited := "---\ncfg: mine\n---\nMY BODY"
	if err := os.WriteFile(tunedPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Status(testCfg(dir, "V2"), "a.md"); got != StateEdited {
		t.Fatalf("setup: want edited, got %q", got)
	}
	if err := Ensure(testCfg(dir, "V3")); err != nil {
		t.Fatal(err)
	}
	if got := read(t, tunedPath); got != edited {
		t.Errorf("'edited' must NOT be refreshed, got %q", got)
	}
}
