package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// mermaidValidateInput is the ask shape for the mermaid_validate tool.
type mermaidValidateInput struct {
	Code string `json:"code"`
}

// knownMermaidTypes are the diagram headers mermaid recognises. The first
// meaningful line of a diagram must start with one of these.
var knownMermaidTypes = []string{
	"graph", "flowchart", "sequenceDiagram", "stateDiagram-v2", "stateDiagram",
	"classDiagram", "erDiagram", "journey", "gantt", "pie", "gitGraph",
	"mindmap", "timeline", "xychart-beta", "quadrantChart", "requirementDiagram",
	"C4Context", "C4Container", "C4Component", "sankey-beta", "block-beta",
	"packet-beta", "architecture-beta",
}

// MermaidValidateTool lints Mermaid diagram source. SwarmGo is pure Go with no JS
// engine, so this is a SYNTAX LINT, not a full parse: it checks the diagram has a
// recognised type header and that brackets/quotes are balanced — the failures that
// most often make a diagram fail to render. It cannot catch every semantic error a
// real Mermaid parser would. The optional `render` flag is accepted for schema
// parity with other agents but ignored (no renderer here).
type MermaidValidateTool struct{}

// NewMermaidValidateTool constructs the mermaid_validate tool.
func NewMermaidValidateTool() MermaidValidateTool { return MermaidValidateTool{} }

func (MermaidValidateTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "mermaid_validate",
		Description: "Lint Mermaid diagram source before you emit it. Lightweight syntax check " +
			"(recognised diagram type + balanced brackets/quotes), NOT a full parser — it catches the " +
			"common breakages but not every semantic error. Returns the detected diagram type plus any " +
			"errors/warnings.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "code": { "type": "string", "description": "The Mermaid diagram source to validate." },
    "render": { "type": "boolean", "description": "Ignored (no renderer); accepted for parity." }
  },
  "required": ["code"],
  "additionalProperties": false
}`),
	}
}

func (MermaidValidateTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var in mermaidValidateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("mermaid_validate", err)
	}
	code := strings.TrimSpace(in.Code)
	if code == "" {
		return "", fmt.Errorf("code is empty")
	}

	var errs, warns []string

	// 1) Diagram type header: first meaningful line (skip blank + %% comments and
	// an optional ```mermaid fence) must start with a known type.
	dtype := ""
	for _, raw := range strings.Split(code, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") || strings.HasPrefix(line, "```") {
			continue
		}
		for _, t := range knownMermaidTypes {
			if line == t || strings.HasPrefix(line, t+" ") || strings.HasPrefix(line, t+"\t") {
				dtype = t
				break
			}
		}
		if dtype == "" {
			errs = append(errs, fmt.Sprintf("first line %q does not start with a recognised diagram type (e.g. graph, flowchart, sequenceDiagram, stateDiagram-v2, classDiagram, erDiagram, xychart-beta)", line))
		}
		break
	}

	// 2) Balanced brackets and quotes across the whole source.
	if msg, ok := checkBalanced(code); !ok {
		errs = append(errs, msg)
	}

	// 3) Soft check: a single-line diagram with a type but no content is suspicious.
	if dtype != "" && len(strings.Split(code, "\n")) == 1 {
		warns = append(warns, "diagram has a type header but no nodes/edges on following lines")
	}

	valid := len(errs) == 0
	var b strings.Builder
	if valid {
		fmt.Fprintf(&b, "VALID — Mermaid syntax lint passed (type: %s).", dtypeOr(dtype))
	} else {
		fmt.Fprintf(&b, "INVALID — %d issue(s):", len(errs))
		for _, e := range errs {
			fmt.Fprintf(&b, "\n- %s", e)
		}
	}
	for _, w := range warns {
		fmt.Fprintf(&b, "\n(warning) %s", w)
	}
	b.WriteString("\nNote: lint only — a full Mermaid parser may catch additional semantic errors.")
	return b.String(), nil
}

func dtypeOr(t string) string {
	if t == "" {
		return "unknown"
	}
	return t
}

// checkBalanced reports the first bracket/quote imbalance found. Double quotes are
// treated as toggling string context so brackets inside labels are ignored.
func checkBalanced(code string) (string, bool) {
	pairs := map[rune]rune{')': '(', ']': '[', '}': '{'}
	open := map[rune]bool{'(': true, '[': true, '{': true}
	var stack []rune
	inStr := false
	for _, r := range code {
		if r == '"' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		switch {
		case open[r]:
			stack = append(stack, r)
		case pairs[r] != 0:
			if len(stack) == 0 || stack[len(stack)-1] != pairs[r] {
				return fmt.Sprintf("unbalanced bracket: unexpected %q", string(r)), false
			}
			stack = stack[:len(stack)-1]
		}
	}
	if inStr {
		return "unbalanced quotes: an opening '\"' has no closing match", false
	}
	if len(stack) > 0 {
		return fmt.Sprintf("unbalanced bracket: %d unclosed %q", len(stack), string(stack[len(stack)-1])), false
	}
	return "", true
}
