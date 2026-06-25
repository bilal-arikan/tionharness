package skills

import "strings"

// frontmatter is the parsed metadata block of a SKILL.md. Unknown keys are kept
// in scalars/lists so future fields degrade gracefully.
type frontmatter struct {
	scalars map[string]string
	lists   map[string][]string
}

func (f frontmatter) scalar(keys ...string) string {
	for _, k := range keys {
		if v, ok := f.scalars[strings.ToLower(k)]; ok {
			return v
		}
	}
	return ""
}

func (f frontmatter) list(keys ...string) []string {
	for _, k := range keys {
		if v, ok := f.lists[strings.ToLower(k)]; ok {
			return v
		}
	}
	return nil
}

// FrontmatterField returns a scalar frontmatter value (first matching key) from raw
// SKILL.md/markdown text. Exposed for the ingest adapters, which read a handful of
// fields (name/description/tools/model) without needing the parser type.
func FrontmatterField(raw string, keys ...string) string {
	fm, _ := parseFrontmatter(raw)
	return fm.scalar(keys...)
}

// FrontmatterList returns a list frontmatter value (first matching key) from raw
// markdown text. Exposed for the ingest adapters.
func FrontmatterList(raw string, keys ...string) []string {
	fm, _ := parseFrontmatter(raw)
	return fm.list(keys...)
}

// FrontmatterBody returns the markdown body (frontmatter stripped) of raw text.
// Exposed for the ingest adapters.
func FrontmatterBody(raw string) string {
	_, body := parseFrontmatter(raw)
	return strings.TrimSpace(body)
}

// splitFrontmatter separates a leading `---`-delimited frontmatter block from the
// markdown body. When no well-formed block is present, fm is empty and body is
// the whole input.
func splitFrontmatter(content string) (fmText, body string) {
	s := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return "", s
	}
	rest := s[len("---\n"):]
	// The closing fence is a line that is exactly "---".
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", s
	}
	fmText = rest[:end]
	after := rest[end+len("\n---"):]
	after = strings.TrimPrefix(after, "\n")
	// Tolerate a trailing newline right after the closing fence marker.
	if strings.HasPrefix(after, "\n") {
		after = after[1:]
	}
	return fmText, after
}

// parseFrontmatter parses the small subset of YAML the skill format needs:
// `key: value` scalars (optionally quoted), inline arrays `key: [a, b]`, and
// block lists:
//
//	key:
//	  - item one
//	  - item two
//
// It is intentionally dependency-free (the project keeps go.mod minimal).
func parseFrontmatter(content string) (frontmatter, string) {
	fmText, body := splitFrontmatter(content)
	fm := frontmatter{scalars: map[string]string{}, lists: map[string][]string{}}
	if fmText == "" {
		return fm, body
	}

	lines := strings.Split(fmText, "\n")
	var curList string // key currently accumulating block-list items
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		line := strings.TrimRight(raw, " \t")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		trimmed := strings.TrimSpace(line)

		// Continuation of a block list: "- item".
		if curList != "" && strings.HasPrefix(trimmed, "- ") {
			if item := unquote(strings.TrimSpace(trimmed[2:])); item != "" {
				fm.lists[curList] = append(fm.lists[curList], item)
			}
			continue
		}
		curList = ""

		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:colon]))
		val := strings.TrimSpace(line[colon+1:])
		if key == "" {
			continue
		}

		switch {
		case isBlockScalarIndicator(val):
			// YAML block scalar: `key: >` (folded) or `key: |` (literal), optionally
			// with a chomping indicator (-/+). The value is the indented lines that
			// follow, common in community SKILL.md descriptions.
			folded := strings.HasPrefix(val, ">")
			text, consumed := collectBlockScalar(lines[i+1:], folded)
			fm.scalars[key] = text
			i += consumed
		case val == "":
			// Begin a block list (or an empty value); record the key so following
			// "- item" lines attach to it.
			curList = key
		case strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]"):
			fm.lists[key] = parseInlineArray(val)
		default:
			fm.scalars[key] = unquote(val)
		}
	}
	return fm, body
}

// isBlockScalarIndicator reports whether a value is a YAML block-scalar header:
// ">" (folded) or "|" (literal), optionally followed by a chomping/indentation
// indicator (-, +, or a digit).
func isBlockScalarIndicator(val string) bool {
	if val != ">" && val != "|" &&
		!strings.HasPrefix(val, "> ") && !strings.HasPrefix(val, "| ") {
		switch val {
		case ">-", ">+", "|-", "|+":
			return true
		default:
			// "|2", ">2-" etc. — first char is the style, rest are indicators.
			if len(val) >= 2 && (val[0] == '>' || val[0] == '|') {
				for _, c := range val[1:] {
					if c != '-' && c != '+' && (c < '0' || c > '9') {
						return false
					}
				}
				return true
			}
			return false
		}
	}
	return val == ">" || val == "|"
}

// collectBlockScalar consumes the indented continuation lines of a block scalar,
// returning the joined text and how many lines were consumed. Folded (>) joins
// non-blank lines with single spaces (blank lines separate paragraphs); literal
// (|) preserves line breaks. De-indents to the first content line's indentation.
func collectBlockScalar(rest []string, folded bool) (string, int) {
	indent := -1
	var out []string
	consumed := 0
	for _, ln := range rest {
		if strings.TrimSpace(ln) == "" {
			out = append(out, "")
			consumed++
			continue
		}
		lead := len(ln) - len(strings.TrimLeft(ln, " \t"))
		if indent < 0 {
			if lead == 0 {
				break // not indented → block scalar had no body
			}
			indent = lead
		}
		if lead < indent {
			break // dedent → end of the block scalar
		}
		out = append(out, ln[indent:])
		consumed++
	}
	// Trim trailing blank lines that belong to the block.
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	if folded {
		// Fold: join non-blank lines with single spaces (callers one-line scalars
		// anyway, so paragraph breaks collapse to a space too).
		var parts []string
		for _, ln := range out {
			if strings.TrimSpace(ln) != "" {
				parts = append(parts, strings.TrimSpace(ln))
			}
		}
		return strings.Join(parts, " "), consumed
	}
	return strings.Join(out, "\n"), consumed
}

// parseInlineArray parses `[a, "b", 'c']` into its trimmed, unquoted items.
func parseInlineArray(s string) []string {
	inner := strings.TrimSpace(s)
	inner = strings.TrimPrefix(inner, "[")
	inner = strings.TrimSuffix(inner, "]")
	if strings.TrimSpace(inner) == "" {
		return nil
	}
	parts := strings.Split(inner, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if item := unquote(strings.TrimSpace(p)); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// setFrontmatterAccess rewrites a SKILL.md's frontmatter so its access mode
// matches shared. Existing `access:`/`shared:` lines are removed; when shared,
// a single `access: shared` line is added. All other frontmatter lines and the
// markdown body are preserved. Newlines are normalised to "\n".
func setFrontmatterAccess(content string, shared bool) string {
	norm := strings.ReplaceAll(content, "\r\n", "\n")

	var fmLines []string
	body := norm
	hadBlock := false
	if strings.HasPrefix(norm, "---\n") {
		rest := norm[len("---\n"):]
		if end := strings.Index(rest, "\n---"); end >= 0 {
			hadBlock = true
			block := rest[:end]
			body = strings.TrimPrefix(rest[end+len("\n---"):], "\n") // body after fence
			for _, ln := range strings.Split(block, "\n") {
				key := ""
				if i := strings.Index(ln, ":"); i >= 0 {
					key = strings.ToLower(strings.TrimSpace(ln[:i]))
				}
				if key == "access" || key == "shared" {
					continue // drop any existing access marker
				}
				fmLines = append(fmLines, ln)
			}
		}
	}
	if shared {
		fmLines = append(fmLines, "access: shared")
	}

	// No frontmatter needed (no prior block and nothing to add) → leave body as-is.
	if !hadBlock && len(fmLines) == 0 {
		return body
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(strings.Join(fmLines, "\n"))
	b.WriteString("\n---\n")
	b.WriteString(body)
	return b.String()
}

// setFrontmatterAutoSummary rewrites a SKILL.md's frontmatter so its auto-summary
// mode matches on. Auto-summary defaults to ON, so an enabled skill carries NO
// marker: any existing `auto_summary:` (and aliases) line is removed, and only
// when on is false a single `auto_summary: false` line is added. All other
// frontmatter lines and the markdown body are preserved.
func setFrontmatterAutoSummary(content string, on bool) string {
	norm := strings.ReplaceAll(content, "\r\n", "\n")

	var fmLines []string
	body := norm
	hadBlock := false
	if strings.HasPrefix(norm, "---\n") {
		rest := norm[len("---\n"):]
		if end := strings.Index(rest, "\n---"); end >= 0 {
			hadBlock = true
			block := rest[:end]
			body = strings.TrimPrefix(rest[end+len("\n---"):], "\n")
			for _, ln := range strings.Split(block, "\n") {
				key := ""
				if i := strings.Index(ln, ":"); i >= 0 {
					key = strings.ToLower(strings.TrimSpace(ln[:i]))
				}
				switch key {
				case "auto_summary", "autosummary", "auto_include", "autoinclude":
					continue // drop any existing auto-summary marker
				}
				fmLines = append(fmLines, ln)
			}
		}
	}
	if !on {
		fmLines = append(fmLines, "auto_summary: false")
	}

	if !hadBlock && len(fmLines) == 0 {
		return body
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(strings.Join(fmLines, "\n"))
	b.WriteString("\n---\n")
	b.WriteString(body)
	return b.String()
}

// fmField is one ordered frontmatter scalar to write. An empty Val removes the
// key (used by setFrontmatterFields).
type fmField struct{ Key, Val string }

// setFrontmatterFields rewrites a SKILL.md's frontmatter, applying the given
// scalar field updates (in order) and optionally replacing the markdown body.
// A field with an empty Val removes that key; a non-empty Val upserts it. Keys
// are matched case-insensitively. Existing block-list lines (e.g. subskills) and
// any keys not listed are preserved in place; managed keys are re-emitted at the
// end in the given order. When body is non-nil it replaces the body. Newlines
// are normalised to "\n".
func setFrontmatterFields(content string, fields []fmField, body *string) string {
	norm := strings.ReplaceAll(content, "\r\n", "\n")

	managed := make(map[string]bool, len(fields))
	for _, f := range fields {
		managed[strings.ToLower(strings.TrimSpace(f.Key))] = true
	}

	var kept []string
	curBody := norm
	if strings.HasPrefix(norm, "---\n") {
		rest := norm[len("---\n"):]
		if end := strings.Index(rest, "\n---"); end >= 0 {
			block := rest[:end]
			curBody = strings.TrimPrefix(rest[end+len("\n---"):], "\n")
			skipList := false // dropping block-list items of a managed key
			for _, ln := range strings.Split(block, "\n") {
				if strings.HasPrefix(strings.TrimSpace(ln), "- ") {
					if !skipList {
						kept = append(kept, ln)
					}
					continue
				}
				skipList = false
				key := ""
				if i := strings.Index(ln, ":"); i >= 0 {
					key = strings.ToLower(strings.TrimSpace(ln[:i]))
				}
				if key != "" && managed[key] {
					skipList = true // also drop any items that belonged to it
					continue
				}
				kept = append(kept, ln)
			}
		}
	}
	if body != nil {
		curBody = *body
	}

	out := append([]string(nil), kept...)
	for _, f := range fields {
		if strings.TrimSpace(f.Val) == "" {
			continue
		}
		out = append(out, f.Key+": "+quoteYAML(f.Val))
	}

	bodyText := strings.TrimLeft(curBody, "\n")
	if len(out) == 0 {
		// No frontmatter to write — return the body alone.
		return bodyText
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(strings.Join(out, "\n"))
	b.WriteString("\n---\n\n")
	b.WriteString(bodyText)
	return b.String()
}

// quoteYAML wraps a scalar in double quotes when it could otherwise be misread
// by the minimal frontmatter parser (contains a colon/hash, leading/trailing
// space, or a YAML indicator at the start). Embedded double quotes are swapped
// for single quotes since the parser's unquote does not handle escapes.
func quoteYAML(v string) string {
	if v == "" {
		return `""`
	}
	needs := strings.ContainsAny(v, ":#\n\"'") ||
		v != strings.TrimSpace(v) ||
		hasYAMLIndicatorPrefix(v)
	if !needs {
		return v
	}
	return `"` + strings.ReplaceAll(v, `"`, `'`) + `"`
}

// hasYAMLIndicatorPrefix reports whether v starts with a YAML indicator char
// that would change how the parser reads the line.
func hasYAMLIndicatorPrefix(v string) bool {
	if v == "" {
		return false
	}
	switch v[0] {
	case '[', '{', '&', '*', '!', '@', '`', '|', '>', '%', '-', '?':
		return true
	}
	return false
}

// unquote strips a single matching pair of surrounding quotes.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
