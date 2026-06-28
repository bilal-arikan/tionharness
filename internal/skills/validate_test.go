package skills

import (
	"testing"
)

func TestValidateSkill(t *testing.T) {
	ws := t.TempDir()
	store := New("", ws)

	// Valid skill.
	writeSkill(t, ws, "good-skill", "---\nname: \"Good\"\ndescription: \"A well-formed skill for testing.\"\n---\n\nDo the thing.\n")
	if r := store.ValidateSkill("good-skill"); !r.Valid {
		t.Fatalf("good-skill should be valid, got errors=%v", r.Errors)
	}

	// Missing description.
	writeSkill(t, ws, "no-desc", "---\nname: \"NoDesc\"\n---\n\nBody here.\n")
	r := store.ValidateSkill("no-desc")
	if r.Valid || !hasErr(r.Errors, "description") {
		t.Fatalf("no-desc should fail on description, got %+v", r)
	}

	// Empty body.
	writeSkill(t, ws, "no-body", "---\nname: \"NoBody\"\ndescription: \"Has frontmatter but no body content.\"\n---\n")
	r = store.ValidateSkill("no-body")
	if r.Valid || !hasErr(r.Errors, "body") {
		t.Fatalf("no-body should fail on empty body, got %+v", r)
	}

	// Not found.
	if r := store.ValidateSkill("ghost"); r.Found || r.Valid {
		t.Fatalf("ghost should be not-found/invalid, got %+v", r)
	}

	// Bad slug.
	if r := store.ValidateSkill("Bad_Slug"); r.Valid {
		t.Fatalf("Bad_Slug should fail slug hygiene")
	}
}

func hasErr(errs []string, substr string) bool {
	for _, e := range errs {
		if containsFold(e, substr) {
			return true
		}
	}
	return false
}

func containsFold(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexFold(s, sub) >= 0)
}

func indexFold(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if equalFold(s[i:i+len(sub)], sub) {
			return i
		}
	}
	return -1
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
