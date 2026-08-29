package insight

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Version is a content hash of everything about a lens that can change what a
// scan PRODUCES: the analysis prompt, the prefilter predicate, the transcript
// scope and the analysis model. It is the second re-scan trigger next to the
// session fingerprint (_Docs/60 §3): improve a lens and every already-scanned
// session becomes due again, even though its content never changed.
//
// Deliberately EXCLUDED: ID, Name, Description, Enabled, Path, DefaultState.
// Renaming a lens or toggling it off and on again is not a semantic change, and
// hashing those fields would force a full (paid) re-scan for cosmetic edits.
func (l Lens) Version() string {
	var b strings.Builder
	b.WriteString("prompt\x00")
	b.WriteString(strings.TrimSpace(l.Prompt))
	b.WriteString("\x00model\x00")
	b.WriteString(l.Model)
	b.WriteString("\x00scope\x00")
	// Scope is a set: sort a copy so a reordered frontmatter list is not a change.
	scope := append([]string(nil), l.Scope...)
	sort.Strings(scope)
	b.WriteString(strings.Join(scope, ","))
	b.WriteString("\x00prefilter\x00")
	b.WriteString(l.Prefilter.versionKey())
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:8]) // 16 hex chars: collision risk is negligible for a per-lens tag
}

// versionKey renders the prefilter deterministically for hashing: every slice is
// sorted and the map is emitted in key order, so semantically identical
// frontmatter written in a different order hashes the same.
func (p Prefilter) versionKey() string {
	sorted := func(in []string) string {
		out := append([]string(nil), in...)
		sort.Strings(out)
		return strings.Join(out, ",")
	}
	keys := make([]string, 0, len(p.MinCount))
	for k := range p.MinCount {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var mc strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&mc, "%s=%d;", k, p.MinCount[k])
	}
	return strings.Join([]string{
		sorted(p.RequiresAny),
		sorted(p.RequiresAll),
		sorted(p.Excludes),
		mc.String(),
		fmt.Sprint(p.MinTokens),
	}, "\x00")
}
