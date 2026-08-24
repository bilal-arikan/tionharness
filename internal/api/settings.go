package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/settings"
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

// publishWorkspacesChanged notifies open UIs that the set of workspaces changed
// (an agent created/renamed/deleted one) so the switcher refreshes its list
// without a manual reload. App-global → no workspace badge, no toast.
func (s *Server) publishWorkspacesChanged(body string) {
	s.bus.Publish(events.Event{
		Type:  "workspaces",
		Level: "info",
		Title: "Workspace listesi güncellendi",
		Body:  body,
	})
}

// languageName maps a settings language code to a human name for the reply-language
// directive. Empty for an unknown code (so no directive is emitted).
func languageName(code string) string {
	switch code {
	case "tr":
		return "Turkish (Türkçe)"
	case "en":
		return "English"
	}
	return ""
}

// userContextBlock renders the user-profile settings AND the configured reply
// language into a system-prompt block so agents address the user correctly and
// default to their language. Empty only when there is no profile and no known
// language (language defaults to "tr", so it is normally always present).
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
	// Preferred reply language (the app's configured language). Emitted even when no
	// other profile field is set, so the agent defaults to the user's language.
	if lang := languageName(s.Language); lang != "" {
		b.WriteString("- Preferred language: reply in " + lang + " by default, unless the user writes to you in another language or asks otherwise.\n")
	}
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
	patch, ok := bindJSONStrict[settings.Patch](w, r)
	if !ok {
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
	// Model is the model id to probe with. Optional: when empty the configured
	// default model is used (correct for the default provider). The per-provider
	// test buttons pass a representative model id, since the global default model
	// is usually for a different provider (e.g. an Anthropic id can't probe
	// MiniMax/OpenRouter).
	Model string `json:"model"`
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
	req, ok := bindJSONStrict[testProviderReq](w, r)
	if !ok {
		return
	}

	provider, err := s.providers.Get(req.Provider)
	if err != nil {
		writeJSON(w, http.StatusOK, testProviderResp{OK: false, Error: err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Empty model → the provider applies its own built-in default model. There is
	// no app-global default-model setting to fall back to any more.
	resp, err := provider.Complete(ctx, providers.Request{
		Model:  req.Model,
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
