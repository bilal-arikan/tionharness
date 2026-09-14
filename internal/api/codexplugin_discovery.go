package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Discovery and import of the codex plugin marketplaces already present on this
// machine.
//
// Why an import rather than direct use: codex reserves its own marketplace names
// ("openai-bundled" and the remote catalogs) and refuses to register them from
// any other source, so a workspace cannot simply point at the bundled directory.
// Importing makes a RENAMED COPY the workspace owns, which codex accepts.
//
// This is always an explicit user action. TionHarness never copies anything on
// its own initiative: the bundled plugins are OpenAI's proprietary files, and
// whether to copy them is the user's call, not the product's default.

// codexMarketplaceManifest is the subset of a marketplace.json we read.
type codexMarketplaceManifest struct {
	Name    string `json:"name"`
	Plugins []struct {
		Name string `json:"name"`
	} `json:"plugins"`
}

// discoveredMarketplaceDTO is one importable marketplace found on this machine.
type discoveredMarketplaceDTO struct {
	Name          string   `json:"name"`
	Root          string   `json:"root"`
	Reserved      bool     `json:"reserved"`
	SuggestedName string   `json:"suggestedName"`
	Plugins       []string `json:"plugins"`
}

// codexMarketplaceManifestPath is where a marketplace root keeps its manifest.
var codexMarketplaceManifestRel = filepath.Join(".agents", "plugins", "marketplace.json")

// handleDiscoverCodexMarketplaces lists the marketplaces installed under the
// user's own codex home, so the settings UI can offer them for import.
//
// Only marketplace manifests are read — never auth.json, secrets/ or any other
// file in that directory. A codex home holds live credentials, and a discovery
// feature has no business walking it.
func (s *Server) handleDiscoverCodexMarketplaces(w http.ResponseWriter, r *http.Request) {
	roots := codexMarketplaceSearchRoots()
	out := make([]discoveredMarketplaceDTO, 0, len(roots))
	seen := make(map[string]bool, len(roots))
	for _, root := range roots {
		manifest, err := readCodexMarketplaceManifest(root)
		if err != nil || strings.TrimSpace(manifest.Name) == "" {
			continue
		}
		if seen[root] {
			continue
		}
		seen[root] = true

		plugins := make([]string, 0, len(manifest.Plugins))
		for _, p := range manifest.Plugins {
			if name := strings.TrimSpace(p.Name); name != "" {
				plugins = append(plugins, name)
			}
		}
		reserved := db.CodexMarketplaceNameReserved(manifest.Name)
		out = append(out, discoveredMarketplaceDTO{
			Name:          manifest.Name,
			Root:          root,
			Reserved:      reserved,
			SuggestedName: suggestedCodexMarketplaceName(manifest.Name, reserved),
			Plugins:       plugins,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// importCodexMarketplaceReq asks for one discovered marketplace to be copied
// into the active workspace under a name the workspace owns.
type importCodexMarketplaceReq struct {
	// Root is the discovered marketplace directory (from the discovery endpoint).
	Root string `json:"root"`
	// Name is the name the copy is registered under. It must not be one codex
	// reserves, which is the whole reason the copy exists.
	Name string `json:"name"`
}

// handleImportCodexMarketplace copies a discovered marketplace into the
// workspace's own plugin directory and registers it in the workspace settings.
func (s *Server) handleImportCodexMarketplace(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	req, ok := bindJSON[importCodexMarketplaceReq](w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(req.Name)
	if !db.ValidCodexMarketplaceName(name) {
		if db.CodexMarketplaceNameReserved(name) {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("%q adı Codex tarafından ayrılmıştır; kopya için farklı bir ad seçin.", name))
			return
		}
		writeError(w, http.StatusBadRequest,
			"Geçersiz marketplace adı: yalnız harf, rakam, _ ve - kullanılabilir.")
		return
	}
	manifest, err := readCodexMarketplaceManifest(req.Root)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Marketplace okunamadı: "+err.Error())
		return
	}

	dest := filepath.Join(wsp.DataDir, "codex-marketplaces", name)
	if err := os.RemoveAll(dest); err != nil {
		writeError(w, http.StatusInternalServerError, "Eski kopya temizlenemedi: "+err.Error())
		return
	}
	if err := copyDirTree(req.Root, dest); err != nil {
		writeError(w, http.StatusInternalServerError, "Marketplace kopyalanamadı: "+err.Error())
		return
	}
	// The copy carries the ORIGINAL name, which codex would reject as reserved.
	// Rewriting it is what makes the copy usable at all.
	if err := rewriteCodexMarketplaceName(dest, name); err != nil {
		writeError(w, http.StatusInternalServerError, "Marketplace adı yazılamadı: "+err.Error())
		return
	}

	plugins := make([]string, 0, len(manifest.Plugins))
	for _, p := range manifest.Plugins {
		if n := strings.TrimSpace(p.Name); n != "" {
			plugins = append(plugins, n+"@"+name)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"marketplace": db.CodexMarketplace{
			Name:       name,
			Source:     dest,
			SourceType: db.CodexMarketplaceLocal,
		},
		// Offered, not enabled: which plugins to turn on stays the user's choice.
		"availablePlugins": plugins,
	})
}

// readCodexMarketplaceManifest reads root's marketplace.json.
func readCodexMarketplaceManifest(root string) (codexMarketplaceManifest, error) {
	var m codexMarketplaceManifest
	if strings.TrimSpace(root) == "" {
		return m, fmt.Errorf("marketplace dizini boş")
	}
	data, err := os.ReadFile(filepath.Join(root, codexMarketplaceManifestRel))
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	return m, nil
}

// rewriteCodexMarketplaceName sets the copied manifest's name, so plugin
// selectors resolve against the workspace's own marketplace instead of the
// reserved original.
func rewriteCodexMarketplaceName(root, name string) error {
	path := filepath.Join(root, codexMarketplaceManifestRel)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	doc["name"] = name
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// suggestedCodexMarketplaceName proposes a non-reserved name for a copy.
func suggestedCodexMarketplaceName(original string, reserved bool) string {
	if !reserved && db.ValidCodexMarketplaceName(original) {
		return original
	}
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			return r
		default:
			return '-'
		}
	}, original)
	cleaned = strings.Trim(cleaned, "-")
	if cleaned == "" {
		cleaned = "codex"
	}
	return "local-" + cleaned
}

// copyDirTree copies src to dst recursively (regular files and directories
// only; symlinks and other special entries are skipped rather than followed).
func copyDirTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s bir dizin değil", src)
	}
	return filepath.Walk(src, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case fi.IsDir():
			return os.MkdirAll(target, 0o755)
		case fi.Mode().IsRegular():
			return copyRegularFile(path, target, fi.Mode())
		default:
			return nil
		}
	})
}

// copyRegularFile copies one file, creating its parent directory.
func copyRegularFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode.Perm())
}
