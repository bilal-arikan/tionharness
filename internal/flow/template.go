package flow

import (
	"regexp"
	"strconv"
	"strings"
)

// Vars are the values a template may reference.
type Vars struct {
	Input   string            // the turn's input ({{input}})
	Last    string            // the previous node's output ({{last}})
	Outputs map[string]string // every node's latest output ({{node.<id>}})
	Visit   int               // how many times the current node has run, 1-based ({{visit}})
	Step    int               // the run's step counter, 1-based ({{step}})
}

var placeholderRe = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_.\-]*)\s*\}\}`)

// Render substitutes {{input}}, {{last}}, {{node.<id>}}, {{visit}} and {{step}}.
// An unknown placeholder is left verbatim so a typo is visible in the output
// instead of silently vanishing. A referenced node that has not run yet
// renders as an empty string.
func Render(tpl string, v Vars) string {
	return placeholderRe.ReplaceAllStringFunc(tpl, func(m string) string {
		key := strings.TrimSpace(m[2 : len(m)-2])
		switch key {
		case "input":
			return v.Input
		case "last":
			return v.Last
		case "visit":
			return strconv.Itoa(v.Visit)
		case "step":
			return strconv.Itoa(v.Step)
		}
		if id, ok := strings.CutPrefix(key, "node."); ok {
			return v.Outputs[id]
		}
		return m
	})
}
