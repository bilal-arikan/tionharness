package settings

// Language registry. TionHarness distinguishes TWO independent language axes and
// they must not be collapsed into one field:
//
//   - Settings.Language   — the AGENT reply language. Feeds the system prompt
//     ("reply in Turkish by default") and non-chat producers such as the insight
//     analyzer. Changing it re-freezes the prompt epoch and cools the prompt
//     cache, so it is deliberately a separate, sticky choice.
//   - Settings.UILanguage — the language of the web UI chrome (buttons, labels,
//     errors). Purely client-side; changing it never touches a prompt or a cache.
//     "" means "follow Settings.Language", which keeps the historical behaviour
//     for every existing installation.
//
// A user may legitimately want an English interface while talking to the agent in
// Turkish (or the reverse), which is exactly why the axes are split.

// SupportedLanguages lists the language codes the app can be configured with, in
// display order. Adding a locale is a two-step change: append it here (backend
// validation + normalization follow automatically) and add the matching catalog
// under frontend/src/i18n/locales/<code>/.
var SupportedLanguages = []string{"tr", "en"}

// DefaultLanguage is the agent reply language used when nothing is configured.
const DefaultLanguage = "tr"

// languageNames maps a code to the human name injected into the system prompt
// ("reply in <name>"). Kept beside SupportedLanguages so a new locale is added in
// exactly one backend place.
var languageNames = map[string]string{
	"tr": "Turkish (Türkçe)",
	"en": "English",
}

// LanguageDisplayName returns the prompt-facing name of a language code, or ""
// for an unknown code (callers then omit the preference line entirely).
func LanguageDisplayName(code string) string { return languageNames[code] }

// isSupportedLanguage reports whether code is a known language code.
func isSupportedLanguage(code string) bool {
	for _, c := range SupportedLanguages {
		if c == code {
			return true
		}
	}
	return false
}

// EffectiveUILanguage resolves the language the interface should render in: the
// explicit UI choice when set, otherwise the agent reply language. Callers get a
// supported code back, never "".
func EffectiveUILanguage(s Settings) string {
	if isSupportedLanguage(s.UILanguage) {
		return s.UILanguage
	}
	if isSupportedLanguage(s.Language) {
		return s.Language
	}
	return DefaultLanguage
}
