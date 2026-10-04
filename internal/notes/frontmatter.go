package notes

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// The on-disk form is Markdown with a leading frontmatter block:
//
//	---
//	id: NOTE12
//	kind: lesson
//	title: "Paths with spaces need quoting"
//	scope: workspace
//	tags: ["shell", "windows"]
//	...
//	---
//	body
//
// Scalars that contain characters a YAML reader would misparse (a colon, a hash,
// quotes, leading/trailing spaces) are written as JSON strings; lists are always
// JSON arrays. The reader accepts plain scalars, JSON strings and JSON arrays, so
// a hand-edited file with an unquoted title still loads. This is a deliberately
// small subset of YAML: every field here is flat, and a full YAML dependency
// would buy nothing but surface.

const fence = "---"

// Marshal renders a note as a frontmatter document.
func Marshal(n Note) []byte {
	var b strings.Builder
	b.WriteString(fence + "\n")
	put := func(k, v string) {
		if v == "" {
			return
		}
		b.WriteString(k + ": " + scalar(v) + "\n")
	}
	putList := func(k string, v []string) {
		if len(v) == 0 {
			return
		}
		js, _ := json.Marshal(v)
		b.WriteString(k + ": " + string(js) + "\n")
	}
	putInt := func(k string, v int64) {
		if v == 0 {
			return
		}
		b.WriteString(k + ": " + strconv.FormatInt(v, 10) + "\n")
	}
	putBool := func(k string, v bool) {
		if v {
			b.WriteString(k + ": true\n")
		}
	}
	put("id", n.ID)
	put("kind", string(n.Kind))
	put("title", n.Title)
	put("scope", string(n.Scope))
	putList("agents", n.Agents)
	putList("projects", n.Projects)
	put("confidence", string(n.Confidence))
	put("verification", n.Verification)
	put("supersedes", n.Supersedes)
	put("superseded_by", n.SupersededBy)
	putInt("created", n.Created)
	putInt("updated", n.Updated)
	put("source_session", n.SourceSession)
	put("source_agent", n.SourceAgent)
	put("source", n.Source)
	put("signature", n.Signature)
	putInt("occurrences", int64(n.Occurrences))
	putList("tags", n.Tags)
	putBool("private", n.Private)
	putBool("archived", n.Archived)
	b.WriteString(fence + "\n")
	body := strings.TrimRight(n.Body, "\n")
	if body != "" {
		b.WriteString(body)
		b.WriteString("\n")
	}
	return []byte(b.String())
}

// scalar renders a frontmatter value: plain when it is safe, a JSON string
// otherwise.
func scalar(v string) string {
	safe := v != "" && !strings.ContainsAny(v, ":#\"'\n\r\t[]{}") &&
		strings.TrimSpace(v) == v && !strings.HasPrefix(v, "-") &&
		!looksLikeOtherType(v)
	if safe {
		return v
	}
	js, _ := json.Marshal(v)
	return string(js)
}

// looksLikeOtherType guards plain scalars that a YAML reader would coerce to a
// number or boolean; those are quoted so the title "42" stays a string.
func looksLikeOtherType(v string) bool {
	switch strings.ToLower(v) {
	case "true", "false", "null", "yes", "no", "~":
		return true
	}
	if _, err := strconv.ParseFloat(v, 64); err == nil {
		return true
	}
	return false
}

// Unmarshal parses a frontmatter document. A document without a frontmatter
// block is an error: every note the store writes has one, so its absence means
// the file is not a note.
func Unmarshal(data []byte) (Note, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimPrefix(text, "\xEF\xBB\xBF")
	if !strings.HasPrefix(text, fence+"\n") && text != fence {
		return Note{}, fmt.Errorf("notes: missing frontmatter")
	}
	rest := strings.TrimPrefix(text, fence+"\n")
	end := strings.Index(rest, "\n"+fence+"\n")
	var head, body string
	switch {
	case end >= 0:
		head = rest[:end]
		body = rest[end+len("\n"+fence+"\n"):]
	case strings.HasSuffix(rest, "\n"+fence):
		head = strings.TrimSuffix(rest, "\n"+fence)
	case rest == fence || rest == fence+"\n":
		head = ""
	default:
		return Note{}, fmt.Errorf("notes: unterminated frontmatter")
	}
	var n Note
	sc := bufio.NewScanner(strings.NewReader(head))
	sc.Buffer(make([]byte, 0, 64*1024), MaxBodyBytes+64*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return Note{}, fmt.Errorf("notes: bad frontmatter line %q", line)
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		switch k {
		case "id":
			n.ID = str(v)
		case "kind":
			n.Kind = Kind(str(v))
		case "title":
			n.Title = str(v)
		case "scope":
			n.Scope = Scope(str(v))
		case "agents":
			n.Agents = list(v)
		case "projects":
			n.Projects = list(v)
		case "confidence":
			n.Confidence = Confidence(str(v))
		case "verification":
			n.Verification = str(v)
		case "supersedes":
			n.Supersedes = str(v)
		case "superseded_by", "supersededby":
			n.SupersededBy = str(v)
		case "created":
			n.Created = integer(v)
		case "updated":
			n.Updated = integer(v)
		case "source_session", "sourcesession":
			n.SourceSession = str(v)
		case "source_agent", "sourceagent":
			n.SourceAgent = str(v)
		case "source":
			n.Source = str(v)
		case "signature":
			n.Signature = str(v)
		case "occurrences":
			n.Occurrences = int(integer(v))
		case "tags":
			n.Tags = list(v)
		case "private":
			n.Private = boolean(v)
		case "archived":
			n.Archived = boolean(v)
		default:
			// Unknown keys are tolerated so a hand-added field does not make the
			// note unreadable; they are dropped on the next write.
		}
	}
	if err := sc.Err(); err != nil {
		return Note{}, fmt.Errorf("notes: frontmatter: %w", err)
	}
	n.Body = strings.TrimRight(body, "\n")
	n.Normalize()
	return n, nil
}

// str decodes a scalar: a JSON string when quoted, the raw text otherwise
// (single quotes are stripped as a courtesy to hand edits).
func str(v string) string {
	if strings.HasPrefix(v, `"`) {
		var s string
		if err := json.Unmarshal([]byte(v), &s); err == nil {
			return s
		}
		return strings.Trim(v, `"`)
	}
	if len(v) >= 2 && strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'") {
		return v[1 : len(v)-1]
	}
	return v
}

// list decodes a JSON array, or a comma-separated plain list as a fallback.
func list(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	if strings.HasPrefix(v, "[") {
		var out []string
		if err := json.Unmarshal([]byte(v), &out); err == nil {
			return out
		}
		v = strings.Trim(v, "[]")
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = str(strings.TrimSpace(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func integer(v string) int64 {
	i, _ := strconv.ParseInt(strings.Trim(str(v), `"`), 10, 64)
	return i
}

func boolean(v string) bool {
	switch strings.ToLower(str(v)) {
	case "true", "yes", "1":
		return true
	}
	return false
}
