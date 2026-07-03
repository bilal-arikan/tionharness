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

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// FSGrepTool searches file contents in the sandbox for a regular expression, with
// ripgrep-style output modes, context lines, filters and .gitignore awareness.
type FSGrepTool struct{ sb Sandbox }

// NewFSGrepTool binds the tool to a workspace sandbox.
func NewFSGrepTool(sb Sandbox) FSGrepTool { return FSGrepTool{sb: sb} }

func (FSGrepTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "Grep",
		Description: "Search file contents for a regular expression (RE2). Searches the working directory by default; pass path to scope to a file or directory. " +
			"output_mode: \"content\" (matching lines, default), \"files_with_matches\" (paths only), or \"count\" (match count per file). " +
			"Filter with glob or type (e.g. \"go\", \"ts\"). Content mode supports -A/-B/-C context, -i case-insensitive, -n line numbers (default on), -o only-matching. " +
			"multiline lets a match span lines. head_limit caps results. Honours .gitignore (always skips .git) unless no_ignore is set.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"pattern":{"type":"string","description":"RE2 regular expression to search for"},
				"path":{"type":"string","description":"File or directory to search (absolute or relative to the working directory); default working directory"},
				"glob":{"type":"string","description":"Glob to restrict which files are searched (e.g. \"**/*.go\")"},
				"type":{"type":"string","description":"Language file-type filter (e.g. go, py, ts, js, rust, java, json, yaml, md)"},
				"output_mode":{"type":"string","enum":["content","files_with_matches","count"],"description":"content (default), files_with_matches, or count"},
				"-i":{"type":"boolean","description":"Case-insensitive search"},
				"-n":{"type":"boolean","description":"Show line numbers (content mode; default true)"},
				"-A":{"type":"integer","description":"Lines of context after each match (content mode)"},
				"-B":{"type":"integer","description":"Lines of context before each match (content mode)"},
				"-C":{"type":"integer","description":"Lines of context before and after each match (content mode)"},
				"-o":{"type":"boolean","description":"Print only the matched part of each line (content mode)"},
				"multiline":{"type":"boolean","description":"Allow a match to span lines ( . matches newline )"},
				"head_limit":{"type":"integer","description":"Limit the number of results returned"},
				"no_ignore":{"type":"boolean","description":"Search files that .gitignore would exclude (default false)"}
			},
			"required":["pattern"],
			"additionalProperties":false
		}`),
	}
}

// grepArgs mirrors the schema; ripgrep-style flag keys keep parity with Claude Code.
type grepArgs struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path"`
	Glob       string `json:"glob"`
	Type       string `json:"type"`
	OutputMode string `json:"output_mode"`
	IgnoreCase bool   `json:"-i"`
	LineNums   *bool  `json:"-n"`
	After      int    `json:"-A"`
	Before     int    `json:"-B"`
	Context    int    `json:"-C"`
	OnlyMatch  bool   `json:"-o"`
	Multiline  bool   `json:"multiline"`
	HeadLimit  int    `json:"head_limit"`
	NoIgnore   bool   `json:"no_ignore"`
}

func (t FSGrepTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args grepArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return "", fmt.Errorf("pattern is required")
	}
	// Fast path: delegate to the ripgrep binary when available (faster + native
	// .gitignore/type handling). Any unsupported case or rg error falls through to
	// the in-process Go engine below, so behaviour is identical either way.
	if out, ok := t.tryRG(ctx, args); ok {
		return out, nil
	}
	re, err := compileGrepRegexp(args)
	if err != nil {
		return "", err
	}
	fileFilter, err := grepFileFilter(args)
	if err != nil {
		return "", err
	}

	files, root, err := t.collectFiles(args, fileFilter)
	if err != nil {
		return "", err
	}

	limit := args.HeadLimit
	if limit <= 0 {
		limit = fsGrepMaxHits
	}
	mode := args.OutputMode
	if mode == "" {
		mode = "content"
	}

	switch mode {
	case "files_with_matches":
		return grepFilesWithMatches(files, root, re, limit), nil
	case "count":
		return grepCount(files, root, re, args.Multiline, limit), nil
	case "content":
		return grepContent(files, root, re, args, limit), nil
	default:
		return "", fmt.Errorf("invalid output_mode %q (want content|files_with_matches|count)", mode)
	}
}

// compileGrepRegexp builds the RE2 matcher, applying case-insensitive and multiline
// (dot-matches-newline) flags as an inline prefix.
func compileGrepRegexp(args grepArgs) (*regexp.Regexp, error) {
	pat := args.Pattern
	var flags string
	if args.IgnoreCase {
		flags += "i"
	}
	if args.Multiline {
		flags += "s" // . matches newline so a match can span lines
	}
	if flags != "" {
		pat = "(?" + flags + ")" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression: %w", err)
	}
	return re, nil
}

// grepFileFilter returns a predicate deciding whether a file (by slash-relative path)
// is in scope, from the optional glob and type filters (both must pass when set).
func grepFileFilter(args grepArgs) (func(rel string) bool, error) {
	var globRe *regexp.Regexp
	if strings.TrimSpace(args.Glob) != "" {
		re, err := globToRegexp(args.Glob)
		if err != nil {
			return nil, err
		}
		globRe = re
	}
	var exts []string
	if strings.TrimSpace(args.Type) != "" {
		e, ok := grepTypeExts[strings.ToLower(strings.TrimSpace(args.Type))]
		if !ok {
			return nil, fmt.Errorf("unknown type %q", args.Type)
		}
		exts = e
	}
	return func(rel string) bool {
		if globRe != nil && !globRe.MatchString(rel) {
			return false
		}
		if exts != nil {
			ext := strings.ToLower(filepath.Ext(rel))
			ok := false
			for _, e := range exts {
				if ext == e {
					ok = true
					break
				}
			}
			if !ok {
				return false
			}
		}
		return true
	}, nil
}

// collectFiles resolves the search target: a single file, or every in-scope,
// non-ignored file under a directory. Returns absolute file paths and the root the
// display paths are relative to.
func (t FSGrepTool) collectFiles(args grepArgs, want func(string) bool) ([]string, string, error) {
	if strings.TrimSpace(args.Path) != "" {
		abs, err := t.sb.Resolve(args.Path)
		if err != nil {
			return nil, "", err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, "", err
		}
		if !info.IsDir() {
			// Single file: the display root is its parent so we show just the name.
			return []string{abs}, filepath.Dir(abs), nil
		}
		return walkGrepFiles(abs, args, want)
	}
	if !t.sb.Ready() {
		return nil, "", fmt.Errorf("filesystem sandbox is not configured")
	}
	return walkGrepFiles(t.sb.Root, args, want)
}

// walkGrepFiles walks root, honouring .gitignore (unless no_ignore) and the file
// filter, and returns the matching files' absolute paths.
func walkGrepFiles(root string, args grepArgs, want func(string) bool) ([]string, string, error) {
	ign := NewIgnoreSet(root, !args.NoIgnore)
	ign.LoadDir("") // root .gitignore applies to the whole tree
	var files []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel := relTo(root, p)
		if d.IsDir() {
			if rel == "" {
				return nil
			}
			ign.LoadDir(rel)
			if ign.Ignored(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if ign.Ignored(rel, false) || !want(rel) {
			return nil
		}
		files = append(files, p)
		return nil
	})
	return files, root, err
}

// readSearchable reads a file for searching, skipping unreadable or binary ones.
func readSearchable(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil || isBinary(data) {
		return "", false
	}
	return string(data), true
}

func grepFilesWithMatches(files []string, root string, re *regexp.Regexp, limit int) string {
	var out []string
	for _, f := range files {
		content, ok := readSearchable(f)
		if !ok {
			continue
		}
		if re.MatchString(content) {
			out = append(out, relTo(root, f))
			if len(out) >= limit {
				break
			}
		}
	}
	if len(out) == 0 {
		return "No matches."
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

func grepCount(files []string, root string, re *regexp.Regexp, multiline bool, limit int) string {
	var out []string
	for _, f := range files {
		content, ok := readSearchable(f)
		if !ok {
			continue
		}
		n := countMatches(content, re, multiline)
		if n > 0 {
			out = append(out, fmt.Sprintf("%s:%d", relTo(root, f), n))
			if len(out) >= limit {
				break
			}
		}
	}
	if len(out) == 0 {
		return "No matches."
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

// countMatches counts non-overlapping matches — per line unless multiline.
func countMatches(content string, re *regexp.Regexp, multiline bool) int {
	if multiline {
		return len(re.FindAllStringIndex(content, -1))
	}
	n := 0
	for _, line := range strings.Split(content, "\n") {
		n += len(re.FindAllStringIndex(line, -1))
	}
	return n
}

func grepContent(files []string, root string, re *regexp.Regexp, args grepArgs, limit int) string {
	before, after := args.Before, args.After
	if args.Context > 0 {
		if args.Context > before {
			before = args.Context
		}
		if args.Context > after {
			after = args.Context
		}
	}
	showNums := args.LineNums == nil || *args.LineNums // default on

	var b strings.Builder
	emitted := 0
	capped := false
	for _, f := range files {
		if capped {
			break
		}
		content, ok := readSearchable(f)
		if !ok {
			continue
		}
		rel := relTo(root, f)
		if args.Multiline {
			emitted, capped = grepEmitMultiline(&b, rel, content, re, showNums, args.OnlyMatch, emitted, limit)
			continue
		}
		emitted, capped = grepEmitLines(&b, rel, content, re, showNums, args.OnlyMatch, before, after, emitted, limit)
	}
	if emitted == 0 {
		return "No matches."
	}
	out := strings.TrimRight(b.String(), "\n")
	if capped {
		out += fmt.Sprintf("\n\n[stopped at %d results — narrow the search or raise head_limit]", limit)
	}
	return out
}

// grepEmitLines renders per-line matches for one file with optional context,
// merging overlapping context windows. Returns the running emitted count and whether
// the head limit was reached.
func grepEmitLines(b *strings.Builder, rel, content string, re *regexp.Regexp, showNums, onlyMatch bool, before, after, emitted, limit int) (int, bool) {
	lines := strings.Split(content, "\n")
	prevEnd := -1 // last line index emitted, for '--' separators and merge
	for i, line := range lines {
		if !re.MatchString(line) {
			continue
		}
		start, end := i-before, i+after
		if start < 0 {
			start = 0
		}
		if end >= len(lines) {
			end = len(lines) - 1
		}
		if before > 0 || after > 0 {
			if prevEnd >= 0 && start <= prevEnd+1 {
				start = prevEnd + 1 // contiguous: continue without a separator
			} else if prevEnd >= 0 {
				b.WriteString("--\n")
			}
		}
		for j := start; j <= end; j++ {
			isMatch := j == i || re.MatchString(lines[j])
			if onlyMatch && j == i {
				for _, m := range re.FindAllString(lines[j], -1) {
					writeGrepRow(b, rel, j+1, m, true, showNums)
					emitted++
					if emitted >= limit {
						return emitted, true
					}
				}
				continue
			}
			writeGrepRow(b, rel, j+1, strings.TrimRight(lines[j], "\r"), isMatch, showNums)
			emitted++
			if emitted >= limit {
				return emitted, true
			}
		}
		prevEnd = end
	}
	return emitted, false
}

// grepEmitMultiline renders whole-regex matches (which may span lines) for one file.
func grepEmitMultiline(b *strings.Builder, rel, content string, re *regexp.Regexp, showNums, onlyMatch bool, emitted, limit int) (int, bool) {
	for _, loc := range re.FindAllStringIndex(content, -1) {
		startLine := 1 + strings.Count(content[:loc[0]], "\n")
		text := content[loc[0]:loc[1]]
		if !onlyMatch {
			// Include the full first line for context around the match start.
			lineStart := strings.LastIndexByte(content[:loc[0]], '\n') + 1
			lineEnd := loc[1]
			if nl := strings.IndexByte(content[loc[1]:], '\n'); nl >= 0 {
				lineEnd = loc[1] + nl
			}
			text = content[lineStart:lineEnd]
		}
		writeGrepRow(b, rel, startLine, strings.TrimRight(text, "\r"), true, showNums)
		emitted++
		if emitted >= limit {
			return emitted, true
		}
	}
	return emitted, false
}

// writeGrepRow writes one output row. Match rows use "path:line:text"; context rows
// use "path-line-text" (ripgrep convention). Line numbers are omitted when showNums
// is false.
func writeGrepRow(b *strings.Builder, rel string, line int, text string, isMatch bool, showNums bool) {
	sep := "-"
	if isMatch {
		sep = ":"
	}
	if showNums {
		fmt.Fprintf(b, "%s%s%d%s%s\n", rel, sep, line, sep, text)
	} else {
		fmt.Fprintf(b, "%s%s%s\n", rel, sep, text)
	}
}

// grepTypeExts maps a language type filter to its file extensions.
var grepTypeExts = map[string][]string{
	"go":         {".go"},
	"py":         {".py", ".pyi"},
	"python":     {".py", ".pyi"},
	"js":         {".js", ".jsx", ".mjs", ".cjs"},
	"jsx":        {".jsx"},
	"ts":         {".ts", ".mts", ".cts"},
	"tsx":        {".tsx"},
	"rust":       {".rs"},
	"java":       {".java"},
	"c":          {".c", ".h"},
	"cpp":        {".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx"},
	"cs":         {".cs"},
	"rb":         {".rb"},
	"ruby":       {".rb"},
	"php":        {".php"},
	"sh":         {".sh", ".bash", ".zsh"},
	"json":       {".json"},
	"yaml":       {".yaml", ".yml"},
	"toml":       {".toml"},
	"md":         {".md", ".markdown"},
	"markdown":   {".md", ".markdown"},
	"html":       {".html", ".htm"},
	"css":        {".css", ".scss", ".sass", ".less"},
	"sql":        {".sql"},
	"xml":        {".xml"},
	"proto":      {".proto"},
	"kotlin":     {".kt", ".kts"},
	"swift":      {".swift"},
	"dockerfile": {".dockerfile"},
}
