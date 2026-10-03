package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ValidToolOverrideTier is shared by persistence and permission readers. The
// visibility values match tools.Visibility* without introducing a DB dependency
// on the tool implementation package.
func ValidToolOverrideTier(tier string) bool {
	switch tier {
	case "full", "summary", "name-only", "hidden", TierBlocked:
		return true
	default:
		return false
	}
}

// ParseAgentToolOverrides is the compatibility boundary for imported agents and
// old stores. Explicit valid overrides win over the legacy denylist. Partial
// results are for display only; permission callers must fail closed on errors.
func ParseAgentToolOverrides(a Agent) (map[string]string, error) {
	out := map[string]string{}
	var errs []error
	if raw := strings.TrimSpace(a.ToolOverrides); raw != "" {
		var overrides map[string]string
		if err := json.Unmarshal([]byte(raw), &overrides); err != nil {
			errs = append(errs, fmt.Errorf("tool_overrides: %w", err))
		} else {
			for name, tier := range overrides {
				if ValidToolOverrideTier(tier) {
					out[name] = tier
				} else {
					errs = append(errs, fmt.Errorf("tool_overrides: tool %q has unknown tier %q", name, tier))
				}
			}
		}
	}
	if raw := strings.TrimSpace(a.BlockedTools); raw != "" && raw != "[]" {
		var legacy []string
		if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
			errs = append(errs, fmt.Errorf("blocked_tools: %w", err))
		} else {
			for _, name := range legacy {
				if _, explicit := out[name]; !explicit {
					out[name] = TierBlocked
				}
			}
		}
	}
	return out, errors.Join(errs...)
}

// encodeAgentToolOverrides derives the old-reader mirror from the single map.
// JSON map keys and blocked names are sorted for stable, idempotent writes.
func encodeAgentToolOverrides(overrides map[string]string) (string, string) {
	blocked := []string{}
	for name, tier := range overrides {
		if tier == TierBlocked {
			blocked = append(blocked, name)
		}
	}
	slices.Sort(blocked)
	canonical, _ := json.Marshal(overrides)
	mirror, _ := json.Marshal(blocked)
	return string(canonical), string(mirror)
}

// normalizeAgentToolOverrides folds valid old/mixed inputs into the canonical
// map at load and write boundaries. Corrupt documents remain byte-for-byte
// intact so normalization cannot turn a denied permission into an allowed one.
// Load normalization is in memory; the next explicit save persists it.
func normalizeAgentToolOverrides(a Agent) Agent {
	overrides, err := ParseAgentToolOverrides(a)
	if err != nil {
		return a
	}
	a.ToolOverrides, a.BlockedTools = encodeAgentToolOverrides(overrides)
	return a
}
