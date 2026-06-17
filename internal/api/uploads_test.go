package api

import (
	"path/filepath"
	"testing"
)

func TestSafePathSegment(t *testing.T) {
	ok := []string{"a7ada60e-8200-4cad", "abc123", "session_1"}
	bad := []string{"", ".", "..", "../etc", "a/b", "a\\b", "..\\x", "x/..", "a..b/../c"}
	for _, s := range ok {
		if !safePathSegment(s) {
			t.Errorf("safePathSegment(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if safePathSegment(s) {
			t.Errorf("safePathSegment(%q) = true, want false (traversal-unsafe)", s)
		}
	}
}

func TestWithinDir(t *testing.T) {
	root := filepath.Join("data", "workspace", "uploads")
	inside := filepath.Join(root, "sid", "file.txt")
	if !withinDir(root, inside) {
		t.Errorf("withinDir: expected %q inside %q", inside, root)
	}
	// A traversal that climbs out of the uploads root must be rejected.
	escape := filepath.Join(root, "..", "..", "secret.json")
	if withinDir(root, escape) {
		t.Errorf("withinDir: expected %q to be OUTSIDE %q", escape, root)
	}
}
