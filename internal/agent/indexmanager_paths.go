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

// codebaseMemoryToolName is the key the status report uses for the code graph.
// It is NOT a ledger key: codebase-memory keeps its store in the user's cache
// directory under its own naming and is not driven through indexstate yet, so
// this name only ever labels an observational status row.
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
