package indexstate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrDropNotConfirmed is returned by Drop when the caller did not supply the
// user's confirmation. It is a distinct error so an HTTP handler can answer 409
// ("confirm first") instead of 500, and so a test can assert that the gate —
// rather than some unrelated failure — is what stopped the delete.
var ErrDropNotConfirmed = errors.New("indeks silme kullanıcı onayı gerektirir")

// ErrDropRootMismatch is returned when the confirmation names a different root
// than the one being dropped.
var ErrDropRootMismatch = errors.New("onay farklı bir kök dizin için verilmiş")

// DropRequest asks for an index to be deleted from disk.
//
// ConfirmRoot is the gate, and it is deliberately not a bool. A bare
// `confirm: true` is a flag any caller can set, including an agent assembling a
// tool call; requiring the caller to REPEAT the root path means the confirming
// party had to know which specific index it was destroying. The path must match
// Root under the host's path comparison rules.
type DropRequest struct {
	Tool        string
	Root        string
	ConfirmRoot string
	// IndexDir is the directory holding the store, relative to Root (e.g.
	// ".zvec-grep"). Empty means the tool keeps its store elsewhere and this
	// package cannot delete it; Drop reports that rather than guessing a path.
	IndexDir string
}

// Drop deletes an index from disk after checking the confirmation gate.
//
// There is NO unconfirmed path into this function and no automatic caller: a
// removed project directory marks its entry Missing and is Forgotten from the
// ledger, which costs nothing, instead of deleting a store the user may still
// want. Re-indexing a large repository is expensive and a wrong automatic delete
// is not recoverable, so the asymmetry is resolved toward keeping the data.
func (m *Manager) Drop(req DropRequest) error {
	root := strings.TrimSpace(req.Root)
	if root == "" || !filepath.IsAbs(root) {
		return fmt.Errorf("indeks kökü mutlak bir yol olmalı: %q", req.Root)
	}
	if strings.TrimSpace(req.ConfirmRoot) == "" {
		return ErrDropNotConfirmed
	}
	if NewKey(req.Tool, req.ConfirmRoot) != NewKey(req.Tool, root) {
		return ErrDropRootMismatch
	}
	if strings.TrimSpace(req.IndexDir) == "" {
		return fmt.Errorf("%s aracının indeks dizini bilinmiyor, silinemez", req.Tool)
	}

	// Guard against a traversal in IndexDir turning a "drop the index" request
	// into a delete of the repository — or of its parent.
	dir := filepath.Join(root, req.IndexDir)
	if rel, err := filepath.Rel(root, dir); err != nil || rel == "." || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("indeks dizini kökün dışına çıkıyor: %q", req.IndexDir)
	}

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("indeks silinemedi: %w", err)
	}
	m.Forget(req.Tool, root)
	return nil
}
