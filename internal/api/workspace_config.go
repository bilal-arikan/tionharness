package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/prompts"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// wsConfigDTO is the client view of a workspace's editable config files: the
// registry prompts (with their embedded defaults for "reset" and their UI
// metadata), the workspace instructions and the free-form README. All live
// under <workspace>/config/.
type wsConfigDTO struct {
	Dir          string                  `json:"dir"`
	Prompts      map[string]string       `json:"prompts"`  // key → current file content
	Defaults     map[string]string       `json:"defaults"` // key → embedded default
	PromptKeys   []string                `json:"promptKeys"`
	PromptMeta   map[string]promptMetaTO `json:"promptMeta"` // key → registry metadata
	Instructions string                  `json:"instructions"`
	Readme       string                  `json:"readme"`
}

// promptMetaTO is the per-key registry metadata the editor renders: label,
// hint, required placeholders and whether an edit only lands on NEW
// sessions/epochs (the prompt rides the cached static prefix).
type promptMetaTO struct {
	Label            string   `json:"label"`
	Hint             string   `json:"hint"`
	Placeholders     []string `json:"placeholders,omitempty"`
	EpochAffecting   bool     `json:"epochAffecting,omitempty"`
	OwnedBySystemKey string   `json:"ownedBySystemKey,omitempty"`
}

// wsConfigPatch is a partial update; omitted fields are left unchanged. A prompt
// entry written as "" clears the file so the compiled-in default takes over.
type wsConfigPatch struct {
	Prompts      map[string]string `json:"prompts"`
	Instructions *string           `json:"instructions"`
	Readme       *string           `json:"readme"`
}

// readFileOr returns a file's content, or fallback when it is missing/unreadable.
func readFileOr(path, fallback string) string {
	if data, err := os.ReadFile(path); err == nil {
		return string(data)
	}
	return fallback
}

// buildWSConfigDTO reads the active workspace's config files into a DTO.
func buildWSConfigDTO(wsp *workspace.Workspace) wsConfigDTO {
	wsDir := wsp.DataDir
	dto := wsConfigDTO{
		Dir:          agent.WorkspaceConfigDir(wsDir),
		Prompts:      map[string]string{},
		Defaults:     map[string]string{},
		PromptKeys:   agent.PromptKeys,
		PromptMeta:   map[string]promptMetaTO{},
		Instructions: readFileOr(agent.InstructionsFilePath(wsDir), ""),
		Readme:       readFileOr(agent.ReadmeFilePath(wsDir), ""),
	}
	for _, spec := range prompts.Specs() {
		key := spec.Key
		def := agent.PromptDefault(key)
		dto.Defaults[key] = def
		dto.Prompts[key] = readFileOr(agent.PromptFilePath(wsDir, key), def)
		if spec.OwnedBySystemKey != "" {
			if systemAgent, _, err := wsp.Runtime.ResolveSystemAgent(spec.OwnedBySystemKey); err == nil {
				dto.Prompts[key] = systemAgent.Soul
			}
		}
		dto.PromptMeta[key] = promptMetaTO{
			Label:            spec.Label,
			Hint:             spec.Hint,
			Placeholders:     spec.Placeholders,
			EpochAffecting:   spec.EpochAffecting,
			OwnedBySystemKey: spec.OwnedBySystemKey,
		}
	}
	return dto
}

// handleGetWorkspaceConfig returns the active workspace's editable config files.
func (s *Server) handleGetWorkspaceConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, buildWSConfigDTO(ws(r)))
}

// handleUpdateWorkspaceConfig writes the changed config files. Prompts and the
// README are written directly; instructions route through the workspace manager
// so ws-settings.json, the live runtime and the file all stay in sync.
func (s *Server) handleUpdateWorkspaceConfig(w http.ResponseWriter, r *http.Request) {
	patch, ok := bindJSON[wsConfigPatch](w, r)
	if !ok {
		return
	}
	wsDir := ws(r).DataDir

	if len(patch.Prompts) > 0 {
		valid := map[string]bool{}
		for _, k := range agent.PromptKeys {
			valid[k] = true
		}
		if err := os.MkdirAll(filepath.Join(agent.WorkspaceConfigDir(wsDir), "prompts"), 0o755); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for key, content := range patch.Prompts {
			if !valid[key] {
				writeError(w, http.StatusBadRequest, "unknown prompt key: "+key)
				return
			}
			if err := os.WriteFile(agent.PromptFilePath(wsDir, key), []byte(content), 0o644); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}

	if patch.Readme != nil {
		if err := os.MkdirAll(agent.WorkspaceConfigDir(wsDir), 0o755); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := os.WriteFile(agent.ReadmeFilePath(wsDir), []byte(*patch.Readme), 0o644); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if patch.Instructions != nil {
		// Routes through the manager: writes the file + ws-settings + live runtime.
		if _, err := s.workspaces.UpdateSettings(ws(r).ID, workspace.WSSettingsPatch{Instructions: patch.Instructions}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, buildWSConfigDTO(ws(r)))
}
