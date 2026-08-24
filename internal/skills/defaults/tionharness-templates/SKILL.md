---
name: "TionHarness Templates"
description: "Fill branded HTML templates with data and show the result inline, without re-emitting the same markup every time (render_template + html-preview)."
when_to_use: "When a task asks for a branded/styled HTML deliverable — a report, an email, a summary sheet — where the layout is fixed and only the data changes."
icon: "🧾"
color: "#6366f1"
access: shared
---
# Branded HTML from templates (render_template → html-preview)

When the LAYOUT is fixed and only the DATA changes, do NOT hand-write the same
branded HTML every time. Fill a template instead: the `render_template` tool runs
the file through Go's `html/template` engine (auto-escaping, XSS-safe), writes the
result to disk, and returns ONLY the output path plus any warnings — the filled
HTML never re-enters your context (that is the token saving). You then show it
inline with an ```html-preview block.

## Workflow

1. Pick a template from the list below (or point at any `.html` file that uses
   `html/template` syntax).
2. Call `render_template` with the template's ABSOLUTE path and a `data` object:

   ```
   render_template({
     "template": "${SKILL_DIR}/templates/report.html",
     "data": {
       "title": "Q1 Review",
       "subtitle": "Engineering",
       "date": "2026-04-01",
       "summary": "Three shipped, one slipped.",
       "items": [
         { "heading": "Auth rewrite", "body": "Shipped ahead of schedule." },
         { "heading": "Billing", "body": "Slipped a sprint on a vendor bug." }
       ]
     }
   })
   ```

   `${SKILL_DIR}` is expanded for you to this skill's directory — pass the
   already-expanded absolute path the tool sees, not the literal `${SKILL_DIR}`.

3. The tool returns a path and a ready-to-paste block. Emit it in your reply so it
   renders inline:

   ```html-preview
   { "src": "<the returned path>", "title": "Q1 Review" }
   ```

   For several rendered files at once, use tabs:
   `{ "items": [ { "src": "...", "label": "Report" }, { "src": "...", "label": "Email" } ] }`

## Available templates

| Template | Path | Required fields | Optional fields |
|----------|------|-----------------|-----------------|
| Report   | `${SKILL_DIR}/templates/report.html` | `title`, `items[]` (`heading`, `body`) | `subtitle`, `date`, `summary`, `footer` |
| Email    | `${SKILL_DIR}/templates/email.html`  | `subject`, `paragraphs[]` (strings)    | `heading`, `recipient`, `ctaUrl`, `ctaLabel`, `footer` |

Each template has a `<name>.meta.json` sidecar declaring its `requiredFields`.

## Soft vs hard failure

- **Missing a required field** is SOFT: the template still renders (that field
  comes out empty) and `render_template` returns a `warning`. Read the warning
  and re-render with the field filled if the output looks incomplete.
- **A broken template or invalid data** (parse/execute error, malformed sidecar)
  is a HARD error — fix the input and call again.

## Writing your own template

Any self-contained `.html` file using `html/template` syntax works:
`{{.field}}`, `{{if .x}}…{{end}}`, `{{range .items}}…{{end}}`. Keep it a single
file with inline `<style>` (the sandboxed preview iframe blocks external
resources). Drop an optional `<name>.meta.json` beside it —
`{ "requiredFields": ["title"] }` — to get soft-validation warnings.
