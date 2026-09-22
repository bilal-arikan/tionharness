package agent

import (
	"context"
	"path/filepath"

	"github.com/bilal-arikan/tionharness/internal/exttools"
)

// exttoolsZvecGrepName is the ledger's tool key for zvec-grep. Aliased to the
// catalog constant so the ledger, the detection code and the version probe can
// never drift onto two different spellings of the same tool.
const exttoolsZvecGrepName = exttools.ZvecGrepToolName

// codebaseMemoryToolName is the ledger's tool key for the code graph. Its store
// lives in the user's cache directory under the server's own naming, so the
// manager drives it through the server's CLI (index_status, index_repository,
// delete_project) instead of touching files — see indexmanager_cbm.go.
const codebaseMemoryToolName = "codebase-memory"

// exttoolsLocalVersion is the version probe, behind a var so tests can stub it
// without running a real binary.
var exttoolsLocalVersion = func(ctx context.Context, path string, args []string) (string, error) {
	return exttools.LocalVersion(ctx, path, args)
}

// zvecGrepIndexPath is the store directory for a root.
func zvecGrepIndexPath(root string) string {
	return filepath.Join(root, zvecGrepIndexDir)
}

// zvecGrepManifestPath is the manifest that marks a root as indexed.
func zvecGrepManifestPath(root string) string {
	return filepath.Join(root, zvecGrepIndexDir, zvecGrepManifest)
}

// isAbsPath wraps filepath.IsAbs so the manager reads as one vocabulary.
func isAbsPath(p string) bool { return filepath.IsAbs(p) }
