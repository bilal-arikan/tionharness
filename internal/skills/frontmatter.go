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
	for _, raw := range lines {
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
