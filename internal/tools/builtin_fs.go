package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

const (
	fsReadMaxBytes    = 256 * 1024 // cap a single file read fed back to the model
	fsReadMaxLineLen  = 2000       // per-line char cap (a huge minified line can't blow the budget)
	fsReadDefaultLine = 2000       // default max lines returned when no explicit limit is given
	fsGrepMaxHits     = 200        // cap grep matches returned
	fsGlobMaxHits     = 500        // cap glob results returned
	fsListMaxItems    = 1000       // cap directory listing entries
)

// FSReadFileTool reads a UTF-8 text file from the workspace sandbox.
type FSReadFileTool struct {
	sb      Sandbox
	tracker *ReadTracker // records a freshness baseline for Edit/Write (nil = off)
}

// NewFSReadFileTool binds the tool to a workspace sandbox. tracker (may be nil)
// records each read as the freshness baseline the Edit/Write guard compares against.
func NewFSReadFileTool(sb Sandbox, tracker *ReadTracker) FSReadFileTool {
	return FSReadFileTool{sb: sb, tracker: tracker}
}

func (FSReadFileTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:   "Read",
		Strict: true, // API-side input validation (schema has additionalProperties:false; registry normalizes required)
		Description: "Read a UTF-8 text file. Output is line-numbered (\"<lineno>\\t<content>\", cat -n style) — when copying text for Edit's old_string, strip ONLY the number+tab prefix; keep the content byte-for-byte (unicode « » ✅ ⏳, emoji and alignment/whitespace exactly as shown, do not normalize). " +
			"By default returns the first 2000 lines (up to 256KB); use offset (1-based start line) and limit (line count) to read a window of a large file. Lines longer than 2000 chars are truncated (marked '… [line truncated]') — avoid copying old_string from such a line. " +
			"Accepts an absolute path or one relative to the working directory.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"path":{"type":"string","description":"Absolute path, or relative to the working directory"},
				"offset":{"type":"integer","description":"1-based line number to start from (default 1)"},
				"limit":{"type":"integer","description":"Maximum number of lines to return (default 2000)"}
			},
			"required":["path"],
			"additionalProperties":false
		}`),
	}
}

func (t FSReadFileTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	abs, err := t.sb.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a directory; use LS", args.Path)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	// Record the freshness baseline over the FULL on-disk content (before any window
	// slicing) so a later Edit/Write can detect an out-of-band change even when only
	// part of the file was read. A ranged read is a partial view.
	partial := args.Offset > 0 || args.Limit > 0
	t.tracker.Record(abs, ReadRecord{
		ModTime: info.ModTime(),
		Size:    int64(len(data)),
		Sum:     contentSum(data),
		Partial: partial,
	})
	return renderNumbered(string(data), args.Offset, args.Limit), nil
}

// renderNumbered slices content to the [offset, offset+limit) 1-based line window
// (offset<=0 → from line 1; limit<=0 → fsReadDefaultLine lines) and prefixes each
// returned line with its 1-based number + tab (cat -n style). Over-long lines are
// truncated per line, and the whole output is capped at fsReadMaxBytes; both cases
// append an explicit marker so the model knows content was cut.
func renderNumbered(content string, offset, limit int) string {
	if content == "" {
		return "(empty file)"
	}
	lines := strings.Split(content, "\n")
	// A trailing newline yields a final empty element; drop it so the line count
	// matches an editor's view.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	total := len(lines)

	start := offset
	if start <= 0 {
		start = 1
	}
	if start > total {
		return fmt.Sprintf("[offset %d is past the end of the file (%d lines)]", start, total)
	}
	max := limit
	if max <= 0 {
		max = fsReadDefaultLine
	}
	end := start - 1 + max
	if end > total {
		end = total
	}

	var b strings.Builder
	byteCap := false
	for i := start - 1; i < end; i++ {
		line := lines[i]
		// Truncate on runes, not bytes: slicing raw bytes could cut a multibyte
		// UTF-8 character in half and emit an invalid byte, breaking the tool's
		// promise to reproduce content (unicode included) exactly.
		if r := []rune(line); len(r) > fsReadMaxLineLen {
			line = string(r[:fsReadMaxLineLen]) + "… [line truncated]"
		}
		row := fmt.Sprintf("%6d\t%s\n", i+1, line)
		if b.Len()+len(row) > fsReadMaxBytes {
			byteCap = true
			end = i // report where we actually stopped
			break
		}
		b.WriteString(row)
	}
	out := strings.TrimRight(b.String(), "\n")
	if byteCap {
		out += fmt.Sprintf("\n\n[stopped at line %d of %d — 256KB output cap reached; continue with offset=%d]", end, total, end+1)
	} else if end < total {
		out += fmt.Sprintf("\n\n[showing lines %d-%d of %d — continue with offset=%d]", start, end, total, end+1)
	}
	return out
}

// FSWriteFileTool writes (creates or overwrites) a text file in the sandbox.
type FSWriteFileTool struct {
	sb      Sandbox
	tracker *ReadTracker // freshness guard for overwriting an existing file (nil = off)
}

// NewFSWriteFileTool binds the tool to a workspace sandbox. tracker (may be nil)
// enforces the read-before-overwrite / not-modified-since-read guard for a file
// that already exists; a brand-new file needs no prior read.
func NewFSWriteFileTool(sb Sandbox, tracker *ReadTracker) FSWriteFileTool {
	return FSWriteFileTool{sb: sb, tracker: tracker}
}

func (FSWriteFileTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "Write",
		Description: "Create or overwrite a text file, creating parent directories as needed. Overwriting an EXISTING file requires reading it first: the write errors if it was never read or was modified since. New files need no prior read. Accepts an absolute path or one relative to the working directory.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"path":{"type":"string","description":"Absolute path, or relative to the working directory"},
				"content":{"type":"string","description":"Full file content to write"}
			},
			"required":["path","content"],
			"additionalProperties":false
		}`),
	}
}

func (t FSWriteFileTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	abs, err := t.sb.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	// Capture the pre-write content (absent = new file) so we can surface a diff.
	old, statErr := os.ReadFile(abs)
	created := statErr != nil
	// Freshness guard: overwriting an EXISTING file requires it to have been read
	// and to be unchanged since (a blind overwrite of an out-of-band edit is the
	// footgun this prevents). A brand-new file needs no prior read.
	if !created {
		if err := checkFreshness(t.tracker, abs, old); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, []byte(args.Content), 0o644); err != nil {
		return "", err
	}
	// Mutation verifier (self-healing): confirm the bytes actually landed —
	// a "successful" write clobbered by AV/concurrent writers must surface as
	// an error, not let the agent build on a change that never happened.
	if err := verifyMutationLanded(abs, []byte(args.Content)); err != nil {
		return "", err
	}
	// Refresh the baseline to the content just authored, so a follow-up Write/Edit
	// does not demand a re-Read of what this agent itself just wrote.
	recordWritten(t.tracker, abs, []byte(args.Content))
	added, removed, patch := lineDiff(string(old), args.Content)
	recordDiff(ctx, FileDiff{Path: t.sb.Rel(abs), Added: added, Removed: removed, Patch: patch, Created: created})
	return fmt.Sprintf("Wrote %d bytes to %s", len(args.Content), t.sb.Rel(abs)), nil
}

// FSEditFileTool performs an exact string replacement in a sandbox file.
type FSEditFileTool struct {
	sb      Sandbox
	tracker *ReadTracker // freshness guard: require a prior, still-current Read (nil = off)
}

// NewFSEditFileTool binds the tool to a workspace sandbox. tracker (may be nil)
// enforces that the file was read and is unchanged since before an edit is applied.
func NewFSEditFileTool(sb Sandbox, tracker *ReadTracker) FSEditFileTool {
	return FSEditFileTool{sb: sb, tracker: tracker}
}

func (FSEditFileTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "Edit",
		Description: "Replace an exact string in a file. Read the target region first and copy old_string VERBATIM from that output — strip only the line-number+tab prefix, never normalize the content (keep unicode/emoji and alignment spaces as-is), since a string rewritten from memory usually will not match. An edit errors if the file was never read or was modified since. By default old_string must occur exactly once; set replace_all to replace every occurrence. If a match keeps failing, target a short UNIQUE ASCII fragment (unicode punctuation and trailing whitespace are the usual culprits). Accepts an absolute path or one relative to the working directory.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"path":{"type":"string","description":"Absolute path, or relative to the working directory"},
				"old_string":{"type":"string","description":"Exact text to find"},
				"new_string":{"type":"string","description":"Replacement text"},
				"replace_all":{"type":"boolean","description":"Replace every occurrence (default false)"}
			},
			"required":["path","old_string","new_string"],
			"additionalProperties":false
		}`),
	}
}

func (t FSEditFileTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	if args.OldString == args.NewString {
		return "", fmt.Errorf("old_string and new_string are identical — fix: make new_string differ from old_string")
	}
	abs, err := t.sb.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	// Freshness guard BEFORE mutating: the file must have been read and be unchanged
	// since, so the edit never silently overwrites an out-of-band modification.
	if err := checkFreshness(t.tracker, abs, data); err != nil {
		return "", err
	}
	old := string(data)
	content, n, err := computeEdit(old, args.OldString, args.NewString, args.ReplaceAll, t.sb.Rel(abs))
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return "", err
	}
	// Mutation verifier (self-healing): the edit must actually be on disk.
	if err := verifyMutationLanded(abs, []byte(content)); err != nil {
		return "", err
	}
	// Refresh the baseline to the post-edit content so a chain of edits on the same
	// file does not require a re-Read between each step.
	recordWritten(t.tracker, abs, []byte(content))
	added, removed, patch := lineDiff(old, content)
	recordDiff(ctx, FileDiff{Path: t.sb.Rel(abs), Added: added, Removed: removed, Patch: patch})
	return fmt.Sprintf("Replaced %d occurrence(s) in %s", n, t.sb.Rel(abs)), nil
}

// FSListDirTool lists the entries of a sandbox directory.
type FSListDirTool struct{ sb Sandbox }

// NewFSListDirTool binds the tool to a workspace sandbox.
func NewFSListDirTool(sb Sandbox) FSListDirTool { return FSListDirTool{sb: sb} }

func (FSListDirTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "LS",
		Description: "List the files and subdirectories of a directory. Accepts an absolute path or one relative to the working directory. Use an empty path or \".\" for the working directory itself. Lists up to 1000 entries (a '... (N more)' marker is appended when that cap is hit).",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"path":{"type":"string","description":"Absolute path, or relative to the working directory (default working directory)"}},
			"additionalProperties":false
		}`),
	}
}

func (t FSListDirTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return "", argErr(err)
		}
	}
	abs, err := t.sb.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "(empty directory)", nil
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir() // directories first
		}
		return entries[i].Name() < entries[j].Name()
	})
	var b strings.Builder
	for i, e := range entries {
		if i >= fsListMaxItems {
			fmt.Fprintf(&b, "... (%d more)\n", len(entries)-fsListMaxItems)
			break
		}
		if e.IsDir() {
			fmt.Fprintf(&b, "%s/\n", e.Name())
			continue
		}
		size := int64(-1)
		if info, err := e.Info(); err == nil {
			size = info.Size()
		}
		fmt.Fprintf(&b, "%s\t%d bytes\n", e.Name(), size)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// catNPrefixRe matches the "<spaces><number>\t" prefix that Read prepends to each
// line (cat -n style). Anchored per line (multiline mode).
var catNPrefixRe = regexp.MustCompile(`(?m)^ *\d+\t`)

// stripCatNPrefixes removes a leading line-number+tab prefix from every line, so an
// old_string/new_string accidentally copied from line-numbered Read output can still
// match the raw file. It is a best-effort recovery, applied only when the verbatim
// match fails (see the Edit tool); text without such prefixes is returned unchanged.
func stripCatNPrefixes(s string) string {
	return catNPrefixRe.ReplaceAllString(s, "")
}

// toCRLF rewrites every line ending in s to CRLF, collapsing any existing CRLF
// first so a mixed or bare-LF input becomes uniformly CRLF. Used by Edit to align
// a model-supplied (LF) old_string to a CRLF file on disk so multi-line matches
// still land — and to keep the written replacement in the file's own convention.
func toCRLF(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}

// computeEdit resolves an Edit's replacement against the on-disk content and returns
// the new file content plus the number of occurrences replaced. Matching is tried in
// widening order, and the first that lands wins:
//
//  1. Verbatim byte match (the fast, exact path).
//  2. Recovery normalizations: strip cat -n line-number prefixes copied from Read's
//     output, and/or align bare-LF newlines to a CRLF file — with the same transform
//     applied to new_string so the write stays clean and the file keeps its endings.
//  3. Whitespace-tolerant, LINE-BASED fallback for old_strings rewritten from memory
//     that differ only in trailing whitespace or indentation. It fires ONLY on a
//     UNIQUE location (or, with replace_all, on every location) so the wrong block is
//     never edited, and it replaces the file's real bytes — never a silent no-op.
//
// When nothing matches it returns a diagnostic error that pinpoints the file line the
// supplied text is closest to and the column at which they diverge, and steers toward
// a short unique ASCII fragment (unicode punctuation/emoji and trailing spaces are the
// usual culprits when a remembered old_string won't match).
func computeEdit(old, oldStr, newStr string, replaceAll bool, rel string) (string, int, error) {
	n := strings.Count(old, oldStr)
	if n == 0 {
		cands := []func(string) string{stripCatNPrefixes}
		if strings.Contains(old, "\r\n") {
			cands = append(cands, toCRLF, func(s string) string { return toCRLF(stripCatNPrefixes(s)) })
		}
		for _, norm := range cands {
			if cand := norm(oldStr); cand != oldStr && strings.Count(old, cand) > 0 {
				oldStr, newStr = cand, norm(newStr)
				n = strings.Count(old, oldStr)
				break
			}
		}
	}
	if n == 0 {
		// Whitespace-tolerant line fallback. rtrim (drop trailing spaces/tabs/CR) is
		// tried before full trim (also fold indentation) because it is the safer, more
		// common divergence; each level demands uniqueness so a fuzzy match can never
		// hit the wrong block.
		fileCRLF := strings.Contains(old, "\r\n")
		for _, norm := range []func(string) string{rtrimLine, strings.TrimSpace} {
			ranges, endsBoundary := fuzzyLineMatch(old, oldStr, norm)
			if len(ranges) == 0 {
				continue
			}
			if !replaceAll && len(ranges) > 1 {
				return "", 0, fmt.Errorf("old_string has no verbatim match in %s, and a whitespace-insensitive match is ambiguous (%d candidates) — add surrounding lines to make it unique, or set replace_all", rel, len(ranges))
			}
			repl := newStr
			if fileCRLF {
				repl = toCRLF(repl)
			}
			if endsBoundary {
				nl := "\n"
				if fileCRLF {
					nl = "\r\n"
				}
				if !strings.HasSuffix(repl, nl) {
					repl += nl // the needle owned a trailing line break; keep the file well-formed
				}
			}
			return applyRanges(old, ranges, repl), len(ranges), nil
		}
	}
	if n == 0 {
		return "", 0, diagnoseNoMatch(old, oldStr, rel)
	}
	if oldStr == newStr {
		return "", 0, fmt.Errorf("old_string and new_string are identical after stripping line-number prefixes — nothing to change")
	}
	if n > 1 && !replaceAll {
		return "", 0, fmt.Errorf("old_string occurs %d times in %s; set replace_all or make it unique", n, rel)
	}
	if replaceAll {
		return strings.ReplaceAll(old, oldStr, newStr), n, nil
	}
	return strings.Replace(old, oldStr, newStr, 1), n, nil
}

// rtrimLine drops trailing spaces, tabs and a carriage return from a single line —
// the whitespace an old_string rewritten from memory most often gets wrong.
func rtrimLine(s string) string { return strings.TrimRight(s, " \t\r") }

// needleLineSlice splits a (possibly CRLF) old_string into its lines for line-based
// matching. endsBoundary reports whether the needle ended on a line break, so the
// caller can keep the file well-formed when it substitutes across that boundary.
func needleLineSlice(needle string) (lines []string, endsBoundary bool) {
	s := strings.ReplaceAll(needle, "\r\n", "\n")
	if strings.HasSuffix(s, "\n") {
		endsBoundary = true
		s = strings.TrimSuffix(s, "\n")
	}
	if s == "" && !endsBoundary {
		return nil, false
	}
	return strings.Split(s, "\n"), endsBoundary
}

// splitKeepOffsets returns each line of content (terminator excluded) alongside the
// byte offset at which it begins, so a line-window match maps back to exact bytes.
func splitKeepOffsets(content string) (lines []string, offs []int) {
	i := 0
	for {
		j := strings.IndexByte(content[i:], '\n')
		if j < 0 {
			lines = append(lines, content[i:])
			offs = append(offs, i)
			return
		}
		lines = append(lines, content[i:i+j])
		offs = append(offs, i)
		i += j + 1
	}
}

// fuzzyLineMatch locates every byte range in content whose consecutive lines equal
// the needle's lines under the per-line normalizer norm. Ranges are non-overlapping.
// endsBoundary is threaded back from the needle so the caller knows whether the range
// should include the trailing line break.
func fuzzyLineMatch(content, needle string, norm func(string) string) (ranges [][2]int, endsBoundary bool) {
	nLines, eb := needleLineSlice(needle)
	endsBoundary = eb
	if len(nLines) == 0 {
		return nil, eb
	}
	nn := make([]string, len(nLines))
	for i, l := range nLines {
		nn[i] = norm(l)
	}
	fLines, fOffs := splitKeepOffsets(content)
	k := len(nn)
	for i := 0; i+k <= len(fLines); i++ {
		match := true
		for j := 0; j < k; j++ {
			if norm(fLines[i+j]) != nn[j] {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		last := i + k - 1
		start := fOffs[i]
		var end int
		if eb {
			if last+1 < len(fOffs) {
				end = fOffs[last+1] // include the terminator after the last matched line
			} else {
				end = len(content)
			}
		} else {
			// Stop at the last line's visible content: a CRLF file leaves a '\r' on the
			// split line, so exclude it too — otherwise the range bisects the CRLF and
			// the write drops the '\r', corrupting the line ending.
			end = fOffs[last] + len(strings.TrimSuffix(fLines[last], "\r"))
		}
		ranges = append(ranges, [2]int{start, end})
		i = last // skip past this match so ranges never overlap
	}
	return ranges, eb
}

// applyRanges rebuilds content with every [start,end) range replaced by replacement.
// Ranges must be ascending and non-overlapping (as fuzzyLineMatch returns them).
func applyRanges(content string, ranges [][2]int, replacement string) string {
	var b strings.Builder
	prev := 0
	for _, r := range ranges {
		b.WriteString(content[prev:r[0]])
		b.WriteString(replacement)
		prev = r[1]
	}
	b.WriteString(content[prev:])
	return b.String()
}

// diagnoseNoMatch builds the actionable error returned when an old_string matches
// nowhere, even fuzzily. It finds the file line the supplied text is closest to and
// reports where they first diverge, so the agent can copy exact bytes instead of
// guessing again.
func diagnoseNoMatch(old, oldStr, rel string) error {
	needleLines, _ := needleLineSlice(oldStr)
	anchor := ""
	for _, l := range needleLines {
		if strings.TrimSpace(l) != "" {
			anchor = l
			break
		}
	}
	base := fmt.Sprintf("old_string not found in %s", rel)
	tail := "Read the file first and copy a short, UNIQUE ASCII fragment verbatim — avoid unicode punctuation/emoji (« » ✅ ⏳) and trailing whitespace, which often differ from what you remember, and drop any line-number prefixes"
	if anchor == "" {
		return fmt.Errorf("%s — %s", base, tail)
	}
	na := strings.TrimSpace(anchor)
	fLines := strings.Split(old, "\n")
	bestIdx, bestScore := -1, -1
	for i, fl := range fLines {
		ft := strings.TrimSpace(fl)
		score := commonPrefixRunes(ft, na)
		if na != "" && strings.Contains(ft, na) {
			score = len([]rune(na)) + 1 // a containing line beats any prefix overlap
		}
		if score > bestScore {
			bestScore, bestIdx = score, i
		}
	}
	if bestIdx < 0 || bestScore <= 0 {
		return fmt.Errorf("%s — no similar line found. %s", base, tail)
	}
	fl := fLines[bestIdx]
	col := firstDiffColumn(fl, anchor)
	return fmt.Errorf("%s. Closest is line %d:\n  file:  %s\n  yours: %s\n  first differ at column %d. Copy the file's exact bytes there, or %s",
		base, bestIdx+1, quoteForMsg(fl), quoteForMsg(anchor), col, tail)
}

// commonPrefixRunes counts the leading runes a and b share.
func commonPrefixRunes(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	n := 0
	for n < len(ra) && n < len(rb) && ra[n] == rb[n] {
		n++
	}
	return n
}

// firstDiffColumn returns the 1-based rune column at which a and b first differ (or
// the length+1 of the shorter when one is a prefix of the other).
func firstDiffColumn(a, b string) int {
	return commonPrefixRunes(a, b) + 1
}

// quoteForMsg renders a line for an error message: quoted (so trailing whitespace and
// unicode are visible) and length-capped so a long line cannot bloat the message.
func quoteForMsg(s string) string {
	const cap = 160
	r := []rune(s)
	if len(r) > cap {
		s = string(r[:cap]) + "…"
	}
	return fmt.Sprintf("%q", s)
}

// globToRegexp converts a glob pattern (supporting **, *, ?) into an anchored
// RE2 regexp matching slash-separated relative paths.
//
// "**/" is a DIRECTORY-BOUNDARY wildcard: it stands for "zero or more complete
// path segments", so "**/x.go" matches "x.go" and "a/b/x.go" but never
// "barx.go". Emitting it as ".*(?:/)?" (the obvious-looking translation) drops
// that boundary and silently over-matches on any filename that merely ENDS with
// the pattern — e.g. "**/log_2026*" hitting "d81c8e90-log_20260729.txt".
func globToRegexp(pattern string) (*regexp.Regexp, error) {
	pattern = filepath.ToSlash(pattern)
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					// "**/" = any number of WHOLE leading segments (possibly none).
					// The group must end in '/', which is what keeps the match on a
					// directory boundary.
					b.WriteString("(?:.*/)?")
					i++
				} else {
					b.WriteString(".*") // bare ** spans directory separators
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// isBinary reports whether data looks like a binary (non-text) file, by the
// presence of a NUL byte in the inspected prefix.
func isBinary(data []byte) bool {
	n := len(data)
	if n > 8000 {
		n = 8000
	}
	for i := 0; i < n; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}
