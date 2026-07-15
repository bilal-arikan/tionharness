package prompts

import (
	"os"
	"strings"
)

// ResolveFile returns the prompt text for key, preferring the override file at
// path and falling back to the embedded default when the file is missing,
// blank, or fails placeholder validation. The fallback is deliberate: a broken
// user edit must degrade to the shipped prompt, never break a turn. (The
// Settings UI shows the live vs default diff, so a fallback is discoverable.)
func ResolveFile(path, key string) string {
	def := Default(key)
	if path == "" {
		return def
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return def
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return def
	}
	s = normalizeLegacy(key, s)
	if Validate(key, s) != nil {
		return def
	}
	return s
}

// NormalizeLegacy exposes the legacy-form conversion for callers that need to
// compare an on-disk override against the current default in normalized form
// (the seed cleanup uses it to recognize an old-build seed artifact whose only
// difference from the shipped default is the %s→{{...}} placeholder migration).
func NormalizeLegacy(key, text string) string { return normalizeLegacy(key, text) }

// normalizeLegacy converts a pre-registry "compact" override (the old
// fmt.Sprintf template with exactly two %s slots) to the named-placeholder
// form, so workspaces edited before the {{summary}}/{{messages}} migration keep
// working without the user touching the file. Any other shape passes through
// untouched and is judged by Validate.
func normalizeLegacy(key, text string) string {
	if key != "compact" || strings.Contains(text, "{{summary}}") {
		return text
	}
	if strings.Count(text, "%s") != 2 || strings.Count(text, "%") != 2 {
		return text
	}
	text = strings.Replace(text, "%s", "{{summary}}", 1)
	return strings.Replace(text, "%s", "{{messages}}", 1)
}
