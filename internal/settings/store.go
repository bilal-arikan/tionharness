package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// reservedProviderIDs are the built-in provider ids a custom provider must not
// shadow.
var reservedProviderIDs = map[string]bool{
	"anthropic":         true,
	"minimax":           true,
	"claude-cli":        true,
	"minimax-anthropic": true,
	"openrouter":        true,
}

// providerIDRe constrains a custom provider id to a clean identifier.
var providerIDRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)

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

// Path returns the absolute path of the settings.json document on disk. Exposed
// so tools can report exactly which file backs the live application settings.
func (s *Store) Path() string { return s.path }

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

// ClaudeCliAuthToken returns the decrypted claude-cli credential (OAuth token or
// API key, per ClaudeCliAuthKind), or "" if none/undecryptable.
func (s *Store) ClaudeCliAuthToken() string {
	return s.decrypt(s.Get().ClaudeCliAuthTokenEnc)
}

// MinimaxKey returns the decrypted MiniMax API key, or "" if none.
func (s *Store) MinimaxKey() string {
	return s.decrypt(s.Get().MinimaxKeyEnc)
}

// OpenRouterKey returns the decrypted OpenRouter API key, or "" if none.
func (s *Store) OpenRouterKey() string {
	return s.decrypt(s.Get().OpenRouterKeyEnc)
}

// CustomProviderKey returns the decrypted key for a custom provider id, or "".
func (s *Store) CustomProviderKey(id string) string {
	s.mu.RLock()
	var enc string
	for _, c := range s.cur.CustomProviders {
		if c.ID == id {
			enc = c.KeyEnc
			break
		}
	}
	s.mu.RUnlock()
	return s.decrypt(enc)
}

// UpsertCustomProvider creates or updates a custom provider by id. The key is
// plaintext and write-only: nil keeps the stored key, "" clears it, a non-empty
// value is encrypted and replaces it. Returns the new settings on success.
func (s *Store) UpsertCustomProvider(p CustomProvider, key *string) (Settings, error) {
	id := strings.TrimSpace(p.ID)
	if !providerIDRe.MatchString(id) {
		return Settings{}, fmt.Errorf("invalid provider id (letters, digits, _, -, . — start with a letter)")
	}
	if reservedProviderIDs[id] {
		return Settings{}, fmt.Errorf("provider id %q is reserved", id)
	}
	if p.Kind != "openai" && p.Kind != "anthropic" {
		return Settings{}, fmt.Errorf("kind must be \"openai\" or \"anthropic\"")
	}
	if strings.TrimSpace(p.BaseURL) == "" {
		return Settings{}, fmt.Errorf("base URL is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cur
	list := make([]CustomProvider, len(next.CustomProviders))
	copy(list, next.CustomProviders)
	idx := -1
	for i, c := range list {
		if c.ID == id {
			idx = i
			break
		}
	}
	entry := CustomProvider{
		ID:           id,
		Label:        strings.TrimSpace(p.Label),
		Kind:         p.Kind,
		BaseURL:      strings.TrimSpace(p.BaseURL),
		DefaultModel: strings.TrimSpace(p.DefaultModel),
		Models:       p.Models,
		Reasoning:    p.Reasoning,
		PromptCache:  strings.TrimSpace(p.PromptCache),
	}
	if entry.Label == "" {
		entry.Label = id
	}
	if idx >= 0 {
		entry.KeyEnc = list[idx].KeyEnc // preserve unless changed below
	}
	if key != nil {
		if *key == "" {
			entry.KeyEnc = ""
		} else {
			enc, err := s.cipher.Encrypt(*key)
			if err != nil {
				return Settings{}, err
			}
			entry.KeyEnc = enc
		}
	}
	if idx >= 0 {
		list[idx] = entry
	} else {
		list = append(list, entry)
	}
	next.CustomProviders = list
	next = normalize(next)
	if err := s.persist(next); err != nil {
		return Settings{}, err
	}
	s.cur = next
	return next, nil
}

// DeleteCustomProvider removes a custom provider by id.
func (s *Store) DeleteCustomProvider(id string) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cur
	list := make([]CustomProvider, 0, len(next.CustomProviders))
	found := false
	for _, c := range next.CustomProviders {
		if c.ID == id {
			found = true
			continue
		}
		list = append(list, c)
	}
	if !found {
		return Settings{}, fmt.Errorf("custom provider %q not found", id)
	}
	next.CustomProviders = list
	if err := s.persist(next); err != nil {
		return Settings{}, err
	}
	s.cur = next
	return next, nil
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
// Invalid enum/format values are rejected up front (see Validate) so a bad
// change never reaches the live subsystems; numeric fields are clamped by
// normalize rather than refused.
func (s *Store) Apply(p Patch) (Settings, error) {
	if err := Validate(p); err != nil {
		return Settings{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	next := s.cur
	applyString(&next.Theme, p.Theme)
	applyString(&next.Accent, p.Accent)
	applyString(&next.ThemePreset, p.ThemePreset)
	applyString(&next.Language, p.Language)

	applyString(&next.DefaultPermissionMode, p.DefaultPermissionMode)
	applyString(&next.ClaudeCLIPath, p.ClaudeCLIPath)
	applyString(&next.ClaudeConfigDir, p.ClaudeConfigDir)
	applyString(&next.ClaudeCliAuthKind, p.ClaudeCliAuthKind)

	applyBool(&next.ExtendedPromptCache, p.ExtendedPromptCache)
	applyBool(&next.AnthropicContextEditing, p.AnthropicContextEditing)
	applyBool(&next.AnthropicNativeToolSearch, p.AnthropicNativeToolSearch)
	applyBool(&next.AnthropicProgrammaticTools, p.AnthropicProgrammaticTools)
	applyBool(&next.AnthropicWebTools, p.AnthropicWebTools)
	applyBool(&next.AnthropicServerCompaction, p.AnthropicServerCompaction)
	applyBool(&next.AnthropicRefusalFallback, p.AnthropicRefusalFallback)
	applyInt(&next.AutonomousTaskBudgetTokens, p.AutonomousTaskBudgetTokens)
	applyBool(&next.DesktopNotifications, p.DesktopNotifications)
	applyBool(&next.KeepAwake, p.KeepAwake)

	applyString(&next.UserName, p.UserName)
	applyString(&next.UserTimezone, p.UserTimezone)
	applyString(&next.UserCity, p.UserCity)
	applyString(&next.UserCountry, p.UserCountry)
	applyString(&next.UserNotes, p.UserNotes)

	applyInt(&next.MaxContextTokens, p.MaxContextTokens)
	applyInt(&next.KeepRecentMsgs, p.KeepRecentMsgs)
	applyInt(&next.ContextBudgetCeil, p.ContextBudgetCeil)
	if p.ContextBudgetFraction != nil {
		next.ContextBudgetFraction = *p.ContextBudgetFraction
	}
	if p.HandoffAuto != nil {
		next.HandoffAuto = *p.HandoffAuto
	}
	if p.HandoffPressure != nil {
		next.HandoffPressure = *p.HandoffPressure
	}
	applyInt(&next.HandoffMaxChain, p.HandoffMaxChain)
	if p.HandoffWriteFile != nil {
		next.HandoffWriteFile = *p.HandoffWriteFile
	}
	if p.ProgressPersist != nil {
		next.ProgressPersist = *p.ProgressPersist
	}
	if p.ProgressResume != nil {
		next.ProgressResume = *p.ProgressResume
	}
	if p.AutonomousAutoContinue != nil {
		next.AutonomousAutoContinue = *p.AutonomousAutoContinue
	}
	applyInt(&next.AutonomousAutoContinueMax, p.AutonomousAutoContinueMax)
	if p.FileFreshnessGuard != nil {
		next.FileFreshnessGuard = *p.FileFreshnessGuard
	}
	if p.AutoTagSessions != nil {
		next.AutoTagSessions = *p.AutoTagSessions
	}
	if p.DebugJournalEnabled != nil {
		next.DebugJournalEnabled = *p.DebugJournalEnabled
	}
	applyInt(&next.DebugJournalCap, p.DebugJournalCap)

	if p.ReactiveCompact != nil {
		next.ReactiveCompact = *p.ReactiveCompact
	}
	applyInt(&next.MaxTokenRetries, p.MaxTokenRetries)
	applyInt(&next.ReactiveKeepRecent, p.ReactiveKeepRecent)
	applyInt(&next.MaxProviderRetries, p.MaxProviderRetries)
	if p.ToolGuardWarnings != nil {
		next.ToolGuardWarnings = *p.ToolGuardWarnings
	}
	if p.ToolGuardHardStop != nil {
		next.ToolGuardHardStop = *p.ToolGuardHardStop
	}
	applyInt(&next.GuardExactWarn, p.GuardExactWarn)
	applyInt(&next.GuardExactBlock, p.GuardExactBlock)
	applyInt(&next.GuardSameToolWarn, p.GuardSameToolWarn)
	applyInt(&next.GuardSameToolHalt, p.GuardSameToolHalt)
	applyInt(&next.GuardNoProgressWarn, p.GuardNoProgressWarn)
	applyInt(&next.GuardNoProgressBlck, p.GuardNoProgressBlck)
	applyInt(&next.StuckTurnThreshold, p.StuckTurnThreshold)
	if p.LessonReflect != nil {
		next.LessonReflect = *p.LessonReflect
	}
	applyInt(&next.MaxOutputTokens, p.MaxOutputTokens)

	if p.AutoTitleEnabled != nil {
		next.AutoTitleEnabled = *p.AutoTitleEnabled
	}
	applyString(&next.TitleModel, p.TitleModel)

	applyBool(&next.EnableShell, p.EnableShell)
	applyBool(&next.EnableCLIHooks, p.EnableCLIHooks)
	applyBool(&next.EnableCodeMode, p.EnableCodeMode)
	applyBool(&next.ClaudeResume, p.ClaudeResume)
	applyBool(&next.ClaudePersistentSession, p.ClaudePersistentSession)
	applyBool(&next.ClaudeSysPromptFile, p.ClaudeSysPromptFile)
	applyInt(&next.DelegationMaxDepth, p.DelegationMaxDepth)
	applyInt(&next.DelegationMaxCalls, p.DelegationMaxCalls)
	applyInt(&next.SpawnMaxConcurrent, p.SpawnMaxConcurrent)
	applyInt(&next.SpawnMaxPerTurn, p.SpawnMaxPerTurn)
	applyInt(&next.CoordinatorMaxWorkers, p.CoordinatorMaxWorkers)
	applyInt(&next.CoordinatorMaxTurns, p.CoordinatorMaxTurns)

	applyBool(&next.AutonomousConfine, p.AutonomousConfine)
	applyBool(&next.GitWorktreeIsolation, p.GitWorktreeIsolation)
	applyBool(&next.AutonomousBootSeq, p.AutonomousBootSeq)

	applyBool(&next.BackupEnabled, p.BackupEnabled)
	applyInt(&next.BackupIntervalHours, p.BackupIntervalHours)
	applyInt(&next.BackupRetain, p.BackupRetain)
	if p.BackupDir != nil {
		next.BackupDir = strings.TrimSpace(*p.BackupDir)
	}

	applyString(&next.MinimaxBaseURL, p.MinimaxBaseURL)
	applyString(&next.OpenRouterBaseURL, p.OpenRouterBaseURL)

	// Secrets: write-only. Empty string clears; non-empty encrypts and replaces.
	if p.ClaudeCliAuthToken != nil {
		if *p.ClaudeCliAuthToken == "" {
			next.ClaudeCliAuthTokenEnc = ""
		} else {
			enc, err := s.cipher.Encrypt(*p.ClaudeCliAuthToken)
			if err != nil {
				return Settings{}, err
			}
			next.ClaudeCliAuthTokenEnc = enc
		}
	}
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
	if p.OpenRouterKey != nil {
		if *p.OpenRouterKey == "" {
			next.OpenRouterKeyEnc = ""
		} else {
			enc, err := s.cipher.Encrypt(*p.OpenRouterKey)
			if err != nil {
				return Settings{}, err
			}
			next.OpenRouterKeyEnc = enc
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
	// Accent must be a valid hex color; coerce anything else (incl. empty or a
	// hand-edited bad file) to the default so the UI never gets broken CSS.
	if !isHexColor(v.Accent) {
		v.Accent = "#8b5cf6"
	}
	if v.Language != "en" {
		v.Language = "tr"
	}
	// Permission mode seeds new agents; an unknown value falls back to "auto".
	switch v.DefaultPermissionMode {
	case "read-only", "ask", "auto":
	default:
		v.DefaultPermissionMode = "auto"
	}
	if v.MaxContextTokens < 500 {
		v.MaxContextTokens = 500
	}
	if v.KeepRecentMsgs < 1 {
		v.KeepRecentMsgs = 1
	}
	// Model-aware transcript budget. Ceil 0 → restore the default (a blank field
	// must not disable big-window budgeting); clamp to a sane band. Fraction is a
	// window share in [0,1]: 0 = "auto" (per-family adaptive, preserved), >0 = fixed.
	if v.ContextBudgetCeil <= 0 {
		v.ContextBudgetCeil = 262144
	}
	if v.ContextBudgetCeil < 8000 {
		v.ContextBudgetCeil = 8000
	}
	if v.ContextBudgetCeil > 2000000 {
		v.ContextBudgetCeil = 2000000
	}
	if v.ContextBudgetFraction < 0 {
		v.ContextBudgetFraction = 0 // negative is invalid → auto
	}
	if v.ContextBudgetFraction > 1 {
		v.ContextBudgetFraction = 1
	}
	// Handoff pressure ratio: 0 selects the default; otherwise clamp to a sane band
	// (well above the memory-pressure warning, below a full window).
	if v.HandoffPressure != 0 {
		if v.HandoffPressure < 0.5 {
			v.HandoffPressure = 0.5
		}
		if v.HandoffPressure > 0.99 {
			v.HandoffPressure = 0.99
		}
	}
	// Handoff chain depth cap: 0 selects the default; otherwise clamp to [1,100].
	if v.HandoffMaxChain != 0 {
		if v.HandoffMaxChain < 1 {
			v.HandoffMaxChain = 1
		}
		if v.HandoffMaxChain > 100 {
			v.HandoffMaxChain = 100
		}
	}
	// Turn recovery (A1): 0 resume attempts is valid (disables resume); clamp the
	// ceiling. Compaction tail needs ≥2 to guarantee a safe fold boundary.
	if v.MaxTokenRetries < 0 {
		v.MaxTokenRetries = 0
	}
	if v.MaxTokenRetries > 10 {
		v.MaxTokenRetries = 10
	}
	if v.ReactiveKeepRecent < 2 {
		v.ReactiveKeepRecent = 2
	}
	if v.ReactiveKeepRecent > 50 {
		v.ReactiveKeepRecent = 50
	}
	// Provider retry: 0 is valid (disables retry); clamp the ceiling so a typo
	// cannot make one turn hammer a failing provider.
	if v.MaxProviderRetries < 0 {
		v.MaxProviderRetries = 0
	}
	if v.MaxProviderRetries > 5 {
		v.MaxProviderRetries = 5
	}
	// Guardrail thresholds: 0 selects the built-in default; clamp negatives to 0
	// and cap so a typo cannot effectively disable the circuit breaker.
	clampGuard := func(v *int) {
		if *v < 0 {
			*v = 0
		}
		if *v > 50 {
			*v = 50
		}
	}
	clampGuard(&v.GuardExactWarn)
	clampGuard(&v.GuardExactBlock)
	clampGuard(&v.GuardSameToolWarn)
	clampGuard(&v.GuardSameToolHalt)
	clampGuard(&v.GuardNoProgressWarn)
	clampGuard(&v.GuardNoProgressBlck)
	// Stuck gate: 0 is valid (disables the gate); clamp the ceiling.
	if v.StuckTurnThreshold < 0 {
		v.StuckTurnThreshold = 0
	}
	if v.StuckTurnThreshold > 20 {
		v.StuckTurnThreshold = 20
	}
	// Output cap: 0 is valid (auto, per-model family). A positive override is
	// clamped to a sane range — a floor so it can't cripple answers, a ceiling at
	// the largest known model output (MiniMax-M3 ≈ 512K).
	if v.MaxOutputTokens < 0 {
		v.MaxOutputTokens = 0
	}
	if v.MaxOutputTokens > 0 && v.MaxOutputTokens < 256 {
		v.MaxOutputTokens = 256
	}
	if v.MaxOutputTokens > 512000 {
		v.MaxOutputTokens = 512000
	}
	// Delegation guards: keep at least one level/call; clamp to sane ceilings.
	if v.DelegationMaxDepth < 1 {
		v.DelegationMaxDepth = 1
	}
	if v.DelegationMaxDepth > 10 {
		v.DelegationMaxDepth = 10
	}
	if v.DelegationMaxCalls < 1 {
		v.DelegationMaxCalls = 1
	}
	if v.DelegationMaxCalls > 100 {
		v.DelegationMaxCalls = 100
	}
	// Spawn guards: keep at least one; clamp to sane ceilings.
	if v.SpawnMaxConcurrent < 1 {
		v.SpawnMaxConcurrent = 1
	}
	if v.SpawnMaxConcurrent > 128 {
		v.SpawnMaxConcurrent = 128
	}
	if v.SpawnMaxPerTurn < 1 {
		v.SpawnMaxPerTurn = 1
	}
	if v.SpawnMaxPerTurn > 64 {
		v.SpawnMaxPerTurn = 64
	}
	// Coordinator guards: workers ≥ 1 (≤ 64), auto-turns ≥ 1 (≤ 500).
	if v.CoordinatorMaxWorkers < 1 {
		v.CoordinatorMaxWorkers = 1
	}
	if v.CoordinatorMaxWorkers > 64 {
		v.CoordinatorMaxWorkers = 64
	}
	if v.CoordinatorMaxTurns < 1 {
		v.CoordinatorMaxTurns = 1
	}
	if v.CoordinatorMaxTurns > 500 {
		v.CoordinatorMaxTurns = 500
	}
	// Workspace backups: interval ≥ 1h, retention ≥ 1 archive; clamp ceilings.
	if v.BackupIntervalHours < 1 {
		v.BackupIntervalHours = 1
	}
	if v.BackupIntervalHours > 8760 { // one year
		v.BackupIntervalHours = 8760
	}
	if v.BackupRetain < 1 {
		v.BackupRetain = 1
	}
	if v.BackupRetain > 1000 {
		v.BackupRetain = 1000
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
