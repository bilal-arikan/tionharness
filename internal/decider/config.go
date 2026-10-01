package decider

import (
	"fmt"
	"slices"
	"strings"
)

// Mode is how one authority uses the decider.
type Mode string

const (
	// ModeOff: the authority never consults the decider.
	ModeOff Mode = "off"
	// ModeShadow: the authority keeps deciding the way it always has; the
	// decider is asked in the background and its answer is only logged next to
	// the existing verdict, so agreement can be measured before switching on.
	ModeShadow Mode = "shadow"
	// ModeOn: the decider's answer drives the authority's behaviour. When no
	// model can answer, the authority falls back to its previous logic (or,
	// if it is fail-closed, refuses).
	ModeOn Mode = "on"
)

func (m Mode) valid() bool {
	return m == ModeOff || m == ModeShadow || m == ModeOn
}

// AuthorityConfig is one authority's settings.
type AuthorityConfig struct {
	Mode      Mode    `json:"mode"`
	Threshold float64 `json:"threshold"`
	// Model is the decision model (instance id) that answers for the
	// authority. "" = the default model.
	Model string `json:"model,omitempty"`
	// Fallback is asked when Model cannot answer (unreachable, backing off,
	// timed out, rejected): a local model first, a hosted one behind it.
	Fallback string `json:"fallback,omitempty"`
	// Challenger is asked the same question in the background after every
	// answer; the two verdicts land in the ledger, so a new model can be
	// measured against the current one on real traffic before it takes over.
	Challenger     string `json:"challenger,omitempty"`
	CandidateLimit int    `json:"candidateLimit,omitempty"`
	SelectionLimit int    `json:"selectionLimit,omitempty"`
	RemindEvery    int    `json:"remindEvery,omitempty"`
	ContextBudget  int    `json:"contextBudget,omitempty"`
}

// Config is the decider's persisted configuration: the master switch, the
// default decision model and every authority's settings. The decision models
// themselves live in their own store (ModelStore).
type Config struct {
	// Enabled is the master switch; while false every authority is off.
	Enabled bool `json:"enabled"`
	// DefaultModel answers for every authority that names no model of its own.
	// "" = the first enabled model.
	DefaultModel string `json:"defaultModel"`
	// Authorities holds per-authority settings; a missing one uses its defaults.
	Authorities map[string]AuthorityConfig `json:"authorities"`
}

// Bounds applied by Normalized and to model settings.
const (
	minTimeoutMs = 500
	maxTimeoutMs = 60000
	minThreshold = 0.5
	maxThreshold = 0.99
)

// DefaultConfig is the configuration used before the user saves one: switched
// off, every authority at its default mode.
func DefaultConfig() Config {
	return Config{}.Normalized()
}

// Normalized fills defaults and clamps values into range. It never fails;
// Validate reports what cannot be repaired. Settings of an authority that is no
// longer registered are dropped.
func (c Config) Normalized() Config {
	out := Config{Enabled: c.Enabled, DefaultModel: strings.TrimSpace(c.DefaultModel)}
	registered := Authorities()
	out.Authorities = make(map[string]AuthorityConfig, len(registered))
	for _, a := range registered {
		ac := c.Authorities[a.ID]
		if ac.Mode == "" {
			ac.Mode = a.DefaultMode
		}
		if !slices.Contains(a.Modes, ac.Mode) {
			// An explicit authority has no shadow mode (there is no previous
			// verdict to compare against), so shadow reads as on there. Any
			// other unsupported value reads as off.
			if a.Explicit && ac.Mode == ModeShadow {
				ac.Mode = ModeOn
			} else {
				ac.Mode = ModeOff
			}
		}
		if ac.Threshold == 0 {
			ac.Threshold = a.DefaultThreshold
		}
		ac.Threshold = min(max(ac.Threshold, minThreshold), maxThreshold)
		// Preserve the persisted shape of authorities that do not use workflow limits.
		if ac.CandidateLimit != 0 || ac.SelectionLimit != 0 || ac.RemindEvery != 0 || ac.ContextBudget != 0 {
			ac = ac.WithWorkflowDefaults()
		}
		ac.Model = strings.TrimSpace(ac.Model)
		ac.Fallback = strings.TrimSpace(ac.Fallback)
		ac.Challenger = strings.TrimSpace(ac.Challenger)
		if ac.Fallback != "" && ac.Fallback == ac.Model {
			ac.Fallback = ""
		}
		if ac.Challenger != "" && ac.Challenger == ac.Model {
			ac.Challenger = ""
		}
		out.Authorities[a.ID] = ac
	}
	return out
}

// withoutMissingModels clears every reference to a decision model known does
// not report: the default model and each authority's model, fallback and
// challenger. A deleted model therefore never leaves an authority pointing at
// nothing; the authority falls back to the default model.
func (c Config) withoutMissingModels(known func(id string) bool) Config {
	out := c
	if out.DefaultModel != "" && !known(out.DefaultModel) {
		out.DefaultModel = ""
	}
	out.Authorities = make(map[string]AuthorityConfig, len(c.Authorities))
	for id, ac := range c.Authorities {
		for _, ref := range []*string{&ac.Model, &ac.Fallback, &ac.Challenger} {
			if *ref != "" && !known(*ref) {
				*ref = ""
			}
		}
		out.Authorities[id] = ac
	}
	return out
}

// Validate reports settings that Normalized cannot repair: an unknown
// authority, a mode that is not one of off/shadow/on, a threshold outside 0..1.
// Model references are checked by the Hub, which knows the models.
func (c Config) Validate() error {
	for id, ac := range c.Authorities {
		if _, ok := AuthorityByID(id); !ok {
			return fmt.Errorf("unknown decision authority %q", id)
		}
		if ac.Mode != "" && !ac.Mode.valid() {
			return fmt.Errorf("authority %q: unknown mode %q", id, ac.Mode)
		}
		if ac.Threshold < 0 || ac.Threshold > 1 {
			return fmt.Errorf("authority %q: threshold %v is outside 0..1", id, ac.Threshold)
		}
	}
	return nil
}

// Authority returns an authority's settings, its defaults when unset.
func (c Config) Authority(id string) AuthorityConfig {
	if ac, ok := c.Authorities[id]; ok {
		return ac
	}
	if a, ok := AuthorityByID(id); ok {
		return AuthorityConfig{Mode: a.DefaultMode, Threshold: a.DefaultThreshold}
	}
	return AuthorityConfig{Mode: ModeOff, Threshold: 0.8}
}

// Mode is the effective mode of an authority: off while the master switch is off.
func (c Config) Mode(authority string) Mode {
	if !c.Enabled {
		return ModeOff
	}
	if m := c.Authority(authority).Mode; m != "" {
		return m
	}
	return ModeOff
}

// Threshold is the configured threshold of an authority (its default when unset).
func (c Config) Threshold(authority string) float64 {
	if t := c.Authority(authority).Threshold; t > 0 {
		return t
	}
	return 0.8
}
