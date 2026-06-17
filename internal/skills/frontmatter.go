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

// unquote strips a single matching pair of surrounding quotes.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
