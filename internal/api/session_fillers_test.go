package api

import "testing"

import "github.com/bilal-arikan/tionharness/internal/db"

// TestBuildFillersSplitsUserBucketByOrigin is the point of the split: in a
// coordinator session almost every "user" message is machine-injected worker
// output, so lumping them together reports the human as the biggest context
// filler when their actual share is a rounding error.
func TestBuildFillersSplitsUserBucketByOrigin(t *testing.T) {
	pending := []db.Message{
		{Role: "user", Text: "kısa soru"},
		{Role: "user", Text: "<task-notification>" + longText(4000) + "</task-notification>", Origin: "worker-note"},
		{Role: "user", Text: "<coordination-status>" + longText(500) + "</coordination-status>", Origin: "worker-note"},
		{Role: "user", Text: "devam et", Origin: "wake"},
		{Role: "assistant", Text: "cevap"},
	}

	got := map[string]contextFiller{}
	fillers, err := buildFillers("", pending, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fillers {
		if _, dup := got[f.Role]; dup {
			// The frontend keys its legend/bar segments on Role — a duplicate
			// would collapse two buckets into one React key.
			t.Fatalf("duplicate filler role %q", f.Role)
		}
		got[f.Role] = f
	}

	for _, role := range []string{"user", "worker-note", "auto-prompt", "assistant"} {
		if _, ok := got[role]; !ok {
			t.Fatalf("missing filler bucket %q (got %v)", role, keys(got))
		}
	}
	if got["worker-note"].Count != 2 {
		t.Errorf("worker-note count = %d, want 2", got["worker-note"].Count)
	}
	if got["user"].Count != 1 {
		t.Errorf("human user count = %d, want 1", got["user"].Count)
	}
	// The whole complaint: the human bucket must no longer carry worker weight.
	if got["user"].Tokens >= got["worker-note"].Tokens {
		t.Errorf("worker output still folded into the user bucket: user=%d worker=%d",
			got["user"].Tokens, got["worker-note"].Tokens)
	}
	if lbl := got["worker-note"].Label; lbl != "Worker sonuçları" {
		t.Errorf("worker-note label = %q", lbl)
	}
}

// TestFillerRoleForUnknownOriginStaysUser pins the fallback: a NEW Origin value
// (added later for some other injected prompt) must land in the plain user
// bucket rather than creating an unlabelled synthetic role.
func TestFillerRoleForUnknownOriginStaysUser(t *testing.T) {
	if got := fillerRoleFor(db.Message{Role: "user", Origin: "something-new"}); got != "user" {
		t.Errorf("unknown origin → %q, want \"user\"", got)
	}
	if got := fillerRoleFor(db.Message{Role: "assistant", Origin: "worker-note"}); got != "assistant" {
		t.Errorf("non-user role must ignore Origin, got %q", got)
	}
}

func longText(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}

func keys(m map[string]contextFiller) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
