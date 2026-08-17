package api

import (
	"errors"
	"net/http"
	"sync"

	"github.com/bilal-arikan/tionswarm/internal/exttools"
)

// externalToolStatus is one tool's detection result for the Settings panel.
//
// Detection itself is presence-only (path resolution, nothing executed). The
// version fields go one step further and run the tool's `--version` flag — a
// side-effect-free call, bounded by a timeout — because "is it installed" is a
// far less useful answer than "which version, and is it current".
type externalToolStatus struct {
	Name     string `json:"name"`
	Desc     string `json:"desc"`
	URL      string `json:"url"`
	Category string `json:"category"`
	Wire     string `json:"wire"`
	Found    bool   `json:"found"`
	Path     string `json:"path,omitempty"`
	// Version is the installed version ("1.3.0"), empty when unreadable.
	Version string `json:"version,omitempty"`
	// VersionError explains why Version is empty, so the UI can say "okunamadı"
	// with a reason instead of silently showing nothing.
	VersionError string `json:"versionError,omitempty"`
	// UpdateKind / UpdateCommand / UpdateNote describe how this tool is upgraded;
	// the UI shows a one-click button only for "command".
	UpdateKind    string `json:"updateKind"`
	UpdateCommand string `json:"updateCommand,omitempty"`
	UpdateNote    string `json:"updateNote,omitempty"`
}

// externalToolUpdate is one tool's upstream release check.
type externalToolUpdate struct {
	Name string `json:"name"`
	// Status is one of exttools.Status* — "unknown" whenever either version is
	// unparseable, never a guess.
	Status      string `json:"status"`
	Latest      string `json:"latest,omitempty"`
	ReleaseURL  string `json:"releaseUrl,omitempty"`
	PublishedAt string `json:"publishedAt,omitempty"`
	// Stale marks a result served from an expired cache because the live fetch
	// failed (offline, rate-limited) — shown to the user rather than hidden.
	Stale bool   `json:"stale,omitempty"`
	Error string `json:"error,omitempty"`
}

// handleExternalTools reports whether each known external tool is present on
// this host, and which version it reports. Detection resolves a path only; the
// version probe runs `<tool> --version` with a 3s timeout (see
// exttools.LocalVersion). Probes run concurrently so the whole list costs one
// timeout, not seven.
func (s *Server) handleExternalTools(w http.ResponseWriter, r *http.Request) {
	out := make([]externalToolStatus, len(exttools.Catalog))
	var wg sync.WaitGroup
	for i, t := range exttools.Catalog {
		st := externalToolStatus{
			Name: t.Name, Desc: t.Desc, URL: t.URL, Category: t.Category, Wire: t.Wire,
			UpdateKind: t.Update.Kind, UpdateCommand: t.Update.UpdateCommandLine(), UpdateNote: t.Update.Note,
		}
		found, p := exttools.Detect(t.Name)
		st.Found, st.Path = found, p
		out[i] = st
		if !found || len(t.VersionArgs) == 0 {
			continue
		}
		wg.Add(1)
		go func(i int, path string, args []string) {
			defer wg.Done()
			v, err := exttools.LocalVersion(r.Context(), path, args)
			if err != nil {
				out[i].VersionError = err.Error()
				return
			}
			out[i].Version = v
		}(i, p, t.VersionArgs)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, out)
}

// handleExternalToolUpdates checks each installed tool's latest published
// release against the installed version.
//
// Separate from the detection endpoint on purpose: this one leaves the machine
// (GitHub API) and is therefore slow and failure-prone, while the panel must
// still render instantly on open. Results are cached for 6h in exttools; pass
// ?refresh=1 to drop the cache first.
func (s *Server) handleExternalToolUpdates(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("refresh") == "1" {
		exttools.InvalidateCache()
	}

	out := make([]externalToolUpdate, len(exttools.Catalog))
	var wg sync.WaitGroup
	for i, t := range exttools.Catalog {
		out[i] = externalToolUpdate{Name: t.Name, Status: exttools.StatusUnknown}
		found, p := exttools.Detect(t.Name)
		if !found {
			out[i].Error = "kurulu değil"
			continue
		}
		repo := t.Repo()
		if repo == "" {
			out[i].Error = "bu araç için GitHub release akışı yok"
			continue
		}
		wg.Add(1)
		go func(i int, t exttools.Tool, repo, path string) {
			defer wg.Done()
			fetch := exttools.LatestRelease
			if t.PreRelease {
				fetch = exttools.LatestPreRelease
			}
			rel, stale, err := fetch(r.Context(), repo)
			if err != nil {
				out[i].Error = err.Error()
				return
			}
			out[i].Latest, out[i].ReleaseURL = rel.Tag, rel.URL
			out[i].PublishedAt, out[i].Stale = rel.PublishedAt, stale

			local, verErr := exttools.LocalVersion(r.Context(), path, t.VersionArgs)
			if verErr != nil {
				out[i].Error = verErr.Error() // status stays "unknown"
				return
			}
			out[i].Status = exttools.Compare(local, rel.Tag)
		}(i, t, repo, p)
	}
	wg.Wait()

	if err := exttools.CacheWriteErr(); err != nil {
		s.logger.Warn("external-tools release cache write failed", "error", err)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleExternalToolUpdate runs one tool's update command.
//
// Deliberately restricted to package-manager-backed tools (exttools.UpdateCommand):
// tools whose upgrade means overwriting a binary or unpacking an archive answer
// 409 with the manual instructions instead. On Windows a running child — an MCP
// stdio server holding its own .exe, a piper synth in flight — locks the file,
// and a half-applied replacement leaves the tool broken with no way back.
//
// This is a USER action only; it is not exposed as an agent tool, so an agent
// cannot silently mutate the machine it runs on.
func (s *Server) handleExternalToolUpdate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	t := exttools.Find(name)
	if t == nil {
		writeError(w, http.StatusNotFound, "bilinmeyen araç: "+name)
		return
	}
	if found, _ := exttools.Detect(t.Name); !found {
		writeError(w, http.StatusConflict, t.Name+" bu cihazda kurulu değil")
		return
	}
	if t.Update.Kind != exttools.UpdateCommand {
		writeError(w, http.StatusConflict, t.Update.Note)
		return
	}

	s.logger.Info("external tool update started", "tool", t.Name, "command", t.Update.UpdateCommandLine())
	out, err := exttools.RunUpdate(r.Context(), *t)
	if err != nil {
		s.logger.Warn("external tool update failed", "tool", t.Name, "error", err)
		status := http.StatusInternalServerError
		if errors.Is(err, exttools.ErrManualUpdate) {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]any{"name": t.Name, "ok": false, "error": err.Error(), "output": out})
		return
	}

	// The installed binary changed, so every cached judgement about it is void.
	version, verErr := exttools.LocalVersion(r.Context(), pathOf(t.Name), t.VersionArgs)
	resp := map[string]any{"name": t.Name, "ok": true, "output": out, "version": version}
	if verErr != nil {
		resp["versionError"] = verErr.Error()
	}
	s.logger.Info("external tool update finished", "tool", t.Name, "version", version)
	writeJSON(w, http.StatusOK, resp)
}

// pathOf re-resolves a tool after an update: a package manager may have moved or
// re-created the executable, so the pre-update path can be stale.
func pathOf(name string) string {
	_, p := exttools.Detect(name)
	return p
}
