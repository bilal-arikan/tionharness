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

	"github.com/bilal-arikan/tionswarm/internal/providers"
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
		Name: "Read",
		Strict: true, // API-side input validation (schema has additionalProperties:false; registry normalizes required)
		Description: "Read a UTF-8 text file. Output is line-numbered (\"<lineno>\\t<content>\", cat -n style) — when copying text for Edit's old_string, strip the number+tab prefix. " +
			"By default returns the first 2000 lines (up to 256KB); use offset (1-based start line) and limit (line count) to read a window of a large file. " +
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
		if len(line) > fsReadMaxLineLen {
			line = line[:fsReadMaxLineLen] + "… [line truncated]"
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
		Description: "Replace an exact string in a file. Read the file first: an edit errors if the file was never read or was modified since it was last read. By default old_string must occur exactly once; set replace_all to replace every occurrence. Accepts an absolute path or one relative to the working directory.",
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
	n := strings.Count(old, args.OldString)
	if n == 0 {
		// Tolerate a common paste mistake: Read output is line-numbered ("<n>\t<text>"),
		// and the model sometimes copies those prefixes into old_string. If stripping
		// the cat -n prefixes makes it match, do so — and strip new_string the same way
		// so the replacement isn't written with stray numbering.
		if so := stripCatNPrefixes(args.OldString); so != args.OldString && strings.Count(old, so) > 0 {
			args.OldString = so
			args.NewString = stripCatNPrefixes(args.NewString)
			n = strings.Count(old, args.OldString)
		}
	}
	if n == 0 {
		return "", fmt.Errorf("old_string not found in %s — fix: Read the file first and copy the exact text, incl. whitespace/indentation (no line-number prefixes)", t.sb.Rel(abs))
	}
	if args.OldString == args.NewString {
		return "", fmt.Errorf("old_string and new_string are identical after stripping line-number prefixes — nothing to change")
	}
	if n > 1 && !args.ReplaceAll {
		return "", fmt.Errorf("old_string occurs %d times in %s; set replace_all or make it unique", n, t.sb.Rel(abs))
	}
	var content string
	if args.ReplaceAll {
		content = strings.ReplaceAll(old, args.OldString, args.NewString)
	} else {
		content = strings.Replace(old, args.OldString, args.NewString, 1)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
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
		Description: "List the files and subdirectories of a directory. Accepts an absolute path or one relative to the working directory. Use an empty path or \".\" for the working directory itself.",
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

// globToRegexp converts a glob pattern (supporting **, *, ?) into an anchored
// RE2 regexp matching slash-separated relative paths.
func globToRegexp(pattern string) (*regexp.Regexp, error) {
	pattern = filepath.ToSlash(pattern)
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*") // ** spans directory separators
				i++
				// swallow a trailing slash after ** so "**/x" also matches "x"
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					b.WriteString("(?:/)?")
					i++
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
