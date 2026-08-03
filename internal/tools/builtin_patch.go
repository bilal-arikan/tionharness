package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// FSApplyPatchTool applies a unified diff (possibly spanning several files and
// multiple hunks each) atomically per file. It is the multi-hunk sibling of Edit:
// where Edit does one exact string replacement, apply_patch consumes a standard
// `diff -u` / `git diff` patch. Hunks are located by CONTEXT MATCHING (the @@ line
// numbers are treated as hints, not trusted), so a patch still applies after
// unrelated edits shifted the lines — but if a hunk's context cannot be found the
// whole file is rejected rather than applied wrong. The freshness guard applies to
// every modified file exactly as it does for Edit/Write.
type FSApplyPatchTool struct {
	sb      Sandbox
	tracker *ReadTracker
}

// NewFSApplyPatchTool binds the tool to a workspace sandbox. tracker (may be nil)
// enforces read-before-write / not-modified-since-read per patched file.
func NewFSApplyPatchTool(sb Sandbox, tracker *ReadTracker) FSApplyPatchTool {
	return FSApplyPatchTool{sb: sb, tracker: tracker}
}

func (FSApplyPatchTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "apply_patch",
		Description: "Apply a unified diff (the output of `diff -u` or `git diff`) to one or more files in a " +
			"single call — the multi-hunk alternative to Edit. Each file section starts with `--- a/<path>` and " +
			"`+++ b/<path>` header lines, followed by one or more `@@ ... @@` hunks of context (space-prefixed), " +
			"removed (`-`) and added (`+`) lines. Hunks are matched by their context, so exact @@ line numbers are " +
			"not required; if the exact context is not found, a whitespace-insensitive match is tried and accepted only " +
			"when it is UNIQUE, otherwise that whole file is rejected (nothing is half-applied) with an error pointing at " +
			"the closest line. Copy context/removed lines verbatim (keep unicode and alignment). Read a file before patching it (same freshness guard as Edit). Use " +
			"`--- /dev/null` to create a file and `+++ /dev/null` to delete one. Paths may be absolute or relative " +
			"to the working directory; a leading a/ or b/ is stripped.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"patch":{"type":"string","description":"The unified diff text to apply"}},
			"required":["patch"],
			"additionalProperties":false
		}`),
	}
}

func (t FSApplyPatchTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Patch string `json:"patch"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	files, err := parseUnifiedDiff(args.Patch)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no file sections found in patch — expected `--- ` / `+++ ` headers")
	}

	// Two-phase so the patch is all-or-nothing across files: first resolve every
	// file's new content (validating freshness + hunk application) WITHOUT writing,
	// then commit. A failure in any file aborts before a single write.
	type pending struct {
		abs     string
		rel     string
		content string // new content ("" for a delete)
		old     string
		created bool
		deleted bool
	}
	var commits []pending
	for _, f := range files {
		p := pending{deleted: f.delete, created: f.create}
		// Deletion path.
		if f.delete {
			abs, err := t.sb.Resolve(f.oldPath)
			if err != nil {
				return "", err
			}
			data, rerr := os.ReadFile(abs)
			if rerr != nil {
				return "", fmt.Errorf("cannot delete %s: %w", f.oldPath, rerr)
			}
			if err := checkFreshness(t.tracker, abs, data); err != nil {
				return "", fmt.Errorf("%s: %w", t.sb.Rel(abs), err)
			}
			p.abs, p.rel, p.old = abs, t.sb.Rel(abs), string(data)
			commits = append(commits, p)
			continue
		}
		// Create or update: resolve the destination path (the +++ side).
		abs, err := t.sb.Resolve(f.newPath)
		if err != nil {
			return "", err
		}
		var old string
		if !f.create {
			data, rerr := os.ReadFile(abs)
			if rerr != nil {
				return "", fmt.Errorf("cannot patch %s: %w (use `--- /dev/null` to create a new file)", f.newPath, rerr)
			}
			if err := checkFreshness(t.tracker, abs, data); err != nil {
				return "", fmt.Errorf("%s: %w", t.sb.Rel(abs), err)
			}
			old = string(data)
		}
		newContent, err := applyHunks(old, f.hunks, f.create)
		if err != nil {
			return "", fmt.Errorf("%s: %w", f.newPath, err)
		}
		p.abs, p.rel, p.old, p.content = abs, t.sb.Rel(abs), old, newContent
		commits = append(commits, p)
	}

	// Commit phase: write/delete, refresh the freshness baseline and record diffs.
	var b strings.Builder
	for _, c := range commits {
		switch {
		case c.deleted:
			if err := os.Remove(c.abs); err != nil {
				return "", fmt.Errorf("delete %s failed after validation: %w", c.rel, err)
			}
			_, removed, patch := lineDiff(c.old, "")
			recordDiff(ctx, FileDiff{Path: c.rel, Removed: removed, Patch: patch})
			fmt.Fprintf(&b, "deleted %s\n", c.rel)
		default:
			if err := os.MkdirAll(filepath.Dir(c.abs), 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(c.abs, []byte(c.content), 0o644); err != nil {
				return "", fmt.Errorf("write %s failed after validation: %w", c.rel, err)
			}
			// Mutation verifier (self-healing): each patched file must actually
			// be on disk as computed before the batch reports success.
			if err := verifyMutationLanded(c.abs, []byte(c.content)); err != nil {
				return "", err
			}
			recordWritten(t.tracker, c.abs, []byte(c.content))
			added, removed, patch := lineDiff(c.old, c.content)
			recordDiff(ctx, FileDiff{Path: c.rel, Added: added, Removed: removed, Patch: patch, Created: c.created})
			verb := "patched"
			if c.created {
				verb = "created"
			}
			fmt.Fprintf(&b, "%s %s (+%d/-%d)\n", verb, c.rel, added, removed)
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// diffFile is one file's worth of a parsed unified diff.
type diffFile struct {
	oldPath string
	newPath string
	create  bool // --- /dev/null
	delete  bool // +++ /dev/null
	hunks   []diffHunk
}

// diffHunk is one @@ block: its context/removed lines form the pre-image to locate
// in the file, its context/added lines the post-image to substitute.
type diffHunk struct {
	pre  []string // lines that must be present (context + removed), in order
	post []string // replacement lines (context + added), in order
}

// parseUnifiedDiff splits a unified diff into per-file hunk sets. It tolerates a
// `diff --git` preamble and `index`/mode lines by ignoring anything that is not a
// ---/+++ header or a hunk body while between files.
func parseUnifiedDiff(patch string) ([]diffFile, error) {
	lines := strings.Split(patch, "\n")
	var files []diffFile
	var cur *diffFile
	var hunk *diffHunk
	flushHunk := func() {
		if cur != nil && hunk != nil {
			cur.hunks = append(cur.hunks, *hunk)
			hunk = nil
		}
	}
	flushFile := func() {
		flushHunk()
		if cur != nil {
			files = append(files, *cur)
			cur = nil
		}
	}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "--- "):
			flushFile()
			old := strings.TrimSpace(strings.TrimPrefix(line, "--- "))
			// The paired +++ header must follow.
			if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "+++ ") {
				return nil, fmt.Errorf("malformed patch: `--- ` header not followed by `+++ ` (line %d)", i+1)
			}
			i++
			newp := strings.TrimSpace(strings.TrimPrefix(lines[i], "+++ "))
			cur = &diffFile{
				oldPath: stripDiffPathPrefix(old),
				newPath: stripDiffPathPrefix(newp),
				create:  isDevNull(old),
				delete:  isDevNull(newp),
			}
		case strings.HasPrefix(line, "@@"):
			if cur == nil {
				return nil, fmt.Errorf("malformed patch: hunk `@@` before any `--- ` file header (line %d)", i+1)
			}
			flushHunk()
			hunk = &diffHunk{}
		case cur != nil && hunk != nil && len(line) > 0 && (line[0] == ' ' || line[0] == '+' || line[0] == '-'):
			body := line[1:]
			switch line[0] {
			case ' ':
				hunk.pre = append(hunk.pre, body)
				hunk.post = append(hunk.post, body)
			case '-':
				hunk.pre = append(hunk.pre, body)
			case '+':
				hunk.post = append(hunk.post, body)
			}
		case line == `\ No newline at end of file`:
			// Informational marker — ignore.
		default:
			// Between-file noise (diff --git, index, blank lines) or an empty context
			// line inside a hunk. A truly empty line inside a hunk is a space-prefixed
			// blank ("" after stripping) and is handled above; here we just skip.
		}
	}
	flushFile()
	return files, nil
}

// stripDiffPathPrefix removes a leading a/ or b/ that git prepends to diff paths.
func stripDiffPathPrefix(p string) string {
	if isDevNull(p) {
		return p
	}
	// Drop a trailing tab-separated timestamp (`diff -u` adds one).
	if tab := strings.IndexByte(p, '\t'); tab >= 0 {
		p = p[:tab]
	}
	if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
		return p[2:]
	}
	return p
}

func isDevNull(p string) bool {
	p = strings.TrimSpace(p)
	return p == "/dev/null" || p == "dev/null"
}

// applyHunks produces the new file content by locating each hunk's pre-image in
// old (searching forward from the previous hunk) and substituting its post-image.
// For a newly created file (create), old is empty and the hunks' post-images are
// concatenated directly. A hunk whose pre-image is not found is a hard error — the
// file is never partially rewritten.
func applyHunks(old string, hunks []diffHunk, create bool) (string, error) {
	if create {
		var out []string
		for _, h := range hunks {
			out = append(out, h.post...)
		}
		return joinLines(out), nil
	}
	oldLines := splitLines(old)
	// A CRLF file (Windows) leaves a trailing '\r' on each split line, but the
	// patch's context/removed lines come from the model as bare LF — so the hunk
	// pre-images would never match. Strip the '\r' for matching, then restore CRLF
	// on write so the file keeps its own line ending.
	crlf := strings.Contains(old, "\r\n")
	if crlf {
		for i := range oldLines {
			oldLines[i] = strings.TrimSuffix(oldLines[i], "\r")
		}
	}
	var result []string
	cursor := 0 // index into oldLines already copied to result
	for hi, h := range hunks {
		idx := indexOfBlock(oldLines, h.pre, cursor)
		if idx < 0 {
			// Verbatim context failed. The hunk is often the RIGHT region but its
			// context/removed lines differ from disk only in trailing whitespace or
			// indentation (the model rewrote them from memory). Retry with widening
			// per-line tolerance — but ONLY accept a UNIQUE location from the cursor
			// onward, so a fuzzy match can never rewrite the wrong block.
			for _, norm := range []func(string) string{rtrimLine, strings.TrimSpace} {
				hits := blockMatchesNorm(oldLines, h.pre, cursor, norm)
				if len(hits) == 1 {
					idx = hits[0]
					break
				}
				if len(hits) > 1 {
					return "", fmt.Errorf("hunk %d has no exact context match and a whitespace-insensitive match is ambiguous (%d candidates from line %d) — regenerate the patch with exact, unique context", hi+1, len(hits), cursor+1)
				}
			}
		}
		if idx < 0 {
			return "", diagnoseHunkMismatch(oldLines, h.pre, cursor, hi+1)
		}
		result = append(result, oldLines[cursor:idx]...) // unchanged run before the hunk
		result = append(result, h.post...)               // the substitution
		cursor = idx + len(h.pre)
	}
	result = append(result, oldLines[cursor:]...) // tail after the last hunk
	out := joinLines(result)
	if crlf {
		out = toCRLF(out)
	}
	return out, nil
}

// indexOfBlock returns the first index >= from at which block occurs contiguously
// in lines, or -1. An empty block matches at `from` (a pure-insertion hunk).
func indexOfBlock(lines, block []string, from int) int {
	if len(block) == 0 {
		return from
	}
	for i := from; i+len(block) <= len(lines); i++ {
		match := true
		for j := range block {
			if lines[i+j] != block[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// blockMatchesNorm returns every index >= from at which block occurs contiguously in
// lines when each line is compared under the per-line normalizer norm (e.g. trailing
// whitespace or full indentation folded). Matches are non-overlapping. An empty block
// matches once at `from`.
func blockMatchesNorm(lines, block []string, from int, norm func(string) string) []int {
	if len(block) == 0 {
		return []int{from}
	}
	nb := make([]string, len(block))
	for j, b := range block {
		nb[j] = norm(b)
	}
	var hits []int
	for i := from; i+len(block) <= len(lines); i++ {
		match := true
		for j := range nb {
			if norm(lines[i+j]) != nb[j] {
				match = false
				break
			}
		}
		if match {
			hits = append(hits, i)
			i += len(block) - 1 // skip past this match so hits never overlap
		}
	}
	return hits
}

// diagnoseHunkMismatch builds the actionable error returned when a hunk's context
// matches nowhere, even fuzzily: it points at the file line the hunk's first non-blank
// context/removed line is closest to (searching from the cursor) and where they first
// diverge, so the patch can be regenerated against exact bytes.
func diagnoseHunkMismatch(lines, pre []string, from, hunkNum int) error {
	anchor := ""
	for _, l := range pre {
		if strings.TrimSpace(l) != "" {
			anchor = l
			break
		}
	}
	base := fmt.Sprintf("hunk %d does not match the file (context/removed lines not found from line %d onward)", hunkNum, from+1)
	tail := "Read the file again and regenerate the patch from its exact bytes — trailing whitespace, indentation and unicode punctuation are the usual culprits"
	if anchor == "" {
		return fmt.Errorf("%s — %s", base, tail)
	}
	na := strings.TrimSpace(anchor)
	bestIdx, bestScore := -1, -1
	for i := from; i < len(lines); i++ {
		ft := strings.TrimSpace(lines[i])
		score := commonPrefixRunes(ft, na)
		if na != "" && strings.Contains(ft, na) {
			score = len([]rune(na)) + 1
		}
		if score > bestScore {
			bestScore, bestIdx = score, i
		}
	}
	if bestIdx < 0 || bestScore <= 0 {
		return fmt.Errorf("%s — no similar line found. %s", base, tail)
	}
	col := firstDiffColumn(lines[bestIdx], anchor)
	return fmt.Errorf("%s. Closest is line %d:\n  file:  %s\n  patch: %s\n  first differ at column %d. %s",
		base, bestIdx+1, quoteForMsg(lines[bestIdx]), quoteForMsg(anchor), col, tail)
}

// joinLines is the inverse of splitLines (diff.go): it re-adds the trailing
// newline that a
// text file conventionally ends with (empty input → empty output, no newline).
func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
