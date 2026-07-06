package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
)

// templateMeta is the optional sidecar (<template>.meta.json) that declares a
// template's required fields and a human description. A template with NO sidecar
// simply has no declared required fields — soft-validation is then a no-op.
type templateMeta struct {
	ID             string   `json:"id"`
	RequiredFields []string `json:"requiredFields"`
	Description    string   `json:"description"`
}

// metaSidecarPath maps a template path to its sidecar meta path:
// report.html → report.meta.json.
func metaSidecarPath(templatePath string) string {
	ext := filepath.Ext(templatePath)
	return strings.TrimSuffix(templatePath, ext) + ".meta.json"
}

// readTemplateMeta loads the sidecar meta for a template if present.
//
// A MISSING sidecar is not an error: it returns a zero meta with ok=false, so a
// template can ship without declared fields. A PRESENT but malformed sidecar IS
// a hard error — a corrupt declaration must fail loudly rather than silently
// disabling validation.
func readTemplateMeta(templatePath string) (meta templateMeta, ok bool, err error) {
	metaPath := metaSidecarPath(templatePath)
	data, readErr := os.ReadFile(metaPath)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return templateMeta{}, false, nil
		}
		return templateMeta{}, false, fmt.Errorf("read template meta %q: %w", metaPath, readErr)
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return templateMeta{}, false, fmt.Errorf("parse template meta %q: %w", metaPath, err)
	}
	return meta, true, nil
}

// missingRequiredFields returns, in declaration order, the required fields that
// are absent from data or present-but-empty. This is the SOFT-validation signal:
// the caller still renders and surfaces these as warnings — a missing business
// field is not a technical failure.
func missingRequiredFields(required []string, data map[string]any) []string {
	var missing []string
	for _, f := range required {
		v, ok := data[f]
		if !ok || isEmptyValue(v) {
			missing = append(missing, f)
		}
	}
	return missing
}

// isEmptyValue reports whether a JSON-decoded value counts as "not provided" for
// required-field validation: nil, a blank string, or an empty array/object. A
// numeric 0 or boolean false is a real value and is NOT treated as empty.
func isEmptyValue(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	return false
}

// renderTemplateFile parses and executes an html/template file with data, using
// auto-escaping (XSS-safe) — the reason we use html/template over naive string
// substitution when mixing agent-authored templates with agent-supplied data.
//
// A parse or execute error is a HARD failure (returned as error). Only missing
// DECLARED fields are soft (they render empty and are reported as warnings by the
// caller) — a technical fault is never swallowed.
func renderTemplateFile(templatePath string, data map[string]any) ([]byte, error) {
	src, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("read template %q: %w", templatePath, err)
	}
	tmpl, err := template.New(filepath.Base(templatePath)).Parse(string(src))
	if err != nil {
		return nil, fmt.Errorf("parse template %q: %w", templatePath, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template %q: %w", templatePath, err)
	}
	return buf.Bytes(), nil
}
