package settings

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
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
// default values. A document that does not parse at all is QUARANTINED (renamed
// to settings.json.corrupt-<unix>) and replaced with defaults instead of failing
// the application start — see openCorrupt.
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
		if qerr := s.quarantine(err); qerr != nil {
			return nil, qerr
		}
		return s, nil
	}
	s.cur = normalize(loaded)
	return s, nil
}

// quarantine moves an unparseable settings.json aside and writes a fresh default
// document in its place. A syntax error in a hand-edited file used to make the
// application unstartable with no way to fix it from the UI — while normalize
// deliberately tolerates bad VALUES for exactly that reason. The event is logged
// at ERROR level with both paths so the user can see what happened and recover
// their old file. A failed rename IS returned: silently overwriting a file we
// could not preserve would destroy the only copy of the user's settings.
func (s *Store) quarantine(parseErr error) error {
	dest := s.path + ".corrupt-" + strconv.FormatInt(time.Now().Unix(), 10)
	if err := os.Rename(s.path, dest); err != nil {
		slog.Error("settings.json is corrupt and could not be quarantined",
			"component", "settings", "path", s.path, "parse_error", parseErr, "error", err)
		return parseErr
	}
	slog.Error("settings.json was corrupt: quarantined and replaced with defaults",
		"component", "settings", "path", s.path, "quarantine", dest, "parse_error", parseErr)
	s.cur = Default()
	return s.persist(s.cur)
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
	applyString(&next.AutoCompactMode, p.AutoCompactMode)
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

	applyBool(&next.EnableShell, p.EnableShell)
	applyBool(&next.EnableCLIHooks, p.EnableCLIHooks)
	applyBool(&next.EnableCodeMode, p.EnableCodeMode)
	applyBool(&next.ClaudeResume, p.ClaudeResume)
	applyBool(&next.ClaudePersistentSession, p.ClaudePersistentSession)
	applyBool(&next.ClaudeCLIToolAllowlist, p.ClaudeCLIToolAllowlist)
	applyBool(&next.ClaudeCLINativeSubagents, p.ClaudeCLINativeSubagents)
	applyBool(&next.ClaudeSysPromptFile, p.ClaudeSysPromptFile)
	applyInt(&next.DelegationMaxDepth, p.DelegationMaxDepth)
	applyInt(&next.DelegationMaxCalls, p.DelegationMaxCalls)
	applyInt(&next.SpawnMaxConcurrent, p.SpawnMaxConcurrent)
	applyInt(&next.SpawnQueueMax, p.SpawnQueueMax)
	applyInt(&next.SpawnMaxPerTurn, p.SpawnMaxPerTurn)
	applyInt(&next.FlowRunRetention, p.FlowRunRetention)
	applyInt(&next.TurnIdleTimeoutMin, p.TurnIdleTimeoutMin)
	applyInt(&next.CodexStdoutIdleSec, p.CodexStdoutIdleSec)
	applyInt(&next.IdleResumeMax, p.IdleResumeMax)
	applyInt(&next.ShellDefaultTimeoutSec, p.ShellDefaultTimeoutSec)
	applyInt(&next.ShellMaxTimeoutSec, p.ShellMaxTimeoutSec)
	applyInt(&next.MaxToolOutputKB, p.MaxToolOutputKB)

	applyBool(&next.AutonomousConfine, p.AutonomousConfine)
	applyBool(&next.AutonomousBootSeq, p.AutonomousBootSeq)

	applyBool(&next.BackupEnabled, p.BackupEnabled)
	applyInt(&next.BackupIntervalHours, p.BackupIntervalHours)
	applyInt(&next.BackupRetain, p.BackupRetain)
	if p.BackupDir != nil {
		next.BackupDir = strings.TrimSpace(*p.BackupDir)
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
		v.ContextBudgetCeil = 400000
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
	// Auto-compaction strategy; an unknown value (incl. empty, i.e. a settings.json
	// written before this field existed) falls back to today's rolling fold.
	switch v.AutoCompactMode {
	case AutoCompactRolling, AutoCompactNative, AutoCompactAuto:
	default:
		v.AutoCompactMode = AutoCompactRolling
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
	// One idle window for every turn kind: at least a minute, at most a day.
	if v.TurnIdleTimeoutMin < 1 {
		v.TurnIdleTimeoutMin = Default().TurnIdleTimeoutMin
	}
	if v.TurnIdleTimeoutMin > 1440 {
		v.TurnIdleTimeoutMin = 1440
	}
	// Flow run retention: 0 = unlimited; a hard ceiling keeps the sweeper cheap.
	if v.FlowRunRetention < 0 {
		v.FlowRunRetention = 0
	}
	if v.FlowRunRetention > 1000 {
		v.FlowRunRetention = 1000
	}
	// Codex stdout-silence watchdog: 0 disables it entirely; a negative value is a
	// typo, not an intent, so it falls back to the default. The value is in seconds;
	// the ceiling is a day, matching the other timeout knobs.
	if v.CodexStdoutIdleSec < 0 {
		v.CodexStdoutIdleSec = Default().CodexStdoutIdleSec
	}
	if v.CodexStdoutIdleSec > 86400 {
		v.CodexStdoutIdleSec = 86400
	}
	// Idle-resume budget: 0 disables it, cap at 5 so a persistently-idle turn cannot
	// chew through many full idle windows before it is finally reported unfinished.
	if v.IdleResumeMax < 0 {
		v.IdleResumeMax = 0
	}
	if v.IdleResumeMax > 5 {
		v.IdleResumeMax = 5
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
