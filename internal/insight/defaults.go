package insight

import (
	"embed"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/seed"
	"github.com/bilal-arikan/tionswarm/internal/skills"
)

// defaultsFS holds the built-in lenses shipped with TionSwarm. They are seeded
// into a workspace's lens dir on startup so every workspace inherits the baseline
// scan intents — and, via the shared shipped-hash ledger, keeps inheriting later
// improvements to them.
//
//go:embed defaults
var defaultsFS embed.FS

// LensesDirRel is the store-root-relative directory that holds the workspace's
// editable lens files.
var LensesDirRel = filepath.Join("insight", "lenses")

// LensesDir returns the absolute lens directory under a store root (db.Root()).
func LensesDir(root string) string { return filepath.Join(root, LensesDirRel) }

// userLensKeys are the frontmatter keys a lens file's OWNER controls. Everything
// else — prefilter, scope, channel, name, description and the whole analysis body
// — is ours: it encodes which sessions the lens must look at and what it should
// look for, and a shipped correction there (a mis-scoped prefilter, a missing
// slice surface) has to reach existing workspaces or the fix is theoretical.
//
// `enabled` is the toggle the UI writes through SetFrontmatterEnabled; `model`
// is the user's analysis-model choice. Both are decisions about THIS install.
var userLensKeys = []string{"enabled", "model"}

// lensBody returns the analysis-instruction body of a lens file — the unit the
// body-aware refresh hashes. Must stay byte-stable across releases (see
// seed.Config.Body).
func lensBody(content []byte) string {
	return skills.FrontmatterBody(string(content))
}

// mergeLens rebuilds a lens whose body is a pristine prior ship but whose
// frontmatter differs from the shipped one — in practice, a lens the user
// enabled/disabled or pointed at another model.
//
// Policy (deliberately NOT the skills one, which preserves all frontmatter): take
// the shipped file whole, then carry the user's own keys back over. A lens's
// frontmatter is mostly MECHANICS, not preference, so preserving it wholesale
// would permanently freeze prefilter/scope fixes — the exact failure that made
// the cache lens run against a slice holding no cache evidence.
func mergeLens(onDisk, embedded []byte) []byte {
	out := embedded
	for _, key := range userLensKeys {
		if v := strings.TrimSpace(skills.FrontmatterField(string(onDisk), key)); v != "" {
			out = SetFrontmatterScalar(out, key, v)
		}
	}
	return out
}

// seedConfig describes the shipped lens tree to the shared seeder.
func seedConfig(dir string) seed.Config {
	return seed.Config{
		FS:        defaultsFS,
		Root:      "defaults",
		Dir:       dir,
		BodyAware: func(name string) bool { return strings.EqualFold(filepath.Ext(name), ".md") },
		Body:      lensBody,
		Merge:     mergeLens,
	}
}

// EnsureDefaults writes any missing built-in lens into dir and refreshes the ones
// the user has not edited (see package seed for the shipped-hash rules). A blank
// dir is a no-op.
func EnsureDefaults(dir string) error {
	return seed.Ensure(seedConfig(dir))
}

// RestoreDefault overwrites one lens with its shipped default, discarding local
// changes, and records it as pristine so future ships refresh it automatically.
// This is the escape hatch for a lens the ledger cannot vouch for — notably one
// seeded BEFORE the ledger existed, which is indistinguishable from a user edit
// and would otherwise stay frozen forever. Returns an error when the id names no
// shipped lens (callers surface it as 404).
func RestoreDefault(dir, id string) error {
	name := lensFileName(id)
	if name == "" {
		return fs.ErrNotExist
	}
	return seed.Restore(seedConfig(dir), name)
}

// HasDefault reports whether a lens id has a shipped default — so the UI offers
// "restore default" only where restoring means something.
func HasDefault(id string) bool {
	name := lensFileName(id)
	// A blank name would open the defaults DIRECTORY, which succeeds — guard it,
	// or every unshipped lens would advertise a default it does not have.
	return name != "" && seed.HasDefault(seedConfig(""), name)
}

// DefaultState classifies a lens file against its shipped default (see seed.State).
// seed.StateNone for a user-authored lens.
func DefaultState(dir, id string) seed.State {
	name := lensFileName(id)
	if name == "" {
		return seed.StateNone
	}
	return seed.Status(seedConfig(dir), name)
}

// lensFileName maps a lens id to its file name in the defaults tree. Shipped
// lenses are named after their id; a path separator would escape the tree, so it
// disqualifies the id outright rather than being sanitised into a different file.
func lensFileName(id string) string {
	if id == "" || strings.ContainsAny(id, `/\`) {
		return ""
	}
	return id + ".md"
}
