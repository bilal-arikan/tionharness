package settings

import (
	"fmt"
	"regexp"
)

// hexColorRe matches a CSS hex color: #RGB, #RGBA, #RRGGBB or #RRGGBBAA.
var hexColorRe = regexp.MustCompile(`^#([0-9A-Fa-f]{3,4}|[0-9A-Fa-f]{6}|[0-9A-Fa-f]{8})$`)

// isHexColor reports whether s is a valid CSS hex color.
func isHexColor(s string) bool { return hexColorRe.MatchString(s) }

// Validate checks the enum/format fields that are actually present in a patch
// and returns a descriptive error for any invalid value. It is the guard rail
// that keeps a bad change (from the Settings screen, the API, or an agent's
// update_settings tool) from being persisted at all.
//
// Numeric fields are intentionally NOT rejected here: they are clamped to safe
// ranges by normalize, so an out-of-range number is corrected rather than
// refused. Only values that have no sensible coercion — unknown enums, malformed
// colors — are rejected so the caller gets clear feedback.
func Validate(p Patch) error {
	if p.Theme != nil {
		switch *p.Theme {
		case ThemeDark, ThemeLight, ThemeSystem:
		default:
			return fmt.Errorf("theme must be one of: dark, light, system (got %q)", *p.Theme)
		}
	}
	if p.Language != nil && *p.Language != "tr" && *p.Language != "en" {
		return fmt.Errorf("language must be \"tr\" or \"en\" (got %q)", *p.Language)
	}
	if p.DefaultProvider != nil {
		switch *p.DefaultProvider {
		case "claude-cli", "anthropic":
		default:
			return fmt.Errorf("defaultProvider must be \"claude-cli\" or \"anthropic\" (got %q)", *p.DefaultProvider)
		}
	}
	if p.DefaultPermissionMode != nil {
		switch *p.DefaultPermissionMode {
		case "", "read-only", "ask", "auto":
		default:
			return fmt.Errorf("defaultPermissionMode must be one of: read-only, ask, auto (got %q)", *p.DefaultPermissionMode)
		}
	}
	if p.Accent != nil && *p.Accent != "" && !isHexColor(*p.Accent) {
		return fmt.Errorf("accent must be a hex color like #8b5cf6 (got %q)", *p.Accent)
	}
	return nil
}
