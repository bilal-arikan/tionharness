package skills

// The shipped-version ledger itself now lives in package seed (shared with the
// insight lens tree). What stays here is the SKILL.md-specific split it is
// parameterised with: which bytes are ours to refresh and which are the user's
// to keep.

// skillBody returns the markdown body of a SKILL.md (frontmatter stripped,
// line endings normalized) — the unit the body-aware refresh hashes and
// replaces. It MUST stay byte-stable: it is what existing .shipped-versions.json
// Bodies hashes were computed with, and changing it would make every installed
// skill look user-edited.
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
