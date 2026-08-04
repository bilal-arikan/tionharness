package tools

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/proc"
)

// rgTimeout bounds a delegated ripgrep run; a search that outlasts it falls back to
// the in-process Go engine.
const rgTimeout = 60 * time.Second

var (
	rgOnce sync.Once
	rgBin  string // resolved "rg" path, "" if absent or disabled
)

// rgExe resolves the ripgrep binary. It returns "" (fast path disabled) when rg is
// not on PATH or TIONSWARM_GREP_NO_RG is set — in which case Grep uses its built-in Go
// engine. The PATH lookup is cached for the process lifetime; the env opt-out is
// re-checked on every call so it can be toggled (e.g. per test) without the cache
// pinning an earlier decision.
func rgExe() string {
	if os.Getenv("TIONSWARM_GREP_NO_RG") != "" {
		return ""
	}
	rgOnce.Do(func() {
		if p, err := exec.LookPath("rg"); err == nil {
			rgBin = p
		} else {
			// One-time notice in the in-app Logs (slog default is the logbuf-backed
			// logger): Grep works either way, just without ripgrep's speed. Only fires
			// when rg is genuinely absent — an env opt-out returns before this Once runs.
			slog.Default().Info("grep: ripgrep (rg) not found on PATH — using the built-in Go search engine")
		}
	})
	return rgBin
}

// tryRG delegates the search to ripgrep when it is available, translating grepArgs to
// rg flags and returning rg's output normalised to the same shape as the Go engine
// (forward-slash paths, no leading "./", head-limited). It returns handled=false on
// any condition it does not confidently support (rg absent, unknown mode/type, rg
// internal error, timeout), so the caller transparently falls back to the Go engine.
func (t FSGrepTool) tryRG(ctx context.Context, args grepArgs) (string, bool) {
	exe := rgExe()
	if exe == "" {
		return "", false
	}
	dir, targets, ok := t.rgTarget(args)
	if !ok {
		return "", false
	}
	mode := args.OutputMode
	if mode == "" {
		mode = "content"
	}
	rgArgs, ok := buildRGArgs(args, mode, targets)
	if !ok {
		return "", false
	}

	runCtx, cancel := context.WithTimeout(ctx, rgTimeout)
	defer cancel()
	cmd := proc.CommandContext(runCtx, exe, rgArgs...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		// Exit 1 = no matches (a normal result); anything else (2 = error, or a
		// start/timeout failure) hands back to the Go engine.
		if ee, isExit := err.(*exec.ExitError); isExit && ee.ExitCode() == 1 {
			return "No matches.", true
		}
		return "", false
	}
	out := normalizeRGOutput(stdout.String(), args.HeadLimit)
	if out == "" {
		return "No matches.", true
	}
	return out, true
}

// rgTarget resolves the directory rg runs in and the positional targets (a file
// name or "." for a whole directory), mirroring collectFiles. ok=false when the
// sandbox is unconfigured, a path cannot be resolved, or a path is missing — the
// caller then defers to the Go engine, which reports the precise error.
func (t FSGrepTool) rgTarget(args grepArgs) (dir string, targets []string, ok bool) {
	if strings.TrimSpace(args.Path) != "" {
		abs, missing, _ := t.resolveGrepTargets(args.Path)
		if len(missing) > 0 || len(abs) == 0 {
			return "", nil, false // let the Go engine surface grepMissingPathErr
		}
		if len(abs) == 1 {
			a := abs[0]
			info, err := os.Stat(a)
			if err != nil {
				return "", nil, false
			}
			if info.IsDir() {
				return a, []string{"."}, true
			}
			return filepath.Dir(a), []string{filepath.Base(a)}, true
		}
		// Multiple targets: run from the sandbox root with each target expressed
		// relative to it, so rg prints the same repo-relative paths as the Go engine.
		if !t.sb.Ready() {
			return "", nil, false
		}
		root := t.sb.Root
		rel := make([]string, 0, len(abs))
		for _, a := range abs {
			r, err := filepath.Rel(root, a)
			if err != nil {
				return "", nil, false
			}
			rel = append(rel, filepath.ToSlash(r))
		}
		return root, rel, true
	}
	if !t.sb.Ready() {
		return "", nil, false
	}
	return t.sb.Root, []string{"."}, true
}

// buildRGArgs maps grepArgs to ripgrep flags. ok=false for an unsupported output mode
// or unknown type filter, so the caller falls back to the Go engine.
func buildRGArgs(args grepArgs, mode string, targets []string) ([]string, bool) {
	// --no-require-git makes rg honour .gitignore even outside a git repo (matching
	// the Go IgnoreSet); --hidden searches dotfiles (rg still auto-skips .git);
	// --path-separator / normalises Windows backslashes to the Go convention;
	// --sort path forces deterministic, lexical file order matching the Go engine's
	// WalkDir order (rg is otherwise parallel/unordered) so both paths are byte-identical.
	out := []string{"--color", "never", "--no-require-git", "--hidden", "--path-separator", "/", "--sort", "path"}

	switch mode {
	case "content":
		out = append(out, "--no-heading", "--with-filename")
		if args.LineNums == nil || *args.LineNums {
			out = append(out, "--line-number")
		} else {
			out = append(out, "--no-line-number")
		}
		if args.Before > 0 {
			out = append(out, "-B", strconv.Itoa(args.Before))
		}
		if args.After > 0 {
			out = append(out, "-A", strconv.Itoa(args.After))
		}
		if args.Context > 0 {
			out = append(out, "-C", strconv.Itoa(args.Context))
		}
		if args.OnlyMatch {
			out = append(out, "--only-matching")
		}
	case "files_with_matches":
		out = append(out, "--files-with-matches")
	case "count":
		out = append(out, "--count-matches")
	default:
		return nil, false
	}

	if args.IgnoreCase {
		out = append(out, "--ignore-case")
	}
	if args.Multiline {
		out = append(out, "--multiline", "--multiline-dotall")
	}
	if args.NoIgnore {
		out = append(out, "--no-ignore")
	}
	if g := strings.TrimSpace(args.Glob); g != "" {
		out = append(out, "--glob", g)
	}
	if ty := strings.ToLower(strings.TrimSpace(args.Type)); ty != "" {
		exts, known := grepTypeExts[ty]
		if !known {
			return nil, false
		}
		for _, e := range exts {
			out = append(out, "--glob", "*"+e) // gitignore-style: matches at any depth
		}
	}
	// -e guards patterns that begin with a dash; targets are positional and last.
	out = append(out, "--regexp", args.Pattern)
	out = append(out, targets...)
	return out, true
}

// normalizeRGOutput strips the leading "./" rg prints for a "." target, drops the
// trailing CR that rg keeps from CRLF files (the Go engine trims it too, so both
// paths match on Windows checkouts), and applies the head limit (default
// fsGrepMaxHits), appending a marker when results are truncated.
func normalizeRGOutput(raw string, headLimit int) string {
	raw = strings.TrimRight(raw, "\n")
	if raw == "" {
		return ""
	}
	limit := headLimit
	if limit <= 0 {
		limit = fsGrepMaxHits
	}
	lines := strings.Split(raw, "\n")
	truncated := false
	if len(lines) > limit {
		lines = lines[:limit]
		truncated = true
	}
	for i, ln := range lines {
		lines[i] = strings.TrimRight(strings.TrimPrefix(ln, "./"), "\r")
	}
	out := strings.Join(lines, "\n")
	if truncated {
		out += "\n\n[stopped at " + strconv.Itoa(limit) + " results — narrow the search or raise head_limit]"
	}
	return out
}
