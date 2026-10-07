package tools

import (
	"encoding/json"
	"testing"
)

func TestReadRefFor(t *testing.T) {
	notes := func(ref string) (string, bool) {
		if ref == "NOTE3" || ref == "Deploy checklist" {
			return "NOTE3", true
		}
		return "", false
	}
	cases := []struct {
		tool, input, want string
	}{
		{"get_view", `{"kind":"session","id":"SES9"}`, "session:SES9"},
		{"get_view", `{"kind":"board","id":"board","sub":"T3"}`, "board:board#T3"},
		{"get_view", `{"kind":"board"}`, "board:board"},
		{"read_artifact", `{"id":"ART2"}`, "artifact:ART2"},
		{"use_skill", `{"slug":"deploy"}`, "skill:deploy"},
		{"note_expand", `{"id":"Deploy checklist"}`, "note:NOTE3"},
		{"note_expand", `{"id":"missing"}`, ""},
		{"list_artifacts", `{}`, ""},
		{"get_view", `{"kind":"session"}`, ""},
	}
	for _, c := range cases {
		got, ok := ReadRefFor(c.tool, json.RawMessage(c.input), notes)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("%s %s = (%q, %v), want %q", c.tool, c.input, got, ok, c.want)
		}
	}
}
