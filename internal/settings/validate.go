package settings

import (
	"fmt"
	"regexp"
	"strings"
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
	if p.Language != nil && !isSupportedLanguage(*p.Language) {
		return fmt.Errorf("language must be one of: %s (got %q)",
			strings.Join(SupportedLanguages, ", "), *p.Language)
	}
	// UILanguage additionally accepts "" — the explicit "follow the agent reply
	// language" state, which is how every pre-i18n installation starts out.
	if p.UILanguage != nil && *p.UILanguage != "" && !isSupportedLanguage(*p.UILanguage) {
		return fmt.Errorf("uiLanguage must be empty or one of: %s (got %q)",
			strings.Join(SupportedLanguages, ", "), *p.UILanguage)
	}
	if p.DefaultPermissionMode != nil {
		switch *p.DefaultPermissionMode {
		case "", "read-only", "ask", "auto":
		default:
			return fmt.Errorf("defaultPermissionMode must be one of: read-only, ask, auto (got %q)", *p.DefaultPermissionMode)
		}
	}
	if p.AutoCompactMode != nil {
		switch *p.AutoCompactMode {
		case "", AutoCompactRolling, AutoCompactNative, AutoCompactAuto:
		default:
			return fmt.Errorf("autoCompactMode must be one of: rolling, native, auto (got %q)", *p.AutoCompactMode)
		}
	}
	if p.Accent != nil && *p.Accent != "" && !isHexColor(*p.Accent) {
		return fmt.Errorf("accent must be a hex color like #8b5cf6 (got %q)", *p.Accent)
	}
	return nil
}
