package insight

import (
	"os"
	"path/filepath"
	"strings"
)

// CheckFilePointer reports whether an app-fix finding's suggested FilePointer
// (a repo-relative path the analyzer proposed, e.g. "internal/agent/toolsetup.go")
// actually resolves to a file under repoDir. The pointer is an LLM guess, so a
// caller uses this to flag unverified/hallucinated paths. A blank repoDir or
// pointer returns false (unknown → treat as unverified).
func CheckFilePointer(repoDir, pointer string) bool {
	repoDir = strings.TrimSpace(repoDir)
	pointer = strings.TrimSpace(pointer)
	if repoDir == "" || pointer == "" {
		return false
	}
	// Only accept a plain repo-relative path; reject absolute or parent-escaping
	// pointers so this check never stats outside the repo.
	if filepath.IsAbs(pointer) || strings.Contains(pointer, "..") {
		return false
	}
	info, err := os.Stat(filepath.Join(repoDir, filepath.FromSlash(pointer)))
	return err == nil && !info.IsDir()
}

// Deduplication has two layers. This file is the CONSERVATIVE ingest layer:
// canonSig collapses formatting-only differences in a signature (case,
// punctuation, whitespace) so `tool:get_flow` and `Tool: get_flow ` merge into
// one finding at write time. It deliberately does NOT merge semantically-similar
// but differently-worded signatures — that would risk fusing distinct issues.
// The aggressive, differently-worded near-duplicates are handled non-destructively
// at display time by the clustering layer (cluster.go).

// canonSig canonicalizes a signature for formatting-insensitive matching:
// lowercased, every run of non-alphanumeric characters collapsed to a single
// space, trimmed. Two signatures with the same canonical form denote the same
// finding shape and are merged on ingest.
func canonSig(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevSpace = false
			continue
		}
		if !prevSpace {
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}
