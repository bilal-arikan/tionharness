package agent

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/exttools"
	"github.com/bilal-arikan/tionharness/internal/proc"
	"github.com/bilal-arikan/tionharness/internal/procwatch"
)

// zvecGrepIndexDir is the directory zvec-grep keeps a workspace index in. It
// lives INSIDE the indexed root, unlike codebase-memory's store in the user's
// cache directory — which is why a new index is excluded from git before it
// exists (ensureZvecGrepGitExclude).
const zvecGrepIndexDir = ".zvec-grep"

// zvecGrepManifest is the file an index keeps at the top of zvecGrepIndexDir.
// Requiring it tells an index apart from zvec-grep's own runtime state, which
// lives in a directory of the SAME name in the user's home (~/.zvec-grep: daemon
// state, model cache) and would otherwise make every directory under home look
// indexed.
const zvecGrepManifest = "manifest.json"

// zvecGrepDefaultEmbedding is the model a NEW index is built with. zvec-grep has
// no built-in default — `zg index` on a fresh root fails without --embedding,
// ZVEC_GREP_EMBEDDING or a configured default — and a background job must never
// fall through to a REMOTE model: remote embedding uploads file contents to a
// provider and needs an interactive authorization grant. This is the local code
// model zvec-grep's own documentation uses.
const zvecGrepDefaultEmbedding = "local/potion-code-16m-v2"

// zvecGrepIndexTimeout bounds one background index run. A first index of a large
// repository takes minutes (the daemon embeds every file), so it is generous; it
// exists so a wedged child cannot hold the ledger's per-root claim for the life
// of the process.
const zvecGrepIndexTimeout = 30 * time.Minute

var (
	runZvecGrepIndex = func(ctx context.Context, command, root, embedding string) ([]byte, error) {
		cmd := proc.CommandContext(ctx, command, "index", root, "--embedding", embedding)
		cmd.Env = os.Environ()
		proc.TreeKill(cmd)
		h := procwatch.Default().Begin(ctx, procwatch.Meta{
			Kind: procwatch.KindExternal, Label: "zvec-grep index",
			Command: command + " index " + root + " --embedding " + embedding, Dir: root,
		})
		out, err := cmd.CombinedOutput()
		h.Started(cmd)
		h.AppendOutput(string(out))
		h.Finish(err)
		return out, err
	}
	// zvecGrepDetect is the catalog's resolver, behind a var so tests can stub it.
	zvecGrepDetect = func() (bool, string) { return exttools.Detect(exttools.ZvecGrepToolName) }
)

// EnsureZvecGrepIndexed brings the index covering cwd to a usable state,
// reporting whether one exists or is being built after the call — so the tool
// loop only tells the model "an index is on its way" when that is true.
//
// The lifecycle itself (which of create/refresh/rebuild is due, the single-run
// claim, and the recorded outcome) lives in EnsureZvecGrepIndex on top of the
// indexstate ledger. This name is kept because it is what the prompt builder,
// the tool loop and the chat turn already call.
func (r *Runtime) EnsureZvecGrepIndexed(ctx context.Context, cwd string) bool {
	return r.EnsureZvecGrepIndex(ctx, cwd)
}

// zvecGrepEmbedding returns the model for a new index: ZVEC_GREP_EMBEDDING when it
// names a local model, the documented local default otherwise.
func zvecGrepEmbedding() string {
	if m := strings.TrimSpace(os.Getenv("ZVEC_GREP_EMBEDDING")); strings.HasPrefix(m, "local/") {
		return m
	}
	return zvecGrepDefaultEmbedding
}

// zvecGrepCLI resolves the executable the auto-index runs: the server row's own
// command when it is the `zg` shim at an absolute path (the binary the MCP server
// itself runs); otherwise the catalog's resolver, which also finds npm global bin
// directories missing from this process's PATH; and as a last resort a bare `zg`
// row command for exec to resolve. "" when none applies.
func zvecGrepCLI(server db.MCPServer) string {
	if isZvecGrepShim(server.Command) && filepath.IsAbs(server.Command) {
		return server.Command
	}
	if found, p := zvecGrepDetect(); found {
		return p
	}
	if isZvecGrepShim(server.Command) {
		return server.Command
	}
	return ""
}

// zvecGrepIndexedRoot returns the nearest directory at or above dir that holds a
// zvec-grep index, or "". The home directory is never taken for one: its
// .zvec-grep is zvec-grep's runtime state, not an index.
func zvecGrepIndexedRoot(dir string) string {
	home, _ := os.UserHomeDir()
	for d := filepath.Clean(dir); ; {
		if home == "" || !zvecGrepSamePath(d, home) {
			if st, err := os.Stat(filepath.Join(d, zvecGrepIndexDir, zvecGrepManifest)); err == nil && !st.IsDir() {
				return d
			}
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// zvecGrepIndexTarget picks where a new index is built: the enclosing git
// repository, so a session that starts in a subdirectory does not mint a nested
// index the repository-level one would duplicate; dir itself when there is no
// repository. The walk stops at the user's home directory — a home under version
// control (a dotfiles repository) must not turn every unversioned folder beneath
// it into an index of the entire home.
func zvecGrepIndexTarget(dir string) string {
	dir = filepath.Clean(dir)
	home, _ := os.UserHomeDir()
	for d := dir; ; {
		if home != "" && zvecGrepSamePath(d, home) {
			return dir
		}
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return dir
		}
		d = parent
	}
}

// zvecGrepBroadDir reports whether dir is the user's home directory or a volume
// root: a working directory there is not a project, and an index would embed
// everything beneath it into a store written at its top.
func zvecGrepBroadDir(dir string) bool {
	dir = filepath.Clean(dir)
	if filepath.Dir(dir) == dir {
		return true
	}
	home, err := os.UserHomeDir()
	return err == nil && home != "" && zvecGrepSamePath(dir, home)
}

// zvecGrepSamePath compares two directory paths the way the host filesystem does:
// case-insensitively on Windows.
func zvecGrepSamePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// zvecGrepExcludeEntry is what ensureZvecGrepGitExclude appends.
const zvecGrepExcludeEntry = "# zvec-grep local index (added by TionHarness)\n" + zvecGrepIndexDir + "/\n"

// ensureZvecGrepGitExclude keeps a new index out of version control before it
// exists. zvec-grep writes its index into the repository and does not ignore it
// itself, so without this the index shows up as untracked files and an agent's
// `git add -A` commits a binary vector store. .git/info/exclude is used rather
// than .gitignore because it is local to this clone: TionHarness must not leave an
// edit in the user's tracked files. A root without a .git DIRECTORY (none, or a
// worktree/submodule whose .git is a file) is left alone.
func ensureZvecGrepGitExclude(root string) error {
	gitDir := filepath.Join(root, ".git")
	if st, err := os.Stat(gitDir); err != nil || !st.IsDir() {
		return nil
	}
	exclude := filepath.Join(gitDir, "info", "exclude")
	data, err := os.ReadFile(exclude)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		switch strings.TrimSpace(line) {
		case zvecGrepIndexDir, zvecGrepIndexDir + "/", "/" + zvecGrepIndexDir, "/" + zvecGrepIndexDir + "/":
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(exclude, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	entry := zvecGrepExcludeEntry
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		entry = "\n" + entry
	}
	if _, err := f.WriteString(entry); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// zvecGrepTail keeps the end of the CLI's output for the log: a failed index run
// can print a long progress stream, and the reason sits at the end.
func zvecGrepTail(out []byte) string {
	const limit = 2048
	s := strings.TrimSpace(string(out))
	if len(s) > limit {
		s = "…" + s[len(s)-limit:]
	}
	return s
}
