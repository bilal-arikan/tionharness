package indexstate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// seedIndex creates a store directory with a file in it and registers it in the
// ledger, returning the root.
func seedIndex(t *testing.T, m *Manager) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".zvec-grep")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.Observe(testTool, root, PhaseReady, "local/m", "1.0.0")
	return root
}

func TestDropRefusesWithoutConfirmation(t *testing.T) {
	m := New()
	root := seedIndex(t, m)

	err := m.Drop(DropRequest{Tool: testTool, Root: root, IndexDir: ".zvec-grep"})
	if !errors.Is(err, ErrDropNotConfirmed) {
		t.Fatalf("err=%v, want ErrDropNotConfirmed", err)
	}
	// The gate must not merely report an error — nothing may be deleted.
	if _, statErr := os.Stat(filepath.Join(root, ".zvec-grep", "manifest.json")); statErr != nil {
		t.Fatal("an unconfirmed drop deleted the index anyway")
	}
	if _, seen := m.Get(testTool, root); !seen {
		t.Error("an unconfirmed drop removed the ledger entry")
	}
}

func TestDropRefusesAConfirmationForADifferentRoot(t *testing.T) {
	m := New()
	root := seedIndex(t, m)
	other := t.TempDir()

	err := m.Drop(DropRequest{Tool: testTool, Root: root, ConfirmRoot: other, IndexDir: ".zvec-grep"})
	if !errors.Is(err, ErrDropRootMismatch) {
		t.Fatalf("err=%v, want ErrDropRootMismatch", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".zvec-grep")); statErr != nil {
		t.Fatal("a mismatched confirmation deleted the index")
	}
}

func TestDropDeletesOnlyWithAMatchingConfirmation(t *testing.T) {
	m := New()
	root := seedIndex(t, m)

	if err := m.Drop(DropRequest{Tool: testTool, Root: root, ConfirmRoot: root, IndexDir: ".zvec-grep"}); err != nil {
		t.Fatalf("confirmed drop failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".zvec-grep")); !os.IsNotExist(err) {
		t.Fatal("the index survived a confirmed drop")
	}
	if _, seen := m.Get(testTool, root); seen {
		t.Error("the ledger entry survived a confirmed drop")
	}
	// The repository itself must be untouched.
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("the drop removed the repository root: %v", err)
	}
}

func TestDropRefusesToEscapeTheRoot(t *testing.T) {
	m := New()
	root := seedIndex(t, m)
	// A traversal in IndexDir must not turn "drop the index" into a delete of
	// the repository or its parent.
	for _, dir := range []string{"..", filepath.Join("..", "sibling"), ".", ""} {
		err := m.Drop(DropRequest{Tool: testTool, Root: root, ConfirmRoot: root, IndexDir: dir})
		if err == nil {
			t.Fatalf("IndexDir %q was accepted", dir)
		}
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("a rejected drop still damaged the root: %v", err)
	}
}

func TestDropRequiresAnAbsoluteRoot(t *testing.T) {
	m := New()
	if err := m.Drop(DropRequest{Tool: testTool, Root: "relative/dir", ConfirmRoot: "relative/dir", IndexDir: ".zvec-grep"}); err == nil {
		t.Fatal("a relative root was accepted")
	}
}

func TestDropRefusesAnUnknownIndexDir(t *testing.T) {
	m := New()
	root := seedIndex(t, m)
	err := m.Drop(DropRequest{Tool: testTool, Root: root, ConfirmRoot: root})
	if err == nil {
		t.Fatal("a drop with no known index directory was accepted")
	}
	if _, statErr := os.Stat(filepath.Join(root, ".zvec-grep")); statErr != nil {
		t.Error("the index was deleted despite an unknown index directory")
	}
}
