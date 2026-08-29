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

// topicMinTokens is the smallest canonical topic allowed to match two findings.
// A one-token topic ("timeout", "low") is too generic to prove two signatures
// describe the same root cause, so it never matches — unless that single token
// is long enough to be an identifier in its own right (topicMinSoloLen), which
// covers the analyzer's hash fallback signatures.
const (
	topicMinTokens  = 2
	topicMinSoloLen = 8
)

// canonTopic reduces a signature to the subject it names — the tool/component
// plus the error shape — so the SAME root cause reported through different
// lenses collapses onto one finding instead of one card per lens. The lens's own
// id prefix is dropped (analyzers slug signatures as "<lens>:<shape>"), the rest
// is lowercased, split on every non-alphanumeric run (so "get_flow", "get-flow"
// and "get flow" agree) and stripped of volatile tokens (bare numbers, ids like
// "SES2047"). Returns "" when what survives is too thin to match on.
func canonTopic(lensID, sig string) string {
	s := strings.TrimSpace(sig)
	if lensID != "" && len(s) > len(lensID)+1 &&
		strings.EqualFold(s[:len(lensID)], lensID) && s[len(lensID)] == ':' {
		s = s[len(lensID)+1:]
	}
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	out := make([]string, 0, len(fields))
	for _, tok := range fields {
		if volatileToken(tok) {
			continue
		}
		out = append(out, tok)
	}
	if len(out) == 1 && len(out[0]) >= topicMinSoloLen {
		return out[0]
	}
	if len(out) < topicMinTokens {
		return ""
	}
	return strings.Join(out, " ")
}

// volatileToken reports whether a signature token identifies one concrete run
// rather than the problem shape: a bare number, or a short-prefix id such as
// "ses2047", "tsk67" or "ws5".
func volatileToken(tok string) bool {
	letters := 0
	for _, r := range tok {
		if r < '0' || r > '9' {
			letters++
			continue
		}
		break
	}
	if letters == len(tok) {
		return false // no trailing digits at all
	}
	if letters > 5 {
		return false // too much text to be an id token
	}
	for i, r := range tok {
		if i < letters {
			if r < 'a' || r > 'z' {
				return false
			}
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
