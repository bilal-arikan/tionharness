package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bilal-arikan/swarmgo/internal/proc"
)

// gitCmdTimeout bounds each git invocation so a slow/hung repo can't stall a
// request.
const gitCmdTimeout = 5 * time.Second

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
}

// handleGitInfo reports the git state of ?path= for the Project panel.
func (s *Server) handleGitInfo(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	path = filepath.Clean(path)
	resp := gitInfoResp{Path: path}
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		resp.Exists = true
	}
	if resp.Exists {
		if out, err := runGit(path, "rev-parse", "--is-inside-work-tree"); err == nil && out == "true" {
			resp.IsGitRepo = true
			resp.Branch, _ = runGit(path, "rev-parse", "--abbrev-ref", "HEAD")
			resp.Remote, _ = runGit(path, "remote", "get-url", "origin")
			resp.UserName, _ = runGit(path, "config", "--local", "user.name")
			resp.UserEmail, _ = runGit(path, "config", "--local", "user.email")
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type gitInitReq struct {
	Path string `json:"path"`
}

// handleGitInit runs `git init` (default branch "main") in an existing directory.
func (s *Server) handleGitInit(w http.ResponseWriter, r *http.Request) {
	var req gitInitReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		writeError(w, http.StatusBadRequest, "directory does not exist")
		return
	}
	// -b main needs git 2.28+; fall back to a plain init when it is rejected.
	if _, err := runGit(path, "init", "-b", "main"); err != nil {
		if _, err2 := runGit(path, "init"); err2 != nil {
			writeError(w, http.StatusInternalServerError, "git init failed: "+err2.Error())
			return
		}
	}
	resp := gitInfoResp{Path: path, Exists: true, IsGitRepo: true}
	resp.Branch, _ = runGit(path, "rev-parse", "--abbrev-ref", "HEAD")
	resp.Remote, _ = runGit(path, "remote", "get-url", "origin")
	resp.UserName, _ = runGit(path, "config", "--local", "user.name")
	resp.UserEmail, _ = runGit(path, "config", "--local", "user.email")
	writeJSON(w, http.StatusOK, resp)
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
	var req gitConfigReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
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
	resp := gitInfoResp{Path: path, Exists: true, IsGitRepo: true}
	resp.Branch, _ = runGit(path, "rev-parse", "--abbrev-ref", "HEAD")
	resp.Remote, _ = runGit(path, "remote", "get-url", "origin")
	resp.UserName, _ = runGit(path, "config", "--local", "user.name")
	resp.UserEmail, _ = runGit(path, "config", "--local", "user.email")
	writeJSON(w, http.StatusOK, resp)
}
