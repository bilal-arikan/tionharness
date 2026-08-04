package view

// CapLines keeps as many leading lines as fit in maxBytes and reports how many
// it dropped.
//
// It exists because the obvious alternative — building the whole string and
// slicing it to a byte budget — has two failure modes that matter when the
// result is fed to a model:
//
//   - It cuts mid-line, so the last entry arrives truncated into something that
//     still LOOKS like a complete record. A half-written error message is worse
//     than no error message, because a reader cannot tell it was cut.
//   - It loses the count. "…(truncated)" says something was dropped but not how
//     much, so nobody can tell whether they are looking at most of the evidence
//     or a tenth of it.
//
// Callers render the dropped count themselves (wording and language vary by
// audience — see View.ElidedNote), so this returns the number rather than a
// sentence.
//
// A maxBytes <= 0 means "no budget": everything is dropped, which is the honest
// answer for a caller that has no room at all. Byte length is used (not runes)
// because the budget it serves is a prompt-size budget.
func CapLines(lines []string, maxBytes int) (kept []string, dropped int) {
	if maxBytes <= 0 {
		return nil, len(lines)
	}
	used := 0
	for i, ln := range lines {
		// +1 for the newline the caller will join with.
		next := used + len(ln) + 1
		if next > maxBytes {
			return lines[:i], len(lines) - i
		}
		used = next
	}
	return lines, 0
}
