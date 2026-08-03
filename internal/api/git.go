package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/proc"
)

// gitCmdTimeout bounds each git invocation so a slow/hung repo can't stall a
// request.
const gitCmdTimeout = 5 * time.Second

// errGitMissing is returned when no git binary is resolvable on PATH. Without it
// every git call fails with an opaque exec error, so the callers translate this
// one into a human message ("git kurulu değil") instead.
var errGitMissing = errors.New("git bulunamadı (PATH'te git yok)")

// gitInstalled reports whether a git binary is resolvable on PATH (honouring
// PATHEXT on Windows). Not cached: git may be installed while the app runs, and
// LookPath is cheap enough for the handful of endpoints that ask.
func gitInstalled() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// runGit runs `git -C dir args...` with a timeout and returns trimmed stdout.
func runGit(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitCmdTimeout)
	defer cancel()
	full := append([]string{"-C", dir}, args...)
	cmd := proc.CommandContext(ctx, "git", full...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// gitInfoResp is the Project panel's view of a path's git state.
type gitInfoResp struct {
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`    // path is an existing directory
	IsGitRepo bool   `json:"isGitRepo"` // inside a git working tree
	Branch    string `json:"branch"`    // current branch ("" when none/detached)
	Remote    string `json:"remote"`    // origin remote URL ("" when unset)
	UserName  string `json:"userName"`  // repo-local user.name ("" when unset)
	UserEmail string `json:"userEmail"` // repo-local user.email ("" when unset)

	// GitInstalled reports whether a git binary exists on this machine at all. It
	// separates "no git" from "not a repo": without it the UI would offer a
	// `git init` button that can only fail. Path-independent.
	GitInstalled bool `json:"gitInstalled"`
}

// gitInfoFor probes a path's git state. Every git-returning handler builds its
// response through this so they cannot drift apart.
func gitInfoFor(path string) gitInfoResp {
	resp := gitInfoResp{Path: path, GitInstalled: gitInstalled()}
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		resp.Exists = true
	}
	if !resp.Exists || !resp.GitInstalled {
		return resp
	}
	if out, err := runGit(path, "rev-parse", "--is-inside-work-tree"); err == nil && out == "true" {
		resp.IsGitRepo = true
		resp.Branch, _ = runGit(path, "rev-parse", "--abbrev-ref", "HEAD")
		resp.Remote, _ = runGit(path, "remote", "get-url", "origin")
		resp.UserName, _ = runGit(path, "config", "--local", "user.name")
		resp.UserEmail, _ = runGit(path, "config", "--local", "user.email")
	}
	return resp
}

// initGitRepo runs `git init` (default branch "main") in an existing directory.
// Shared by the /api/git/init endpoint and the create-workspace flow.
func initGitRepo(dir string) error {
	if !gitInstalled() {
		return errGitMissing
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return errors.New("directory does not exist: " + dir)
	}
	// -b main needs git 2.28+; fall back to a plain init when it is rejected.
	if _, err := runGit(dir, "init", "-b", "main"); err != nil {
		if _, err2 := runGit(dir, "init"); err2 != nil {
			return errors.New("git init failed: " + err2.Error())
		}
	}
	return nil
}

// prepareGitRepo is the idempotent form used by every caller that wants "make
// this path a repo": optionally create the folder, skip when it is already a
// working tree, otherwise init it and seed a starter .gitignore.
func prepareGitRepo(dir string, createDir bool) error {
	if createDir {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if info := gitInfoFor(dir); info.IsGitRepo {
		return nil // already versioned — leave the repo (and its .gitignore) alone
	}
	if err := initGitRepo(dir); err != nil {
		return err
	}
	// The repo now EXISTS; only the ignore file failed. Say so plainly instead of
	// swallowing it — a caller reporting a bare "git init failed" here would be a
	// lie, and silently shipping a repo with no .gitignore is what this guards.
	if err := writeDefaultGitignore(dir); err != nil {
		return errors.New("git init tamam, .gitignore yazılamadı: " + err.Error())
	}
	return nil
}

// handleGitInfo reports the git state of ?path= for the Project panel.
func (s *Server) handleGitInfo(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	writeJSON(w, http.StatusOK, gitInfoFor(filepath.Clean(path)))
}

type gitInitReq struct {
	Path string `json:"path"`
	// CreateDir opts into creating the folder when it does not exist yet, so a path
	// typed for a project that has not been laid out can be initialised in one step.
	// Default false keeps the endpoint strict: a typo must not silently create a dir.
	CreateDir bool `json:"createDir"`
}

// handleGitInit runs `git init` (default branch "main") in a directory,
// optionally creating it first.
func (s *Server) handleGitInit(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[gitInitReq](w, r)
	if !ok {
		return
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if err := prepareGitRepo(path, req.CreateDir); err != nil {
		// A missing binary / missing directory is a client-correctable condition,
		// not a server fault — 400 so the UI can show the reason inline.
		if errors.Is(err, errGitMissing) || strings.HasPrefix(err.Error(), "directory does not exist") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, gitInfoFor(path))
}

type gitConfigReq struct {
	Path      string `json:"path"`
	Remote    string `json:"remote"`    // origin URL ("" leaves unchanged)
	UserName  string `json:"userName"`  // "" leaves unchanged
	UserEmail string `json:"userEmail"` // "" leaves unchanged
}

// handleGitConfig applies repo-local git settings: origin remote URL and the
// user.name / user.email identity. Only non-empty fields are written.
func (s *Server) handleGitConfig(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[gitConfigReq](w, r)
	if !ok {
		return
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if out, err := runGit(path, "rev-parse", "--is-inside-work-tree"); err != nil || out != "true" {
		writeError(w, http.StatusBadRequest, "not a git repository")
		return
	}
	if u := strings.TrimSpace(req.UserName); u != "" {
		if _, err := runGit(path, "config", "--local", "user.name", u); err != nil {
			writeError(w, http.StatusInternalServerError, "set user.name failed: "+err.Error())
			return
		}
	}
	if e := strings.TrimSpace(req.UserEmail); e != "" {
		if _, err := runGit(path, "config", "--local", "user.email", e); err != nil {
			writeError(w, http.StatusInternalServerError, "set user.email failed: "+err.Error())
			return
		}
	}
	if rem := strings.TrimSpace(req.Remote); rem != "" {
		// set-url when origin exists, else add it.
		if _, err := runGit(path, "remote", "get-url", "origin"); err == nil {
			_, err = runGit(path, "remote", "set-url", "origin", rem)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "set remote failed: "+err.Error())
				return
			}
		} else if _, err := runGit(path, "remote", "add", "origin", rem); err != nil {
			writeError(w, http.StatusInternalServerError, "add remote failed: "+err.Error())
			return
		}
	}
	// Reflect the new state. The path arrives in the body, but handleGitInfo reads
	// the query — re-read directly here instead.
	writeJSON(w, http.StatusOK, gitInfoFor(path))
}
