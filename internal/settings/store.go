package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// fileName is the settings document inside the data directory.
const fileName = "settings.json"

// Cipher seals/opens the sensitive API key. config.Secret satisfies it; the
// interface keeps this package free of a config import.
type Cipher interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(encoded string) (string, error)
}

// Store is the thread-safe, file-backed holder of application settings.
type Store struct {
	path   string
	cipher Cipher

	mu  sync.RWMutex
	cur Settings
}

// Open loads settings.json from dataDir, creating it with defaults if absent.
// Missing individual fields (e.g. after a schema addition) fall back to their
// default values.
func Open(dataDir string, cipher Cipher) (*Store, error) {
	s := &Store{
		path:   filepath.Join(dataDir, fileName),
		cipher: cipher,
		cur:    Default(),
	}

	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		if err := s.persist(s.cur); err != nil {
			return nil, err
		}
		return s, nil
	}
	if err != nil {
		return nil, err
	}

	// Start from defaults so newly added fields are populated, then overlay the
	// persisted document.
	loaded := Default()
	if err := json.Unmarshal(data, &loaded); err != nil {
		return nil, err
	}
	s.cur = normalize(loaded)
	return s, nil
}

// Get returns a copy of the current settings (including the encrypted secret).
func (s *Store) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// DTO returns the masked, client-facing view.
func (s *Store) DTO() DTO {
	return s.Get().ToDTO()
}

// AnthropicKey returns the decrypted API key, or "" if none/undecryptable.
func (s *Store) AnthropicKey() string {
	return s.decrypt(s.Get().AnthropicKeyEnc)
}

// MinimaxKey returns the decrypted MiniMax API key, or "" if none.
func (s *Store) MinimaxKey() string {
	return s.decrypt(s.Get().MinimaxKeyEnc)
}

// decrypt opens an encrypted value, returning "" on empty/failure.
func (s *Store) decrypt(enc string) string {
	if enc == "" {
		return ""
	}
	plain, err := s.cipher.Decrypt(enc)
	if err != nil {
		return ""
	}
	return plain
}

// Apply merges a patch into the current settings, persists, and returns the new
// state. The write-only AnthropicKey field is encrypted (or cleared) here.
func (s *Store) Apply(p Patch) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := s.cur
	applyString(&next.Theme, p.Theme)
	applyString(&next.Accent, p.Accent)
	applyString(&next.ThemePreset, p.ThemePreset)
	applyString(&next.Language, p.Language)

	applyString(&next.DefaultProvider, p.DefaultProvider)
	applyString(&next.DefaultModel, p.DefaultModel)
	applyString(&next.DefaultPermissionMode, p.DefaultPermissionMode)
	applyString(&next.ClaudeCLIPath, p.ClaudeCLIPath)

	applyBool(&next.OneMillionContext, p.OneMillionContext)
	applyBool(&next.ExtendedPromptCache, p.ExtendedPromptCache)
	applyBool(&next.DesktopNotifications, p.DesktopNotifications)
	applyBool(&next.KeepAwake, p.KeepAwake)

	applyString(&next.UserName, p.UserName)
	applyString(&next.UserTimezone, p.UserTimezone)
	applyString(&next.UserCity, p.UserCity)
	applyString(&next.UserCountry, p.UserCountry)
	applyString(&next.UserNotes, p.UserNotes)

	applyInt(&next.MaxContextTokens, p.MaxContextTokens)
	applyInt(&next.KeepRecentMsgs, p.KeepRecentMsgs)
	applyInt(&next.RecallTopN, p.RecallTopN)
	if p.RecallMinScore != nil {
		next.RecallMinScore = *p.RecallMinScore
	}
	applyInt(&next.JournalCap, p.JournalCap)
	applyInt(&next.JournalMaxLen, p.JournalMaxLen)
	if p.AutoReflect != nil {
		next.AutoReflect = *p.AutoReflect
	}
	applyInt(&next.AutoReflectThreshold, p.AutoReflectThreshold)

	applyInt(&next.DefaultDailyCallLimit, p.DefaultDailyCallLimit)
	applyInt(&next.DefaultDailyTokenLimit, p.DefaultDailyTokenLimit)

	applyInt(&next.DefaultHeartbeatSec, p.DefaultHeartbeatSec)
	if p.PauseAutonomy != nil {
		next.PauseAutonomy = *p.PauseAutonomy
	}

	if p.AutoTitleEnabled != nil {
		next.AutoTitleEnabled = *p.AutoTitleEnabled
	}
	applyString(&next.TitleModel, p.TitleModel)

	applyString(&next.MCPGatewayURL, p.MCPGatewayURL)
	applyString(&next.LogLevel, p.LogLevel)

	applyString(&next.MinimaxBaseURL, p.MinimaxBaseURL)

	// Secrets: write-only. Empty string clears; non-empty encrypts and replaces.
	if p.AnthropicKey != nil {
		if *p.AnthropicKey == "" {
			next.AnthropicKeyEnc = ""
		} else {
			enc, err := s.cipher.Encrypt(*p.AnthropicKey)
			if err != nil {
				return Settings{}, err
			}
			next.AnthropicKeyEnc = enc
		}
	}
	if p.MinimaxKey != nil {
		if *p.MinimaxKey == "" {
			next.MinimaxKeyEnc = ""
		} else {
			enc, err := s.cipher.Encrypt(*p.MinimaxKey)
			if err != nil {
				return Settings{}, err
			}
			next.MinimaxKeyEnc = enc
		}
	}

	next = normalize(next)
	if err := s.persist(next); err != nil {
		return Settings{}, err
	}
	s.cur = next
	return next, nil
}

// persist writes settings to disk atomically (temp file + rename).
func (s *Store) persist(v Settings) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// normalize clamps numeric fields to safe ranges and fills empty enums.
func normalize(v Settings) Settings {
	if v.Theme != ThemeLight && v.Theme != ThemeSystem {
		v.Theme = ThemeDark
	}
	if v.Accent == "" {
		v.Accent = "#8b5cf6"
	}
	if v.Language != "en" {
		v.Language = "tr"
	}
	if v.DefaultProvider != "anthropic" {
		v.DefaultProvider = "claude-cli"
	}
	if v.MaxContextTokens < 500 {
		v.MaxContextTokens = 500
	}
	if v.KeepRecentMsgs < 1 {
		v.KeepRecentMsgs = 1
	}
	if v.RecallTopN < 0 {
		v.RecallTopN = 0
	}
	if v.RecallMinScore < 0 {
		v.RecallMinScore = 0
	}
	if v.RecallMinScore > 1 {
		v.RecallMinScore = 1
	}
	// Journal bounds: keep at least a small buffer; clamp to sane ceilings.
	if v.JournalCap < 1 {
		v.JournalCap = 1
	}
	if v.JournalCap > 1000 {
		v.JournalCap = 1000
	}
	if v.JournalMaxLen < 64 {
		v.JournalMaxLen = 64
	}
	if v.JournalMaxLen > 65536 {
		v.JournalMaxLen = 65536
	}
	// Auto-reflect threshold: at least 2 entries to summarize; cap at 1000.
	if v.AutoReflectThreshold < 2 {
		v.AutoReflectThreshold = 2
	}
	if v.AutoReflectThreshold > 1000 {
		v.AutoReflectThreshold = 1000
	}
	if v.DefaultDailyCallLimit < 0 {
		v.DefaultDailyCallLimit = 0
	}
	if v.DefaultDailyTokenLimit < 0 {
		v.DefaultDailyTokenLimit = 0
	}
	if v.DefaultHeartbeatSec < 5 {
		v.DefaultHeartbeatSec = 5
	}
	switch v.LogLevel {
	case "debug", "warn", "error", "info":
	default:
		v.LogLevel = "info"
	}
	return v
}

func applyString(dst *string, src *string) {
	if src != nil {
		*dst = *src
	}
}

func applyInt(dst *int, src *int) {
	if src != nil {
		*dst = *src
	}
}

func applyBool(dst *bool, src *bool) {
	if src != nil {
		*dst = *src
	}
}
