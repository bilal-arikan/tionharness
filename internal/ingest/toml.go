package ingest

import "strings"

// parseSimpleTOML extracts top-level `key = value` pairs from a minimal TOML
// document — enough for Claude Code slash commands (description = "…", prompt =
// """…""") without pulling in a TOML dependency (go.mod stays at uuid+cron). It
// supports single-/double-quoted single-line values, plus multi-line values
// opened by either triple-quote delimiter:
//
//	"""…"""
//	'''…'''
//
// Tables ([section]) and arrays are ignored. Keys are lower-cased; later
// duplicates win.
func parseSimpleTOML(raw string) map[string]string {
	out := map[string]string{}
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:eq]))
		val := strings.TrimSpace(line[eq+1:])
		if key == "" {
			continue
		}
		if open := tripleQuote(val); open != "" {
			// Multi-line value: accumulate until the closing triple-quote.
			rest := strings.TrimPrefix(val, open)
			if end := strings.Index(rest, open); end >= 0 {
				out[key] = rest[:end] // closes on the same line
				continue
			}
			var b strings.Builder
			b.WriteString(rest)
			i++
			for ; i < len(lines); i++ {
				if end := strings.Index(lines[i], open); end >= 0 {
					b.WriteString("\n")
					b.WriteString(lines[i][:end])
					break
				}
				b.WriteString("\n")
				b.WriteString(lines[i])
			}
			out[key] = strings.Trim(b.String(), "\n")
			continue
		}
		out[key] = unquoteTOML(val)
	}
	return out
}

// tripleQuote returns the triple-quote delimiter a value opens with:
//
//	"""
//	'''
//
// or "" when it isn't a multi-line string.
func tripleQuote(v string) string {
	switch {
	case strings.HasPrefix(v, `"""`):
		return `"""`
	case strings.HasPrefix(v, "'''"):
		return "'''"
	}
	return ""
}

// unquoteTOML strips a single-line value's surrounding quotes (and a trailing inline
// comment for bare values), returning the literal text.
func unquoteTOML(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			inner := v[1 : len(v)-1]
			if v[0] == '"' {
				inner = strings.ReplaceAll(inner, `\"`, `"`)
				inner = strings.ReplaceAll(inner, `\n`, "\n")
				inner = strings.ReplaceAll(inner, `\t`, "\t")
			}
			return inner
		}
	}
	// Bare value: drop a trailing inline comment.
	if i := strings.IndexByte(v, '#'); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v
}
