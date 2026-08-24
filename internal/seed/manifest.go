// Package seed implements the shipped-defaults refresh shared by every embedded
// default tree in TionHarness (skills, insight lenses, …).
//
// The problem it solves: a default file is seeded into a workspace once and then
// becomes a plain editable file. A naive "write it only if missing" seed protects
// user edits but also FREEZES every untouched copy — a shipped improvement never
// reaches an existing install. The opposite (always overwrite) destroys edits.
//
// The way out is to stop GUESSING whether the user edited a file and instead
// record what we last shipped, so "untouched" becomes a provable fact.
package seed

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// ManifestName is the sidecar, at the root of a seed dir, recording what Ensure
// last shipped for each default file. A dotfile, so the stores that scan these
// dirs (skills looks for SKILL.md, insight for *.md) never mistake it for content.
const ManifestName = ".shipped-versions.json"

// Manifest records, per default file (keyed by slash-relative path), the sha256
// of the content last shipped:
//
//   - Files: whole-file hash. Matching it on disk proves the file is a pristine
//     prior ship → safe to refresh completely.
//   - Bodies: for body-aware files, the hash of the body alone (frontmatter
//     excluded). Needed because the APP itself rewrites frontmatter in place
//     (skills: visibility markers; lenses: the enabled toggle), which changes the
//     whole-file hash. Without this ledger a single toggle would look like a user
//     edit and freeze the file forever.
//
// Legacy manifests were a flat {path: whole-hash} map; LoadManifest still reads
// those into Files.
type Manifest struct {
	Files  map[string]string `json:"files"`
	Bodies map[string]string `json:"bodies,omitempty"`
}

// LoadManifest reads the sidecar from dir. A missing or unreadable manifest
// yields empty maps — every on-disk file is then of unknown provenance and is
// preserved (see Ensure's bootstrapping note).
func LoadManifest(dir string) Manifest {
	m := Manifest{Files: map[string]string{}, Bodies: map[string]string{}}
	data, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return m
	}
	var v2 Manifest
	if err := json.Unmarshal(data, &v2); err == nil && v2.Files != nil {
		m.Files = v2.Files
		if v2.Bodies != nil {
			m.Bodies = v2.Bodies
		}
		return m
	}
	var legacy map[string]string
	if err := json.Unmarshal(data, &legacy); err == nil && legacy != nil {
		m.Files = legacy
	}
	return m
}

// SaveManifest writes the sidecar back to dir.
func SaveManifest(dir string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ManifestName), data, 0o644)
}

// SHA256Hex returns the lowercase hex sha256 of b.
func SHA256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
