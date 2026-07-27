package tools

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The tool-level happy paths (read-then-edit, blind write, out-of-band change)
// live in builtin_fs_test.go / builtin_patch_test.go. These cover the unit-level
// contract of the tracker itself: the invariants those tools rely on but never
// exercise directly.

// A nil *ReadTracker is a documented valid value — catalog/preview builds and
// tests construct fs tools without one. Every method must tolerate it, and the
// guard must degrade to "no enforcement" rather than panicking or rejecting.
func TestNilTrackerIsANoOp(t *testing.T) {
	var tr *ReadTracker

	tr.Record("/some/path", ReadRecord{Size: 1})

	if rec, ok := tr.Get("/some/path"); ok || rec.Size != 0 {
		t.Fatalf("nil tracker Get should report no record, got %+v ok=%v", rec, ok)
	}
	if err := checkFreshness(nil, "/never/read", []byte("anything")); err != nil {
		t.Fatalf("nil tracker must disable the guard, got %v", err)
	}
	recordWritten(nil, "/some/path", []byte("content"))
}

// checkFreshness compares a content hash, NOT mtime. A cloud-sync or antivirus
// touch bumps mtime without changing bytes; treating that as a modification
// would make Edit fail for a file nobody actually changed.
func TestFreshnessIgnoresMtimeBumpWhenContentIsIdentical(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	content := []byte("stable content")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	tr := NewReadTracker()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	tr.Record(path, ReadRecord{ModTime: info.ModTime(), Size: int64(len(content)), Sum: contentSum(content)})

	// Bump mtime an hour into the future, bytes untouched.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkFreshness(tr, path, current); err != nil {
		t.Fatalf("mtime-only change must not trip the guard: %v", err)
	}
}

// The mirror case: an edit that leaves size and mtime untouched (in-place
// single-character swap) must still be caught, because the check is on content.
func TestFreshnessCatchesSameSizeSameMtimeEdit(t *testing.T) {
	tr := NewReadTracker()
	original := []byte("aaaa")
	now := time.Now()
	tr.Record("/f", ReadRecord{ModTime: now, Size: int64(len(original)), Sum: contentSum(original)})

	err := checkFreshness(tr, "/f", []byte("aaab")) // same length, same recorded mtime
	if err == nil {
		t.Fatal("same-size content change must trip the stale guard")
	}
	if !strings.Contains(err.Error(), "modified since") {
		t.Fatalf("want modified-since error, got %v", err)
	}
}

func TestFreshnessDistinguishesNeverReadFromStale(t *testing.T) {
	tr := NewReadTracker()

	err := checkFreshness(tr, "/unread", []byte("x"))
	if err == nil {
		t.Fatal("unread file must be rejected")
	}
	// The two messages steer the model differently ("Read it first" vs "Read it
	// again"), so they must not collapse into one.
	if !strings.Contains(err.Error(), "not been read") {
		t.Fatalf("want not-read error, got %v", err)
	}
	if strings.Contains(err.Error(), "modified since") {
		t.Fatalf("not-read error must not read as a staleness error: %v", err)
	}
}

// recordWritten refreshes the baseline so a tool that just authored content can
// immediately write again without a Read round-trip. If it stored the wrong
// hash, the agent's own second write would be rejected as "modified since".
func TestRecordWrittenRebaselinesForAnImmediateSecondWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	authored := []byte("version one")
	if err := os.WriteFile(path, authored, 0o644); err != nil {
		t.Fatal(err)
	}

	tr := NewReadTracker()
	recordWritten(tr, path, authored)

	if err := checkFreshness(tr, path, authored); err != nil {
		t.Fatalf("just-written content must pass the guard: %v", err)
	}

	rec, ok := tr.Get(path)
	if !ok {
		t.Fatal("recordWritten stored no baseline")
	}
	if rec.Size != int64(len(authored)) {
		t.Errorf("Size = %d, want %d", rec.Size, len(authored))
	}
	if rec.ModTime.IsZero() {
		t.Error("ModTime should be stat'd from disk after a write")
	}
	if rec.Partial {
		t.Error("an authored write is never a partial read")
	}
}

// recordWritten stats the path for mtime; a missing file (write failed, or the
// caller records before flush) must not panic — mtime is best-effort.
func TestRecordWrittenToleratesMissingFile(t *testing.T) {
	tr := NewReadTracker()
	missing := filepath.Join(t.TempDir(), "never-created.txt")

	recordWritten(tr, missing, []byte("content"))

	rec, ok := tr.Get(missing)
	if !ok {
		t.Fatal("baseline should still be recorded when stat fails")
	}
	if !rec.ModTime.IsZero() {
		t.Errorf("ModTime should stay zero when the file is absent, got %v", rec.ModTime)
	}
}

// Record replaces rather than merges, so re-reading a file after an external
// change adopts the new content as the baseline.
func TestRecordReplacesPreviousBaseline(t *testing.T) {
	tr := NewReadTracker()
	first := []byte("first")
	second := []byte("second")

	tr.Record("/f", ReadRecord{Size: int64(len(first)), Sum: contentSum(first)})
	tr.Record("/f", ReadRecord{Size: int64(len(second)), Sum: contentSum(second)})

	if err := checkFreshness(tr, "/f", second); err != nil {
		t.Fatalf("latest baseline should win: %v", err)
	}
	if err := checkFreshness(tr, "/f", first); err == nil {
		t.Fatal("superseded baseline must no longer validate")
	}
}

// Baselines are per-path: reading one file must not vouch for another.
func TestBaselinesAreScopedPerPath(t *testing.T) {
	tr := NewReadTracker()
	content := []byte("shared bytes")
	tr.Record("/a", ReadRecord{Size: int64(len(content)), Sum: contentSum(content)})

	if err := checkFreshness(tr, "/b", content); err == nil {
		t.Fatal("a baseline for /a must not satisfy the guard for /b, even with identical content")
	}
}

// One tracker is shared across a session, and a turn can run parallel tool
// calls (TurnStep.Batch), so Record/Get race under -race.
func TestTrackerIsSafeForConcurrentUse(t *testing.T) {
	tr := NewReadTracker()
	const workers = 16

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := filepath.Join("/p", string(rune('a'+i)))
			body := []byte(strings.Repeat("x", i+1))
			for n := 0; n < 100; n++ {
				tr.Record(path, ReadRecord{Size: int64(len(body)), Sum: contentSum(body)})
				_, _ = tr.Get(path)
				_ = checkFreshness(tr, path, body)
				recordWritten(tr, path, body)
			}
		}(i)
	}
	wg.Wait()

	for i := 0; i < workers; i++ {
		path := filepath.Join("/p", string(rune('a'+i)))
		if _, ok := tr.Get(path); !ok {
			t.Errorf("baseline for %s lost under concurrency", path)
		}
	}
}
