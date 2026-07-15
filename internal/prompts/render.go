package prompts

import (
	"fmt"
	"strings"
)

// Render substitutes {{name}} placeholders in tmpl with the given values.
// Unknown placeholders in the template are left as-is (they render literally,
// which makes a typo visible instead of silently vanishing); keys in vars that
// the template does not reference are simply ignored.
func Render(tmpl string, vars map[string]string) string {
	out := tmpl
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

// Validate reports whether text is a usable override for key: non-blank and
// carrying every placeholder the Spec requires. An unknown key is an error so
// callers cannot silently resolve a prompt the registry does not know.
func Validate(key, text string) error {
	spec, ok := Get(key)
	if !ok {
		return fmt.Errorf("prompts: unknown key %q", key)
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("prompts: %s override is blank", key)
	}
	var missing []string
	for _, p := range spec.Placeholders {
		if !strings.Contains(text, "{{"+p+"}}") {
			missing = append(missing, "{{"+p+"}}")
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("prompts: %s override is missing required placeholder(s) %s", key, strings.Join(missing, ", "))
	}
	return nil
}
