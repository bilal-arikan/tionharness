package decider

import (
	"fmt"
	"slices"
	"sort"
	"sync"
)

// An Authority ("karar mercii") is one place in the app that hands a decision to
// a decision model: the stall judge, the shell-command risk check, a flow's
// judge node, and whatever comes next — a model router, a session janitor, a
// context picker. Authorities self-register from the package that implements
// them (RegisterAuthority in an init()), so this package knows none of them by
// name. Each gets the same machinery for free: an off / shadow / on switch, a
// threshold, its own decision model with an optional fallback and challenger,
// a ledger with agreement, latency and cost numbers, and the settings UI.
type Authority struct {
	// ID is the stable identifier ("tool-risk"), used in settings and the ledger.
	ID string `json:"id"`
	// Group sorts the authority into a settings section (Group* constants).
	Group string `json:"group"`
	// Pattern names the shape of its decision (Pattern* constants); the
	// settings page shows it, and the patterns.go helpers implement the
	// common ones.
	Pattern     Pattern `json:"pattern"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	// Modes are the modes the authority supports. Explicit authorities have no
	// previous logic to compare against, so they offer no shadow mode.
	Modes            []Mode  `json:"modes"`
	DefaultMode      Mode    `json:"defaultMode"`
	DefaultThreshold float64 `json:"defaultThreshold"`
	// ThresholdHint says what the threshold means for this authority.
	ThresholdHint string `json:"thresholdHint"`
	// Explicit authorities act only where the user asked for a judgement (a
	// flow node's "judge" match mode, a "judge" phase gate).
	Explicit bool `json:"explicit"`
	// FailClosed authorities refuse when no model can answer (a phase gate
	// stays shut) instead of falling back to earlier logic.
	FailClosed bool `json:"failClosed,omitempty"`
	// Order sorts authorities within their group.
	Order int `json:"order,omitempty"`
}

// Pattern is the shape of an authority's decision.
type Pattern string

const (
	// PatternGate: does a condition hold? (one yes/no question)
	PatternGate Pattern = "gate"
	// PatternPick: which one of N options? (one choice question)
	PatternPick Pattern = "pick"
	// PatternRate: where on an ordered scale? (one score question)
	PatternRate Pattern = "rate"
	// PatternSelect: which candidates are relevant? (a yes/no per candidate)
	PatternSelect Pattern = "select"
	// PatternTriage: which label does each item get? (a choice per item)
	PatternTriage Pattern = "triage"
)

// Settings groups, in display order.
const (
	GroupSafety       = "safety"
	GroupCoordination = "coordination"
	GroupFlows        = "flows"
	GroupRouting      = "routing"
	GroupContext      = "context"
	GroupHousekeeping = "housekeeping"
)

// Groups lists the settings groups in display order.
func Groups() []string {
	return []string{GroupSafety, GroupCoordination, GroupFlows, GroupRouting, GroupContext, GroupHousekeeping}
}

var (
	authoritiesMu sync.RWMutex
	authorities   = map[string]Authority{}
)

// RegisterAuthority adds an authority. It panics on a malformed or duplicate
// descriptor: both are programming errors in an init() function.
func RegisterAuthority(a Authority) {
	if err := a.check(); err != nil {
		panic("decider: " + err.Error())
	}
	authoritiesMu.Lock()
	defer authoritiesMu.Unlock()
	if _, dup := authorities[a.ID]; dup {
		panic(fmt.Sprintf("decider: authority %q registered twice", a.ID))
	}
	a.Modes = slices.Clone(a.Modes)
	authorities[a.ID] = a
}

func (a Authority) check() error {
	if err := validKey(a.ID); err != nil {
		return fmt.Errorf("authority id %q: %w", a.ID, err)
	}
	if !slices.Contains(Groups(), a.Group) {
		return fmt.Errorf("authority %q: unknown group %q", a.ID, a.Group)
	}
	if len(a.Modes) == 0 || !slices.Contains(a.Modes, a.DefaultMode) {
		return fmt.Errorf("authority %q: default mode %q is not one of its modes", a.ID, a.DefaultMode)
	}
	for _, m := range a.Modes {
		if !m.valid() {
			return fmt.Errorf("authority %q: unknown mode %q", a.ID, m)
		}
	}
	if a.Explicit && slices.Contains(a.Modes, ModeShadow) {
		return fmt.Errorf("authority %q: an explicit authority has no shadow mode", a.ID)
	}
	if a.DefaultThreshold < minThreshold || a.DefaultThreshold > maxThreshold {
		return fmt.Errorf("authority %q: default threshold %v is outside %v..%v", a.ID, a.DefaultThreshold, minThreshold, maxThreshold)
	}
	return nil
}

// Authorities lists every registered authority by group, then order, then id.
func Authorities() []Authority {
	authoritiesMu.RLock()
	out := make([]Authority, 0, len(authorities))
	for _, a := range authorities {
		out = append(out, a)
	}
	authoritiesMu.RUnlock()
	groupRank := func(g string) int { return slices.Index(Groups(), g) }
	sort.Slice(out, func(i, j int) bool {
		if gi, gj := groupRank(out[i].Group), groupRank(out[j].Group); gi != gj {
			return gi < gj
		}
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// AuthorityByID returns the authority registered under id.
func AuthorityByID(id string) (Authority, bool) {
	authoritiesMu.RLock()
	defer authoritiesMu.RUnlock()
	a, ok := authorities[id]
	return a, ok
}
