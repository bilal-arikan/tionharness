package decider

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Mode is how one site uses the decider.
type Mode string

const (
	// ModeOff: the site never consults the decider.
	ModeOff Mode = "off"
	// ModeShadow: the site keeps deciding the way it always has; the decider is
	// asked in the background and its answer is only logged next to the
	// existing verdict, so agreement can be measured before switching on.
	ModeShadow Mode = "shadow"
	// ModeOn: the decider's answer drives the site's behaviour. When the
	// decider is unreachable the site falls back to its previous logic.
	ModeOn Mode = "on"
)

// Site ids: every place in the app that can consult the decider.
const (
	// SiteStallJudge: "did the coordinator stop without doing what it said?"
	SiteStallJudge = "stall-judge"
	// SiteToolRisk: a second look at shell commands that would run without a
	// human decision (auto mode, or an "always allow" grant).
	SiteToolRisk = "tool-risk"
	// SiteFlowJudge: branch/loop nodes whose match mode is "judge".
	SiteFlowJudge = "flow-judge"
	// SitePhaseGate: Rota phase gates of kind "judge".
	SitePhaseGate = "phase-gate"
)

// Site describes one consumer of the decider for the settings UI.
type Site struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// Modes are the modes the site supports. Explicit sites have no previous
	// logic to compare against, so they offer no shadow mode.
	Modes            []Mode  `json:"modes"`
	DefaultMode      Mode    `json:"defaultMode"`
	DefaultThreshold float64 `json:"defaultThreshold"`
	// ThresholdHint says what the threshold means at this site.
	ThresholdHint string `json:"thresholdHint"`
	// Explicit sites act only where the user asked for a judgement (a flow
	// node's "judge" match mode, a "judge" phase gate).
	Explicit bool `json:"explicit"`
}

var sites = []Site{
	{
		ID:               SiteStallJudge,
		Label:            "Coordinator stall judge",
		Description:      "Decides whether a coordinator's last message promised worker spawns it never made. Replaces a Haiku call per idle coordinator turn.",
		Modes:            []Mode{ModeOff, ModeShadow, ModeOn},
		DefaultMode:      ModeShadow,
		DefaultThreshold: 0.7,
		ThresholdHint:    "Minimum probability of \"stalled\" before the coordinator is nudged.",
	},
	{
		ID:               SiteToolRisk,
		Label:            "Shell command risk check",
		Description:      "Takes a second look at shell commands that would run without asking (auto mode or an \"always allow\" grant). In on mode a risky command is turned into an approval prompt; it never blocks an unattended run.",
		Modes:            []Mode{ModeOff, ModeShadow, ModeOn},
		DefaultMode:      ModeShadow,
		DefaultThreshold: 0.8,
		ThresholdHint:    "Minimum probability that the command needs approval before the user is asked.",
	},
	{
		ID:               SiteFlowJudge,
		Label:            "Flow judge nodes",
		Description:      "Branch and loop nodes whose match mode is \"judge\" ask the decider which arm the last output belongs to, or whether the loop's exit condition holds.",
		Modes:            []Mode{ModeOff, ModeOn},
		DefaultMode:      ModeOn,
		DefaultThreshold: 0.6,
		ThresholdHint:    "Minimum probability for an arm to be taken (otherwise the default arm) or for the exit condition to count as met.",
		Explicit:         true,
	},
	{
		ID:               SitePhaseGate,
		Label:            "Rota judge gates",
		Description:      "Phase gates of kind \"judge\" ask the decider whether the phase's exit condition holds, judged against the root session's recent transcript.",
		Modes:            []Mode{ModeOff, ModeOn},
		DefaultMode:      ModeOn,
		DefaultThreshold: 0.8,
		ThresholdHint:    "Minimum probability that the exit condition holds before the gate opens.",
		Explicit:         true,
	},
}

// Sites lists every known site.
func Sites() []Site {
	return slices.Clone(sites)
}

// SiteByID returns the site with id.
func SiteByID(id string) (Site, bool) {
	for _, s := range sites {
		if s.ID == id {
			return s, true
		}
	}
	return Site{}, false
}

// SiteConfig is one site's settings.
type SiteConfig struct {
	Mode      Mode    `json:"mode"`
	Threshold float64 `json:"threshold"`
}

// Config is the decider's persisted configuration.
type Config struct {
	// Enabled is the master switch; while false every site is off.
	Enabled bool `json:"enabled"`
	// Backend is the registered backend id.
	Backend string `json:"backend"`
	// ProviderInstanceID names the provider instance whose credentials the
	// backend uses. "" = the first enabled instance the backend accepts.
	ProviderInstanceID string `json:"providerInstanceId"`
	// Model is the decision model id. "" = the backend's default.
	Model string `json:"model"`
	// TimeoutMs bounds one attempt of a call.
	TimeoutMs int `json:"timeoutMs"`
	// Sites holds per-site settings; a missing site uses its defaults.
	Sites map[string]SiteConfig `json:"sites"`
}

// Bounds applied by Normalized.
const (
	minTimeoutMs = 500
	maxTimeoutMs = 15000
	minThreshold = 0.5
	maxThreshold = 0.99
)

// DefaultConfig is the configuration used before the user saves one: switched
// off, OpenRouter + pinned Jev, every site at its default mode.
func DefaultConfig() Config {
	return Config{
		Backend:   OpenRouterBackendID,
		Model:     JevModel,
		TimeoutMs: int(DefaultTimeout / time.Millisecond),
	}.Normalized()
}

// Normalized fills defaults and clamps values into range. It never fails;
// Validate reports what cannot be repaired.
func (c Config) Normalized() Config {
	out := c
	out.Backend = strings.TrimSpace(out.Backend)
	if out.Backend == "" {
		out.Backend = OpenRouterBackendID
	}
	out.ProviderInstanceID = strings.TrimSpace(out.ProviderInstanceID)
	out.Model = strings.TrimSpace(out.Model)
	if out.Model == "" {
		if b, ok := Lookup(out.Backend); ok {
			out.Model = b.Manifest().DefaultModel
		}
	}
	if out.TimeoutMs <= 0 {
		out.TimeoutMs = int(DefaultTimeout / time.Millisecond)
	}
	out.TimeoutMs = min(max(out.TimeoutMs, minTimeoutMs), maxTimeoutMs)
	sitesOut := make(map[string]SiteConfig, len(sites))
	for _, s := range sites {
		sc, ok := c.Sites[s.ID]
		if !ok || sc.Mode == "" {
			sc.Mode = s.DefaultMode
		}
		if !slices.Contains(s.Modes, sc.Mode) {
			// An explicit site has no shadow mode (there is no previous verdict
			// to compare against), so shadow reads as on there. Any other
			// unsupported value reads as off.
			if s.Explicit && sc.Mode == ModeShadow {
				sc.Mode = ModeOn
			} else {
				sc.Mode = ModeOff
			}
		}
		if sc.Threshold == 0 {
			sc.Threshold = s.DefaultThreshold
		}
		sc.Threshold = min(max(sc.Threshold, minThreshold), maxThreshold)
		sitesOut[s.ID] = sc
	}
	out.Sites = sitesOut
	return out
}

// Validate reports settings that Normalized cannot repair: an unknown backend,
// an unknown site, or a mode that is not one of off/shadow/on.
func (c Config) Validate() error {
	if _, ok := Lookup(strings.TrimSpace(c.Backend)); c.Backend != "" && !ok {
		return fmt.Errorf("unknown decision backend %q", c.Backend)
	}
	for id, sc := range c.Sites {
		if _, ok := SiteByID(id); !ok {
			return fmt.Errorf("unknown decider site %q", id)
		}
		switch sc.Mode {
		case "", ModeOff, ModeShadow, ModeOn:
		default:
			return fmt.Errorf("site %q: unknown mode %q", id, sc.Mode)
		}
		if sc.Threshold < 0 || sc.Threshold > 1 {
			return fmt.Errorf("site %q: threshold %v is outside 0..1", id, sc.Threshold)
		}
	}
	return nil
}

// SiteMode is the effective mode of a site: off while the master switch is off.
func (c Config) SiteMode(site string) Mode {
	if !c.Enabled {
		return ModeOff
	}
	if sc, ok := c.Sites[site]; ok && sc.Mode != "" {
		return sc.Mode
	}
	if s, ok := SiteByID(site); ok {
		return s.DefaultMode
	}
	return ModeOff
}

// SiteThreshold is the configured threshold of a site (its default when unset).
func (c Config) SiteThreshold(site string) float64 {
	if sc, ok := c.Sites[site]; ok && sc.Threshold > 0 {
		return sc.Threshold
	}
	if s, ok := SiteByID(site); ok {
		return s.DefaultThreshold
	}
	return 0.8
}

// Timeout is the per-attempt timeout.
func (c Config) Timeout() time.Duration {
	if c.TimeoutMs <= 0 {
		return DefaultTimeout
	}
	return time.Duration(c.TimeoutMs) * time.Millisecond
}
