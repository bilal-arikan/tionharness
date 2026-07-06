package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTemplate writes a template (and optional sidecar) into dir and returns the
// template's absolute path.
func writeTemplate(t *testing.T, dir, name, body, meta string) string {
	t.Helper()
	tp := filepath.Join(dir, name)
	if err := os.WriteFile(tp, []byte(body), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if meta != "" {
		mp := metaSidecarPath(tp)
		if err := os.WriteFile(mp, []byte(meta), 0o644); err != nil {
			t.Fatalf("write meta: %v", err)
		}
	}
	return tp
}

// callRender invokes the tool and returns (output, error).
func callRender(t *testing.T, renderDir, templatePath string, data map[string]any, outputName string) (string, error) {
	t.Helper()
	in := map[string]any{"template": templatePath, "data": data}
	if outputName != "" {
		in["output_name"] = outputName
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	return NewRenderTemplateTool(renderDir).Call(context.Background(), raw)
}

func TestRenderTemplate_AutoEscapesData(t *testing.T) {
	dir := t.TempDir()
	renderDir := filepath.Join(dir, "render")
	tp := writeTemplate(t, dir, "greet.html", `<h1>{{.title}}</h1>`, "")

	out, err := callRender(t, renderDir, tp, map[string]any{"title": `<script>alert(1)</script>`}, "")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	// The output path is echoed back; read the file and confirm the script tag was
	// escaped by html/template (never emitted raw).
	path := extractRenderedPath(t, out)
	rendered, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered: %v", err)
	}
	if strings.Contains(string(rendered), "<script>") {
		t.Errorf("expected auto-escaped output, got raw <script>: %s", rendered)
	}
	if !strings.Contains(string(rendered), "&lt;script&gt;") {
		t.Errorf("expected escaped &lt;script&gt; in output, got: %s", rendered)
	}
}

func TestRenderTemplate_MissingRequiredFieldWarnsNotFails(t *testing.T) {
	dir := t.TempDir()
	renderDir := filepath.Join(dir, "render")
	meta := `{"id":"report","requiredFields":["title","items"]}`
	tp := writeTemplate(t, dir, "report.html", `<h1>{{.title}}</h1>`, meta)

	out, err := callRender(t, renderDir, tp, map[string]any{"title": "Q1"}, "")
	if err != nil {
		t.Fatalf("missing field must be SOFT (render + warn), got hard error: %v", err)
	}
	if !strings.Contains(out, "missing required field: items") {
		t.Errorf("expected warning for missing 'items', got: %s", out)
	}
	// The file must still have been written despite the missing field.
	if _, err := os.Stat(extractRenderedPath(t, out)); err != nil {
		t.Errorf("expected output written despite warning: %v", err)
	}
}

func TestRenderTemplate_BrokenTemplateHardFails(t *testing.T) {
	dir := t.TempDir()
	renderDir := filepath.Join(dir, "render")
	tp := writeTemplate(t, dir, "bad.html", `<h1>{{.title</h1>`, "") // unclosed action

	if _, err := callRender(t, renderDir, tp, map[string]any{"title": "x"}, ""); err == nil {
		t.Error("expected hard error on a broken template, got nil")
	}
}

func TestRenderTemplate_MalformedSidecarHardFails(t *testing.T) {
	dir := t.TempDir()
	renderDir := filepath.Join(dir, "render")
	tp := writeTemplate(t, dir, "report.html", `<h1>{{.title}}</h1>`, `{ not json`)

	if _, err := callRender(t, renderDir, tp, map[string]any{"title": "x"}, ""); err == nil {
		t.Error("expected hard error on a malformed sidecar meta, got nil")
	}
}

func TestRenderTemplate_NoSessionHardFails(t *testing.T) {
	dir := t.TempDir()
	tp := writeTemplate(t, dir, "report.html", `<h1>{{.title}}</h1>`, "")

	if _, err := callRender(t, "" /* no render dir = no session */, tp, map[string]any{"title": "x"}, ""); err == nil {
		t.Error("expected hard error when no session is bound, got nil")
	}
}

func TestRenderTemplate_MissingTemplateHardFails(t *testing.T) {
	dir := t.TempDir()
	renderDir := filepath.Join(dir, "render")
	missing := filepath.Join(dir, "nope.html")

	if _, err := callRender(t, renderDir, missing, map[string]any{}, ""); err == nil {
		t.Error("expected hard error for a missing template file, got nil")
	}
}

func TestRenderTemplate_OutputUnderRenderDir(t *testing.T) {
	dir := t.TempDir()
	renderDir := filepath.Join(dir, "render")
	tp := writeTemplate(t, dir, "report.html", `<p>{{.body}}</p>`, "")

	// A crafted output_name with traversal must be reduced to a bare basename and
	// stay inside the render dir.
	out, err := callRender(t, renderDir, tp, map[string]any{"body": "hi"}, "../escape.html")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	path := extractRenderedPath(t, out)
	rel, err := filepath.Rel(renderDir, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Errorf("output escaped render dir: renderDir=%s path=%s rel=%s", renderDir, path, rel)
	}
	if filepath.Base(path) != "escape.html" {
		t.Errorf("expected basename escape.html, got %s", filepath.Base(path))
	}
}

func TestRenderTemplate_MissingFieldWithoutSidecarNoWarn(t *testing.T) {
	dir := t.TempDir()
	renderDir := filepath.Join(dir, "render")
	tp := writeTemplate(t, dir, "plain.html", `<p>{{.body}}</p>`, "") // no sidecar

	out, err := callRender(t, renderDir, tp, map[string]any{}, "")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if strings.Contains(out, "Warnings") {
		t.Errorf("expected no warnings without a sidecar, got: %s", out)
	}
}

// extractRenderedPath pulls the "Rendered <path> (" prefix path out of the tool's
// result summary, so tests can inspect the written file.
func extractRenderedPath(t *testing.T, out string) string {
	t.Helper()
	const prefix = "Rendered "
	i := strings.Index(out, prefix)
	if i < 0 {
		t.Fatalf("no rendered-path prefix in output: %s", out)
	}
	rest := out[i+len(prefix):]
	end := strings.Index(rest, " (")
	if end < 0 {
		t.Fatalf("no size suffix after path in output: %s", out)
	}
	return rest[:end]
}
