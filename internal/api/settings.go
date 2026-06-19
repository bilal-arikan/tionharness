package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/bilal/swarmgo/internal/events"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/settings"
)

// publishSettingsChanged notifies open UIs (over the /api/events SSE feed) that
// the application-wide settings changed, so every window's Settings screen and
// live theme refresh without a manual reload. WorkspaceID is empty because
// settings are application-global. Body carries who made the change.
func (s *Server) publishSettingsChanged(body string) {
	s.bus.Publish(events.Event{
		Type:  "settings",
		Level: "info",
		Title: "Ayarlar güncellendi",
		Body:  body,
	})
}

// userContextBlock renders the user-profile settings into a system-prompt block
// so agents address the user correctly. Empty when no profile fields are set.
func userContextBlock(s settings.Settings) string {
	var b strings.Builder
	add := func(label, val string) {
		if strings.TrimSpace(val) != "" {
			b.WriteString("- ")
			b.WriteString(label)
			b.WriteString(": ")
			b.WriteString(strings.TrimSpace(val))
			b.WriteString("\n")
		}
	}
	add("Name", s.UserName)
	loc := strings.TrimSpace(strings.Trim(strings.TrimSpace(s.UserCity)+", "+strings.TrimSpace(s.UserCountry), ", "))
	add("Location", loc)
	add("Timezone", s.UserTimezone)
	add("Notes", s.UserNotes)
	if b.Len() == 0 {
		return ""
	}
	return "## About the user\n" + strings.TrimRight(b.String(), "\n")
}

// handleGetSettings returns the masked, client-facing settings document.
func (s *Server) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.settings.DTO())
}

// handleUpdateSettings merges a partial patch, persists it, and re-applies the
// settings to every live subsystem.
func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var patch settings.Patch
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	// Reject invalid enum/format values with a 400 (client error) so the UI shows
	// a clear message instead of a generic 500. Apply re-validates defensively.
	if err := settings.Validate(patch); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.settings.Apply(patch); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.applySettings()
	// Sync other open windows (the editing window already has the new state).
	s.publishSettingsChanged("Uygulama ayarları güncellendi.")
	writeJSON(w, http.StatusOK, s.settings.DTO())
}

type testProviderReq struct {
	Provider string `json:"provider"`
}

type testProviderResp struct {
	OK     bool   `json:"ok"`
	Model  string `json:"model,omitempty"`
	Sample string `json:"sample,omitempty"`
	Error  string `json:"error,omitempty"`
}

// handleTestProvider performs a minimal live completion to verify a provider is
// configured and reachable (e.g. claude-cli logged in, Anthropic key valid).
func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	var req testProviderReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	provider, err := s.providers.Get(req.Provider)
	if err != nil {
		writeJSON(w, http.StatusOK, testProviderResp{OK: false, Error: err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, providers.Request{
		Model:  s.settings.Get().DefaultModel,
		System: "You are a connectivity probe. Reply with exactly: OK",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: "ping"},
		},
	})
	if err != nil {
		writeJSON(w, http.StatusOK, testProviderResp{OK: false, Error: err.Error()})
		return
	}

	sample := resp.Text
	if len(sample) > 80 {
		sample = sample[:80]
	}
	writeJSON(w, http.StatusOK, testProviderResp{OK: true, Model: resp.Model, Sample: sample})
}
