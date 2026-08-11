package seed

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Config describes one embedded default tree and how to refresh it.
//
// BodyAware/Body/Merge are optional and travel together: leave them nil for a
// tree whose files are all-or-nothing (only the whole-file rules apply). Supply
// all three for files split into APP-OWNED content and USER-OWNED config, so a
// shipped content update can still land under config the user (or the app) has
// changed.
type Config struct {
	FS   fs.FS  // embedded tree
	Root string // path of the tree's root inside FS (e.g. "defaults")
	Dir  string // destination directory on disk; blank makes Ensure a no-op

	// BodyAware reports whether a file (by base name) is split into config +
	// body. nil = no file is.
	BodyAware func(name string) bool
	// Body extracts the comparable body of a body-aware file. The unit hashed
	// into Manifest.Bodies — it MUST stay stable across releases, or every file
	// would look edited after an upgrade.
	Body func(content []byte) string
	// Merge builds the refreshed content for a body-aware file whose body is a
	// pristine prior ship but whose config differs from the shipped one. It owns
	// the whole policy of what carries over: skills keep the on-disk frontmatter
	// verbatim, lenses keep only the user's own keys and adopt the rest.
	Merge func(onDisk, embedded []byte) []byte
}

func (c Config) bodyAware(name string) bool {
	return c.BodyAware != nil && c.Body != nil && c.Merge != nil && c.BodyAware(name)
}

// Ensure writes an embedded default tree into cfg.Dir, version-aware and (for
// body-aware files) config-aware:
//
//   - A missing file is written and its shipped hashes recorded.
//   - A file identical to the embedded one is left alone; its hashes are recorded
//     so a FUTURE ship knows it was pristine.
//   - A file whose WHOLE content matches the previously shipped hash was never
//     touched → fully refreshed, config included. This is the path that carries
//     shipped updates into existing installs.
//   - A body-aware file matching no whole hash but whose BODY matches the embedded
//     or previously shipped body only had its CONFIG changed → cfg.Merge decides
//     what the refreshed file looks like.
//   - Anything else is a real user edit and is preserved untouched.
//
// Bootstrapping (no manifest yet, or a legacy flat one): nothing is overwritten;
// hashes self-seed for every file whose content (or body) already matches the
// embedded tree, so subsequent ships can refresh them. Note the consequence — a
// file shipped BEFORE the manifest existed and since superseded looks
// indistinguishable from a user edit and stays frozen. Restoring one is a
// deliberate user action (see the per-file reset the callers expose), not
// something this function may guess at.
func Ensure(cfg Config) error {
	if cfg.Dir == "" {
		return nil
	}
	manifest := LoadManifest(cfg.Dir)
	changed := false

	walkErr := fs.WalkDir(cfg.FS, cfg.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(cfg.Root, p)
		if relErr != nil {
			return relErr
		}
		key := filepath.ToSlash(rel) // stable manifest key across OSes
		dest := filepath.Join(cfg.Dir, filepath.FromSlash(rel))

		embedded, readErr := fs.ReadFile(cfg.FS, p)
		if readErr != nil {
			return readErr
		}
		hEmbed := SHA256Hex(embedded)
		aware := cfg.bodyAware(d.Name())
		var hEmbedBody string
		if aware {
			hEmbedBody = SHA256Hex([]byte(cfg.Body(embedded)))
		}
		recordShipped := func() {
			if manifest.Files[key] != hEmbed {
				manifest.Files[key] = hEmbed
				changed = true
			}
			if aware && manifest.Bodies[key] != hEmbedBody {
				manifest.Bodies[key] = hEmbedBody
				changed = true
			}
		}

		onDisk, statErr := os.ReadFile(dest)
		if statErr != nil {
			// Missing (or unreadable) — write it fresh and record the shipped hashes.
			if mkErr := os.MkdirAll(filepath.Dir(dest), 0o755); mkErr != nil {
				return mkErr
			}
			if wErr := os.WriteFile(dest, embedded, 0o644); wErr != nil {
				return wErr
			}
			recordShipped()
			return nil
		}

		hDisk := SHA256Hex(onDisk)
		if hDisk == hEmbed {
			recordShipped() // already current; make sure it is recorded as pristine
			return nil
		}

		// Untouched previously-shipped WHOLE file → full refresh. Stays first: it
		// is the only path allowed to replace config, and a whole-file match proves
		// nobody (user or app) changed the config either.
		if prev, ok := manifest.Files[key]; ok && prev == hDisk {
			if wErr := os.WriteFile(dest, embedded, 0o644); wErr != nil {
				return wErr
			}
			recordShipped()
			return nil
		}

		if !aware {
			return nil // user edit — preserve
		}

		// Config-aware path: compare bodies alone, so a shipped body update still
		// lands under config that was legitimately changed.
		hDiskBody := SHA256Hex([]byte(cfg.Body(onDisk)))
		if hDiskBody == hEmbedBody {
			// Body already current, only the config differs. Track the body as
			// pristine so the NEXT shipped body update can refresh it.
			if manifest.Bodies[key] != hEmbedBody {
				manifest.Bodies[key] = hEmbedBody
				changed = true
			}
			return nil
		}
		if prev, ok := manifest.Bodies[key]; ok && prev == hDiskBody {
			if wErr := os.WriteFile(dest, cfg.Merge(onDisk, embedded), 0o644); wErr != nil {
				return wErr
			}
			manifest.Bodies[key] = hEmbedBody
			// The merged file is NOT the embedded bytes, so its whole-file hash is
			// unknown-by-construction: drop any stale Files entry rather than record
			// a hash that would later mis-classify the merge result as a prior ship.
			if _, ok := manifest.Files[key]; ok {
				delete(manifest.Files, key)
			}
			changed = true
			return nil
		}
		return nil // body edited by the user → preserve
	})
	if walkErr != nil {
		return walkErr
	}
	if changed {
		return SaveManifest(cfg.Dir, manifest)
	}
	return nil
}

// Restore overwrites one file in cfg.Dir with its embedded default and records it
// as pristine, discarding whatever was there. This is the deliberate escape hatch
// for the case Ensure must not touch: a file of unknown provenance (edited, or
// shipped before the manifest existed) that the user wants back at the default.
// rel is the slash-relative path inside the tree (e.g. "context-cache-opt.md").
// A path with no embedded counterpart is an error — callers surface it as 404.
func Restore(cfg Config, rel string) error {
	if cfg.Dir == "" {
		return os.ErrInvalid
	}
	embedded, err := fs.ReadFile(cfg.FS, cfg.Root+"/"+rel)
	if err != nil {
		return err
	}
	dest := filepath.Join(cfg.Dir, filepath.FromSlash(rel))
	if mkErr := os.MkdirAll(filepath.Dir(dest), 0o755); mkErr != nil {
		return mkErr
	}
	if wErr := os.WriteFile(dest, embedded, 0o644); wErr != nil {
		return wErr
	}
	// Record it, so the file is immediately a known pristine ship and the next
	// shipped update refreshes it without the user resetting again.
	m := LoadManifest(cfg.Dir)
	m.Files[rel] = SHA256Hex(embedded)
	if cfg.bodyAware(filepath.Base(rel)) {
		m.Bodies[rel] = SHA256Hex([]byte(cfg.Body(embedded)))
	}
	return SaveManifest(cfg.Dir, m)
}

// HasDefault reports whether rel names a file in the embedded tree — so a UI can
// offer "restore default" only where there is one.
func HasDefault(cfg Config, rel string) bool {
	f, err := cfg.FS.Open(cfg.Root + "/" + rel)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// State is how one on-disk file relates to its shipped default — the ledger made
// legible. Its point is to answer the question the refresh rules otherwise hide:
// WILL this file keep receiving shipped improvements, or has it opted out?
type State string

const (
	// StateNone: no shipped default (a user-authored file, or one not in the tree).
	StateNone State = ""
	// StateDefault: byte-identical to the shipped file. Refreshes automatically.
	StateDefault State = "default"
	// StateTuned: the config differs but the body is the shipped one — the normal
	// result of a UI toggle. STILL refreshes automatically (that is what the Bodies
	// ledger buys), so it is informational, not a warning.
	StateTuned State = "tuned"
	// StateEdited: the body (or an opaque file's content) was changed. This file is
	// deliberately FROZEN — shipped updates will no longer reach it until someone
	// restores the default. The one state a UI should actually draw attention to.
	StateEdited State = "edited"
)

// Status classifies one file against its shipped default. Computed live from the
// two contents rather than read from the manifest: the manifest records what we
// last SHIPPED, which answers "may we overwrite this?", while a UI needs "how does
// this differ from the default right now?" — related questions with different
// answers (a file refreshed a moment ago is pristine either way, but one edited
// since the last ship is only distinguishable by comparing content).
func Status(cfg Config, rel string) State {
	embedded, err := fs.ReadFile(cfg.FS, cfg.Root+"/"+rel)
	if err != nil {
		return StateNone
	}
	onDisk, err := os.ReadFile(filepath.Join(cfg.Dir, filepath.FromSlash(rel)))
	if err != nil {
		return StateNone
	}
	if SHA256Hex(onDisk) == SHA256Hex(embedded) {
		return StateDefault
	}
	if cfg.bodyAware(filepath.Base(rel)) && cfg.Body(onDisk) == cfg.Body(embedded) {
		return StateTuned
	}
	return StateEdited
}
