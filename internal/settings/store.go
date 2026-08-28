package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// Decrypt is the exported form of decrypt, for callers outside this package
// that need to open a Settings *Enc field's plaintext (e.g. ProviderStore's
// boot migration, _Docs/71 §3).
func (s *Store) Decrypt(enc string) string { return s.decrypt(enc) }

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
	applyString(&next.UILanguage, p.UILanguage)

	applyString(&next.DefaultPermissionMode, p.DefaultPermissionMode)
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
	if p.LessonMaxAgeDays != nil {
		next.LessonMaxAgeDays = *p.LessonMaxAgeDays
	}
	applyInt(&next.MaxOutputTokens, p.MaxOutputTokens)

	if p.AutoTitleEnabled != nil {
		next.AutoTitleEnabled = *p.AutoTitleEnabled
	}

	applyBool(&next.EnableShell, p.EnableShell)
	applyBool(&next.EnableCLIHooks, p.EnableCLIHooks)
	applyBool(&next.EnableCodeMode, p.EnableCodeMode)
	applyBool(&next.ClaudeResume, p.ClaudeResume)
	applyBool(&next.ClaudePersistentSession, p.ClaudePersistentSession)
	applyBool(&next.ClaudeSysPromptFile, p.ClaudeSysPromptFile)
	applyInt(&next.DelegationMaxDepth, p.DelegationMaxDepth)
	applyInt(&next.DelegationMaxCalls, p.DelegationMaxCalls)
	applyInt(&next.SpawnMaxConcurrent, p.SpawnMaxConcurrent)
	applyInt(&next.SpawnQueueMax, p.SpawnQueueMax)
	applyInt(&next.SpawnMaxPerTurn, p.SpawnMaxPerTurn)
	applyInt(&next.SpawnTimeoutMin, p.SpawnTimeoutMin)
	applyInt(&next.SpawnIdleTimeoutMin, p.SpawnIdleTimeoutMin)
	applyInt(&next.ChatTurnTimeoutMin, p.ChatTurnTimeoutMin)
	applyInt(&next.ChatTurnIdleTimeoutMin, p.ChatTurnIdleTimeoutMin)
	applyInt(&next.CodexStdoutIdleMin, p.CodexStdoutIdleMin)
	applyInt(&next.IdleResumeMax, p.IdleResumeMax)
	applyInt(&next.ScheduleTimeoutMin, p.ScheduleTimeoutMin)
	applyInt(&next.TurnWatchdogMin, p.TurnWatchdogMin)
	applyInt(&next.TurnIdleWatchdogMin, p.TurnIdleWatchdogMin)
	applyInt(&next.ShellDefaultTimeoutSec, p.ShellDefaultTimeoutSec)
	applyInt(&next.ShellMaxTimeoutSec, p.ShellMaxTimeoutSec)
	applyInt(&next.MaxToolOutputKB, p.MaxToolOutputKB)
	applyInt(&next.AgentMessageMaxKB, p.AgentMessageMaxKB)
	applyInt(&next.CoordinatorMaxWorkers, p.CoordinatorMaxWorkers)
	applyInt(&next.CoordinatorMaxTurns, p.CoordinatorMaxTurns)
	applyInt(&next.CoordinatorMaxDepth, p.CoordinatorMaxDepth)
	applyInt(&next.CoordinatorMaxSubtreeSessions, p.CoordinatorMaxSubtreeSessions)
	applyInt(&next.CoordinatorSettleGraceSec, p.CoordinatorSettleGraceSec)
	applyBool(&next.CoordinatorStallGuard, p.CoordinatorStallGuard)
	applyInt(&next.CoordinatorStallSweepMin, p.CoordinatorStallSweepMin)
	applyInt(&next.CoordinatorStallMaxNudges, p.CoordinatorStallMaxNudges)

	applyBool(&next.AutonomousConfine, p.AutonomousConfine)
	applyBool(&next.AutonomousBootSeq, p.AutonomousBootSeq)

	applyBool(&next.BackupEnabled, p.BackupEnabled)
	applyInt(&next.BackupIntervalHours, p.BackupIntervalHours)
	applyInt(&next.BackupRetain, p.BackupRetain)
	if p.BackupDir != nil {
		next.BackupDir = strings.TrimSpace(*p.BackupDir)
	}

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
	if !isSupportedLanguage(v.Language) {
		v.Language = DefaultLanguage
	}
	// UILanguage keeps "" (= follow Language); only an unknown non-empty code is
	// coerced away, so a hand-edited file can never wedge the UI on a missing catalog.
	if v.UILanguage != "" && !isSupportedLanguage(v.UILanguage) {
		v.UILanguage = ""
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
	if v.SpawnQueueMax < 1 {
		v.SpawnQueueMax = 1
	}
	if v.SpawnQueueMax > 128 {
		v.SpawnQueueMax = 128
	}
	if v.SpawnMaxPerTurn < 1 {
		v.SpawnMaxPerTurn = 1
	}
	if v.SpawnMaxPerTurn > 64 {
		v.SpawnMaxPerTurn = 64
	}
	// Turn deadlines (minutes): at least one minute, at most a day — same bounds the
	// settings UI enforces. The idle watchdog must stay BELOW the hard ceiling, else
	// it never fires and a hung turn burns the full wall clock.
	if v.SpawnTimeoutMin < 1 {
		v.SpawnTimeoutMin = 1
	}
	if v.SpawnTimeoutMin > 1440 {
		v.SpawnTimeoutMin = 1440
	}
	if v.SpawnIdleTimeoutMin < 1 {
		v.SpawnIdleTimeoutMin = 1
	}
	if v.SpawnIdleTimeoutMin > v.SpawnTimeoutMin {
		v.SpawnIdleTimeoutMin = v.SpawnTimeoutMin
	}
	// Interactive chat timeouts use 0 as an explicit disabled value.
	if v.ChatTurnTimeoutMin < 0 {
		v.ChatTurnTimeoutMin = Default().ChatTurnTimeoutMin
	}
	if v.ChatTurnTimeoutMin > 1440 {
		v.ChatTurnTimeoutMin = 1440
	}
	if v.ChatTurnIdleTimeoutMin < 0 {
		v.ChatTurnIdleTimeoutMin = Default().ChatTurnIdleTimeoutMin
	}
	if v.ChatTurnIdleTimeoutMin > 1440 {
		v.ChatTurnIdleTimeoutMin = 1440
	}
	if v.ChatTurnTimeoutMin > 0 && v.ChatTurnIdleTimeoutMin > v.ChatTurnTimeoutMin {
		v.ChatTurnIdleTimeoutMin = v.ChatTurnTimeoutMin
	}
	// Codex stdout-silence watchdog: 0 disables it entirely; a negative value is a
	// typo, not an intent, so it falls back to the default. The day-long ceiling
	// matches the other timeout knobs.
	if v.CodexStdoutIdleMin < 0 {
		v.CodexStdoutIdleMin = Default().CodexStdoutIdleMin
	}
	if v.CodexStdoutIdleMin > 1440 {
		v.CodexStdoutIdleMin = 1440
	}
	// Idle-resume budget: 0 disables it, cap at 5 so a persistently-idle turn cannot
	// chew through many full idle windows before it is finally reported unfinished.
	if v.IdleResumeMax < 0 {
		v.IdleResumeMax = 0
	}
	if v.IdleResumeMax > 5 {
		v.IdleResumeMax = 5
	}
	if v.ScheduleTimeoutMin < 1 {
		v.ScheduleTimeoutMin = 1
	}
	if v.ScheduleTimeoutMin > 1440 {
		v.ScheduleTimeoutMin = 1440
	}
	// Queued-turn watchdog: same day-long bound, but it must stay ABOVE the deadlines
	// above — it exists to break a wedged queue, not to cut a turn those knobs still
	// permit. Raising it here (rather than rejecting) keeps an inconsistent config
	// working; agent.Tunables.TurnWatchdog applies the same floor at read time.
	if v.TurnWatchdogMin < 1 {
		v.TurnWatchdogMin = 1
	}
	if v.TurnWatchdogMin < v.SpawnTimeoutMin {
		v.TurnWatchdogMin = v.SpawnTimeoutMin
	}
	if v.TurnWatchdogMin < v.ScheduleTimeoutMin {
		v.TurnWatchdogMin = v.ScheduleTimeoutMin
	}
	if v.TurnWatchdogMin > 1440 {
		v.TurnWatchdogMin = 1440
	}
	// The inactivity window must stay BELOW the hard ceiling, else it can never fire
	// and a wedged turn burns the full wall clock — the same ordering rule the
	// spawn idle watchdog above follows.
	if v.TurnIdleWatchdogMin < 1 {
		v.TurnIdleWatchdogMin = 1
	}
	if v.TurnIdleWatchdogMin > v.TurnWatchdogMin {
		v.TurnIdleWatchdogMin = v.TurnWatchdogMin
	}
	// Coordinator guards: workers ≥ 1 (≤ 64).
	if v.CoordinatorMaxWorkers < 1 {
		v.CoordinatorMaxWorkers = 1
	}
	if v.CoordinatorMaxWorkers > 64 {
		v.CoordinatorMaxWorkers = 64
	}
	// Auto-turns and the tree-wide worker budget are no longer user-configurable:
	// the settings UI dropped both inputs, and the product decision is that they
	// stay unlimited (-1). normalize runs on every load and save, so this also
	// coerces any previously-persisted finite value in existing workspaces to
	// unlimited the next time their settings.json is read.
	v.CoordinatorMaxTurns = -1
	// -1 is meaningful here (explicitly unlimited), so only values below that are
	// clamped. The upper bounds are sanity ceilings, not policy: depth 12 with the
	// default 8 workers per node is already astronomically wide, and the subtree cap
	// is the guard that actually holds.
	if v.CoordinatorMaxDepth < -1 {
		v.CoordinatorMaxDepth = -1
	}
	if v.CoordinatorMaxDepth > 12 {
		v.CoordinatorMaxDepth = 12
	}
	// Tree-wide worker budget: welded unlimited alongside CoordinatorMaxTurns above
	// (UI input removed; -1 = unlimited).
	v.CoordinatorMaxSubtreeSessions = -1
	// A backstop firing in a couple of seconds would race every normal synthesis
	// turn and report "incomplete" over work that was about to finish; one that
	// waits an hour is not a backstop. 0 keeps the built-in default.
	if v.CoordinatorSettleGraceSec != 0 && v.CoordinatorSettleGraceSec < 5 {
		v.CoordinatorSettleGraceSec = 5
	}
	if v.CoordinatorSettleGraceSec > 1800 {
		v.CoordinatorSettleGraceSec = 1800
	}
	// Stall sweeper window: 0 = built-in default (5 min); any negative = sweeper off
	// (normalized to -1); positive is minutes with a 1-day sanity ceiling.
	if v.CoordinatorStallSweepMin < -1 {
		v.CoordinatorStallSweepMin = -1
	}
	if v.CoordinatorStallSweepMin > 1440 {
		v.CoordinatorStallSweepMin = 1440
	}
	// Stall nudge cap: 0 = default (2); no negatives; a small sanity ceiling — beyond
	// a handful the sweeper's escalation is the right tool, not more nagging.
	if v.CoordinatorStallMaxNudges < 0 {
		v.CoordinatorStallMaxNudges = 0
	}
	if v.CoordinatorStallMaxNudges > 10 {
		v.CoordinatorStallMaxNudges = 10
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
