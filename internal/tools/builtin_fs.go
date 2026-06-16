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

	"github.com/bilal/swarmgo/internal/providers"
)

const (
	fsReadMaxBytes  = 256 * 1024 // cap a single file read fed back to the model
	fsGrepMaxHits   = 200        // cap grep matches returned
	fsGlobMaxHits   = 500        // cap glob results returned
	fsListMaxItems  = 1000       // cap directory listing entries
)

// FSReadFileTool reads a UTF-8 text file from the workspace sandbox.
type FSReadFileTool struct{ sb Sandbox }

// NewFSReadFileTool binds the tool to a workspace sandbox.
func NewFSReadFileTool(sb Sandbox) FSReadFileTool { return FSReadFileTool{sb: sb} }

func (FSReadFileTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "read_file",
		Description: "Read a UTF-8 text file from the workspace. Returns up to 256KB. Paths are relative to the workspace root.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"path":{"type":"string","description":"File path relative to the workspace root"}},
			"required":["path"],
			"additionalProperties":false
		}`),
	}
}

func (t FSReadFileTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
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
		return "", fmt.Errorf("%q is a directory; use list_dir", args.Path)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	truncated := false
	if len(data) > fsReadMaxBytes {
		data = data[:fsReadMaxBytes]
		truncated = true
	}
	out := string(data)
	if truncated {
		out += "\n\n[truncated at 256KB]"
	}
	return out, nil
}

// FSWriteFileTool writes (creates or overwrites) a text file in the sandbox.
type FSWriteFileTool struct{ sb Sandbox }

// NewFSWriteFileTool binds the tool to a workspace sandbox.
func NewFSWriteFileTool(sb Sandbox) FSWriteFileTool { return FSWriteFileTool{sb: sb} }

func (FSWriteFileTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "write_file",
		Description: "Create or overwrite a text file in the workspace, creating parent directories as needed. Paths are relative to the workspace root.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"path":{"type":"string","description":"File path relative to the workspace root"},
				"content":{"type":"string","description":"Full file content to write"}
			},
			"required":["path","content"],
			"additionalProperties":false
		}`),
	}
}

func (t FSWriteFileTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	abs, err := t.sb.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, []byte(args.Content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Wrote %d bytes to %s", len(args.Content), t.sb.Rel(abs)), nil
}

// FSEditFileTool performs an exact string replacement in a sandbox file.
type FSEditFileTool struct{ sb Sandbox }

// NewFSEditFileTool binds the tool to a workspace sandbox.
func NewFSEditFileTool(sb Sandbox) FSEditFileTool { return FSEditFileTool{sb: sb} }

func (FSEditFileTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "edit_file",
		Description: "Replace an exact string in a workspace file. By default old_string must occur exactly once. Set replace_all to replace every occurrence.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"path":{"type":"string","description":"File path relative to the workspace root"},
				"old_string":{"type":"string","description":"Exact text to find"},
				"new_string":{"type":"string","description":"Replacement text"},
				"replace_all":{"type":"boolean","description":"Replace every occurrence (default false)"}
			},
			"required":["path","old_string","new_string"],
			"additionalProperties":false
		}`),
	}
}

func (t FSEditFileTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.OldString == args.NewString {
		return "", fmt.Errorf("old_string and new_string are identical")
	}
	abs, err := t.sb.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	content := string(data)
	n := strings.Count(content, args.OldString)
	if n == 0 {
		return "", fmt.Errorf("old_string not found in %s", t.sb.Rel(abs))
	}
	if n > 1 && !args.ReplaceAll {
		return "", fmt.Errorf("old_string occurs %d times in %s; set replace_all or make it unique", n, t.sb.Rel(abs))
	}
	if args.ReplaceAll {
		content = strings.ReplaceAll(content, args.OldString, args.NewString)
	} else {
		content = strings.Replace(content, args.OldString, args.NewString, 1)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Replaced %d occurrence(s) in %s", n, t.sb.Rel(abs)), nil
}

// FSListDirTool lists the entries of a sandbox directory.
type FSListDirTool struct{ sb Sandbox }

// NewFSListDirTool binds the tool to a workspace sandbox.
func NewFSListDirTool(sb Sandbox) FSListDirTool { return FSListDirTool{sb: sb} }

func (FSListDirTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "list_dir",
		Description: "List the files and subdirectories of a workspace directory. Use an empty path or \".\" for the workspace root.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"path":{"type":"string","description":"Directory path relative to the workspace root (default root)"}},
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
			return "", fmt.Errorf("invalid arguments: %w", err)
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

// FSGlobTool finds files in the sandbox matching a glob pattern.
type FSGlobTool struct{ sb Sandbox }

// NewFSGlobTool binds the tool to a workspace sandbox.
func NewFSGlobTool(sb Sandbox) FSGlobTool { return FSGlobTool{sb: sb} }

func (FSGlobTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "glob",
		Description: "Find files in the workspace whose path matches a glob pattern (e.g. \"**/*.go\", \"src/*.ts\"). Returns paths relative to the workspace root.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"pattern":{"type":"string","description":"Glob pattern; ** matches any number of directories"}},
			"required":["pattern"],
			"additionalProperties":false
		}`),
	}
}

func (t FSGlobTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return "", fmt.Errorf("pattern is required")
	}
	if !t.sb.Ready() {
		return "", fmt.Errorf("filesystem sandbox is not configured")
	}
	re, err := globToRegexp(args.Pattern)
	if err != nil {
		return "", err
	}
	var hits []string
	walkErr := filepath.WalkDir(t.sb.Root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			return nil
		}
		rel := t.sb.Rel(p)
		if re.MatchString(rel) {
			hits = append(hits, rel)
		}
		if len(hits) >= fsGlobMaxHits {
			return filepath.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}
	if len(hits) == 0 {
		return "No files matched.", nil
	}
	sort.Strings(hits)
	return strings.Join(hits, "\n"), nil
}

// FSGrepTool searches file contents in the sandbox for a regular expression.
type FSGrepTool struct{ sb Sandbox }

// NewFSGrepTool binds the tool to a workspace sandbox.
func NewFSGrepTool(sb Sandbox) FSGrepTool { return FSGrepTool{sb: sb} }

func (FSGrepTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "grep",
		Description: "Search workspace file contents for a regular expression. Returns matching lines as path:line:text. Optionally restrict to files matching a glob.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"pattern":{"type":"string","description":"RE2 regular expression to search for"},
				"glob":{"type":"string","description":"Optional glob to restrict which files are searched (e.g. \"**/*.go\")"}
			},
			"required":["pattern"],
			"additionalProperties":false
		}`),
	}
}

func (t FSGrepTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Pattern string `json:"pattern"`
		Glob    string `json:"glob"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return "", fmt.Errorf("pattern is required")
	}
	if !t.sb.Ready() {
		return "", fmt.Errorf("filesystem sandbox is not configured")
	}
	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regular expression: %w", err)
	}
	var globRe *regexp.Regexp
	if strings.TrimSpace(args.Glob) != "" {
		globRe, err = globToRegexp(args.Glob)
		if err != nil {
			return "", err
		}
	}

	var hits []string
	walkErr := filepath.WalkDir(t.sb.Root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel := t.sb.Rel(p)
		if globRe != nil && !globRe.MatchString(rel) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil || isBinary(data) {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if re.MatchString(line) {
				hits = append(hits, fmt.Sprintf("%s:%d:%s", rel, i+1, strings.TrimRight(line, "\r")))
				if len(hits) >= fsGrepMaxHits {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}
	if len(hits) == 0 {
		return "No matches.", nil
	}
	out := strings.Join(hits, "\n")
	if len(hits) >= fsGrepMaxHits {
		out += fmt.Sprintf("\n\n[stopped at %d matches]", fsGrepMaxHits)
	}
	return out, nil
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
