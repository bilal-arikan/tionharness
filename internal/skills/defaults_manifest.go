package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// shippedManifestName is the sidecar, at the root of the seed dir, that records
// what EnsureDefaults last shipped for each default file. It is a dotfile, so
// the skill store (which scans subdirs for SKILL.md) never treats it as a skill.
const shippedManifestName = ".shipped-versions.json"

// shippedManifest records, per default file (keyed by slash-relative path), the
// sha256 hashes of the content last shipped by EnsureDefaults:
//
//   - Files: whole-file hash of the shipped content. Matching it on disk means
//     the file is a pristine prior ship → safe to fully refresh (frontmatter
//     included).
//   - Bodies: for SKILL.md files, the hash of the markdown BODY alone
//     (frontmatter excluded). The app tunes visibility frontmatter in place
//     (access/group/auto_summary/name_only/summary_only), which changes the
//     whole-file hash; without the body ledger one such tune froze the file
//     forever and shipped body updates never reached it again.
//
// Legacy manifests were a flat {path: whole-hash} map; loadShippedManifest
// still reads those into Files.
type shippedManifest struct {
	Files  map[string]string `json:"files"`
	Bodies map[string]string `json:"bodies,omitempty"`
}

// loadShippedManifest reads the shipped-version sidecar from dir. A missing or
// unreadable manifest yields empty maps (every existing on-disk file is then
// treated as unknown-provenance and preserved). A legacy flat map is folded
// into Files.
func loadShippedManifest(dir string) shippedManifest {
	m := shippedManifest{Files: map[string]string{}, Bodies: map[string]string{}}
	data, err := os.ReadFile(filepath.Join(dir, shippedManifestName))
	if err != nil {
		return m
	}
	var v2 shippedManifest
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

// saveShippedManifest writes the shipped-version sidecar back to dir.
func saveShippedManifest(dir string, m shippedManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, shippedManifestName), data, 0o644)
}

// skillBody returns the markdown body of a SKILL.md (frontmatter stripped,
// line endings normalized) — the unit the body-aware refresh hashes and
// replaces.
func skillBody(content []byte) string {
	_, body := splitFrontmatter(string(content))
	return body
}

// rebuildSkillFile joins preserved frontmatter text (delimiters excluded, as
// returned by splitFrontmatter) with a freshly shipped body in canonical form.
func rebuildSkillFile(fmText, body string) []byte {
	if fmText == "" {
		return []byte(body)
	}
	return []byte("---\n" + fmText + "\n---\n\n" + body)
}
