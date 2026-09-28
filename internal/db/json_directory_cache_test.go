package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUsageCachePreservesExternalChangesAndDeletions(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "usage")
	cache := filepath.Join(root, "cache.json")
	os.MkdirAll(dir, 0700)
	path := filepath.Join(dir, "S1.json")
	if err := atomicWriteJSON(path, SessionUsage{SessionID: "S1", Calls: 2}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		rows, err := loadCachedJSONDir[SessionUsage](dir, cache)
		if err != nil || len(rows) != 1 || rows[0].Calls != 2 {
			t.Fatalf("initial: %+v %v", rows, err)
		}
	}
	if err := atomicWriteJSON(path, SessionUsage{SessionID: "S1", Calls: 100}); err != nil {
		t.Fatal(err)
	}
	rows, err := loadCachedJSONDir[SessionUsage](dir, cache)
	if err != nil || len(rows) != 1 || rows[0].Calls != 100 {
		t.Fatalf("external update lost: %+v %v", rows, err)
	}
	if err := os.WriteFile(cache, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	rows, err = loadCachedJSONDir[SessionUsage](dir, cache)
	if err != nil || len(rows) != 1 || rows[0].Calls != 100 {
		t.Fatalf("cache fallback lost usage: %+v %v", rows, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	rows, err = loadCachedJSONDir[SessionUsage](dir, cache)
	if err != nil || len(rows) != 0 {
		t.Fatalf("deleted row resurrected: %+v %v", rows, err)
	}
}
