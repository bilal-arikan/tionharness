package tools

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// RenderTemplateTool fills a branded HTML template (Go html/template syntax) with
// JSON data and writes the result under the session render dir, returning ONLY
// the output path plus any soft-validation warnings — never the HTML itself — so
// the filled markup never re-enters the model's context (the token-saving point).
// The chat UI shows it inline via an ```html-preview block.
//
// Failure model: a technical fault — missing/unreadable template, template parse
// or execute error, malformed sidecar meta, write failure, or no session bound —
// is a HARD error and fails loudly. Only a missing DECLARED required field is
// soft: the template still renders (the field is empty) and the absence is
// returned as a warning, matching the soft-validation contract.
type RenderTemplateTool struct {
	// renderDir is <store>/render/<sessionID>; empty when no session is bound to
	// the turn (catalog/preview builds), which makes Call fail loudly.
	renderDir string
}

// NewRenderTemplateTool binds the tool to a session's render directory. Pass
// Runtime.SessionRenderDir(SessionIDFrom(ctx)); an empty dir disables the tool
// (Call returns an error) rather than writing output to an ambiguous location.
func NewRenderTemplateTool(renderDir string) RenderTemplateTool {
	return RenderTemplateTool{renderDir: renderDir}
}

func (RenderTemplateTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "render_template",
		Description: "Fill a branded HTML template (Go html/template syntax, auto-escaped) with JSON data and " +
			"write the result to a file, returning ONLY the output path plus any warnings — NOT the HTML — so the " +
			"filled markup never re-enters your context (saves tokens). Show it inline by emitting an ```html-preview " +
			"block with {\"src\":\"<returned path>\"}. Templates live beside a skill (e.g. ${SKILL_DIR}/templates/report.html); " +
			"an optional <template>.meta.json sidecar declares requiredFields. Missing required fields are SOFT (still " +
			"renders, returns a warning); a broken template or invalid data is a hard error.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"template":{"type":"string","description":"Path to the .html template file (absolute, or relative to the working directory; e.g. an expanded ${SKILL_DIR}/templates/report.html)"},
				"data":{"type":"object","description":"JSON object of field values fed to the template (e.g. {\"title\":\"Q1\",\"items\":[...]})"},
				"output_name":{"type":"string","description":"Optional output file basename (defaults to <template>-<hash>.html). Written under the session render dir; any directory part is stripped."}
			},
			"required":["template","data"],
			"additionalProperties":false
		}`),
	}
}

func (t RenderTemplateTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Template   string         `json:"template"`
		Data       map[string]any `json:"data"`
		OutputName string         `json:"output_name"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	if strings.TrimSpace(args.Template) == "" {
		return "", fmt.Errorf("template path is required")
	}
	// data is declared required in the schema; enforce it here too (mirroring
	// transform_data) so an omitted object fails loudly instead of silently
	// rendering every field empty. An explicit empty object {} is allowed for
	// templates with no dynamic fields.
	if args.Data == nil {
		return "", fmt.Errorf("data is required (pass {} for a template with no dynamic fields)")
	}
	if t.renderDir == "" {
		return "", fmt.Errorf("render_template is unavailable: no session is bound to this turn")
	}

	templatePath := filepath.Clean(args.Template)
	if st, err := os.Stat(templatePath); err != nil {
		return "", fmt.Errorf("template %q not found: %w", args.Template, err)
	} else if st.IsDir() {
		return "", fmt.Errorf("template %q is a directory, not a file", args.Template)
	}

	// Soft-validation: read the optional sidecar and compute missing declared
	// fields. A malformed sidecar is a hard error (readTemplateMeta).
	meta, _, err := readTemplateMeta(templatePath)
	if err != nil {
		return "", err
	}
	var warnings []string
	for _, f := range missingRequiredFields(meta.RequiredFields, args.Data) {
		warnings = append(warnings, "missing required field: "+f)
	}

	// Render — hard-fails on a parse or execute error.
	out, err := renderTemplateFile(templatePath, args.Data)
	if err != nil {
		return "", err
	}

	// Write under the session render dir. The output name is reduced to a bare
	// basename, so a crafted output_name can never escape the render dir.
	if err := os.MkdirAll(t.renderDir, 0o755); err != nil {
		return "", fmt.Errorf("create render directory: %w", err)
	}
	outPath := filepath.Join(t.renderDir, renderOutputName(args.OutputName, templatePath, out))
	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		return "", fmt.Errorf("write rendered output: %w", err)
	}

	return formatRenderResult(outPath, len(out), warnings), nil
}

// renderOutputName picks the output basename: an explicit output_name (reduced to
// its base name with a .html extension enforced) or "<templatestem>-<hash>.html".
// The content hash keeps repeated renders of the same template from clobbering
// each other while staying stable for identical output.
func renderOutputName(explicit, templatePath string, content []byte) string {
	if e := strings.TrimSpace(explicit); e != "" {
		base := filepath.Base(filepath.Clean(e))
		if base != "." && base != string(filepath.Separator) {
			if !strings.EqualFold(filepath.Ext(base), ".html") {
				base += ".html"
			}
			return base
		}
	}
	sum := sha1.Sum(content)
	stem := strings.TrimSuffix(filepath.Base(templatePath), filepath.Ext(templatePath))
	return fmt.Sprintf("%s-%s.html", stem, hex.EncodeToString(sum[:])[:8])
}

// formatRenderResult is the compact summary returned to the model: the output
// path (for the html-preview src), its size, a ready-to-emit html-preview block,
// and any soft-validation warnings. The rendered HTML is intentionally excluded.
func formatRenderResult(outPath string, size int, warnings []string) string {
	src, _ := json.Marshal(outPath) // marshalling a string never fails
	var b strings.Builder
	fmt.Fprintf(&b, "Rendered %s (%s).", outPath, humanBytes(int64(size)))
	fmt.Fprintf(&b, "\nShow it inline with:\n```html-preview\n{\"src\": %s}\n```", string(src))
	if len(warnings) > 0 {
		b.WriteString("\n\nWarnings (rendered anyway):")
		for _, w := range warnings {
			fmt.Fprintf(&b, "\n- %s", w)
		}
	}
	return b.String()
}
