package archive

import (
	"errors"
	"strings"
	"testing"
)

func TestParseFilter(t *testing.T) {
	cases := map[string]Filter{
		"": All, "true": Only, "1": Only, "only": Only, "TRUE": Only,
		"false": Active, "0": Active, "active": Active, "all": All,
	}
	for in, want := range cases {
		got, err := ParseFilter(in, All)
		if err != nil || got != want {
			t.Errorf("ParseFilter(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if got, _ := ParseFilter("", Active); got != Active {
		t.Errorf("empty value must yield the default, got %v", got)
	}
	if _, err := ParseFilter("maybe", Active); err == nil {
		t.Error("an unknown value must be an error, not a silent default")
	}
}

func TestFilterKeepAndApply(t *testing.T) {
	type item struct {
		id       string
		archived bool
	}
	items := []item{{"a", false}, {"b", true}, {"c", false}}
	ids := func(f Filter) string {
		var b strings.Builder
		for _, it := range Apply(items, f, func(it item) bool { return it.archived }) {
			b.WriteString(it.id)
		}
		return b.String()
	}
	if got := ids(Active); got != "ac" {
		t.Errorf("Active = %q", got)
	}
	if got := ids(Only); got != "b" {
		t.Errorf("Only = %q", got)
	}
	if got := ids(All); got != "abc" {
		t.Errorf("All = %q", got)
	}
	if FromBool(true) != Only || FromBool(false) != Active {
		t.Error("FromBool must follow the list_tasks convention")
	}
}

func TestErrorWrapsAndNames(t *testing.T) {
	err := Error("agent", "AGT1", "Ada")
	if !errors.Is(err, ErrArchived) {
		t.Fatal("Error must wrap ErrArchived")
	}
	if msg := err.Error(); !strings.Contains(msg, "Ada (AGT1)") || !strings.Contains(msg, "unarchive") {
		t.Fatalf("message must name the entity and the fix: %q", msg)
	}
	if msg := Error("skill", "s", "").Error(); !strings.Contains(msg, "skill s is archived") {
		t.Fatalf("id-only label: %q", msg)
	}
}
