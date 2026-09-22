package treepin

import (
	"fmt"
	"strconv"
	"strings"
)

// tokenPrefix versions the text form so a future layout change is detected
// instead of silently misparsed.
const tokenPrefix = "treepin1"

// String renders the pin as one line a coordinator can paste into a validator
// brief: treepin1 head=<sha> files=<n> digest=<sha256> scope=<p1>,<p2>
// Scope goes last so it may contain spaces; commas separate pathspecs.
func (p Pin) String() string {
	return fmt.Sprintf("%s head=%s files=%d digest=%s scope=%s", tokenPrefix, p.Head, p.Files, p.Digest, strings.Join(p.Scope, ","))
}

// Parse reads a pin rendered by Pin.String.
func Parse(s string) (Pin, error) {
	s = strings.TrimSpace(s)
	rest, ok := strings.CutPrefix(s, tokenPrefix+" ")
	if !ok {
		return Pin{}, fmt.Errorf("treepin: not a %s token: %q", tokenPrefix, s)
	}
	var p Pin
	for _, key := range []string{"head", "files", "digest"} {
		field, tail, _ := strings.Cut(rest, " ")
		val, ok := strings.CutPrefix(field, key+"=")
		if !ok {
			return Pin{}, fmt.Errorf("treepin: expected %s= in %q", key, s)
		}
		switch key {
		case "head":
			p.Head = val
		case "files":
			n, err := strconv.Atoi(val)
			if err != nil {
				return Pin{}, fmt.Errorf("treepin: bad files count %q", val)
			}
			p.Files = n
		case "digest":
			p.Digest = val
		}
		rest = tail
	}
	scope, ok := strings.CutPrefix(rest, "scope=")
	if !ok {
		return Pin{}, fmt.Errorf("treepin: expected scope= in %q", s)
	}
	p.Scope = cleanScope(strings.Split(scope, ","))
	if p.Head == "" || p.Digest == "" || len(p.Scope) == 0 {
		return Pin{}, fmt.Errorf("treepin: incomplete token %q", s)
	}
	return p, nil
}
