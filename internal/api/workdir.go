package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// workdirInfo is the client view of a session's working directory: the stored
// override (Dir), the effective cwd actually used (Effective), and live git/fs
// facts for the badge.
type workdirInfo struct {
	Dir       string `json:"dir"`       // session override ("" = none)
	Effective string `json:"effective"` // override or workspace default
	Exists    bool   `json:"exists"`    // whether Effective points at a real dir
	IsGitRepo bool   `json:"isGitRepo"`
	Branch    string `json:"branch"` // current git branch ("" when not a repo)
}

// handleGetSessionWorkdir returns the session's working-directory state for the
// composer badge.
func (s *Server) handleGetSessionWorkdir(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()
	wsp := ws(r)
	session, err := wsp.DB.GetSession(ctx, id)
	if writeDBError(w, err, "session not found") {
		return
	}
	eff := strings.TrimSpace(session.WorkingDir)
	if eff == "" {
		eff = wsp.Runtime.WorkspaceDefaultDir()
	}
	info := workdirInfo{Dir: strings.TrimSpace(session.WorkingDir), Effective: eff}
	if eff != "" {
		if fi, statErr := os.Stat(eff); statErr == nil && fi.IsDir() {
			info.Exists = true
			if br := gitBranch(eff); br != "" {
				info.IsGitRepo = true
				info.Branch = br
			}
		}
	}
	writeJSON(w, http.StatusOK, info)
}

type setWorkdirReq struct {
	// Dir is the new working directory (absolute). Empty resets to the workspace
	// default.
	Dir string `json:"dir"`
}

// handleSetSessionWorkdir sets (or clears) a session's working directory. A
// non-empty dir must be an absolute path to an existing directory.
func (s *Server) handleSetSessionWorkdir(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req setWorkdirReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	dir := strings.TrimSpace(req.Dir)
	if dir != "" {
		if !filepath.IsAbs(dir) {
			writeError(w, http.StatusBadRequest, "working directory must be an absolute path")
			return
		}
		dir = filepath.Clean(dir)
		fi, err := os.Stat(dir)
		if err != nil || !fi.IsDir() {
			writeError(w, http.StatusBadRequest, "directory does not exist")
			return
		}
	}
	ctx := r.Context()
	wsp := ws(r)
	if _, err := wsp.DB.GetSession(ctx, id); writeDBError(w, err, "session not found") {
		return
	}
	if err := wsp.DB.SetSessionWorkingDir(ctx, id, dir); writeDBError(w, err, "") {
		return
	}
	// Reuse the GET shape so the client refreshes the badge in one round-trip.
	s.handleGetSessionWorkdir(w, r)
}

// browseEntry is one selectable directory in the folder picker.
type browseEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// browseResp is the folder picker's view of one directory level.
type browseResp struct {
	Path    string        `json:"path"`    // the listed directory ("" = drive/root list)
	Parent  string        `json:"parent"`  // parent dir ("" when at a root)
	Entries []browseEntry `json:"entries"` // immediate subdirectories (dirs only)
}

// handleBrowseDirs lists the immediate subdirectories of ?path= for the folder
// picker. An empty path returns the filesystem roots (drive letters on Windows,
// "/" elsewhere). Directories only; hidden entries are skipped.
func (s *Server) handleBrowseDirs(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))

	if path == "" {
		writeJSON(w, http.StatusOK, browseResp{Entries: rootEntries()})
		return
	}
	path = filepath.Clean(path)
	entries, err := os.ReadDir(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read directory: "+err.Error())
		return
	}
	out := make([]browseEntry, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		out = append(out, browseEntry{Name: e.Name(), Path: filepath.Join(path, e.Name())})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })

	parent := filepath.Dir(path)
	if parent == path { // already at a root (e.g. "C:\\" or "/")
		parent = ""
	}
	writeJSON(w, http.StatusOK, browseResp{Path: path, Parent: parent, Entries: out})
}

// rootEntries returns the filesystem roots for the picker: drive letters on
// Windows, "/" on POSIX.
func rootEntries() []browseEntry {
	if runtime.GOOS != "windows" {
		return []browseEntry{{Name: "/", Path: "/"}}
	}
	var roots []browseEntry
	for c := 'A'; c <= 'Z'; c++ {
		drive := string(c) + ":\\"
		if _, err := os.Stat(drive); err == nil {
			roots = append(roots, browseEntry{Name: drive, Path: drive})
		}
	}
	return roots
}
