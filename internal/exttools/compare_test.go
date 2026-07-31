package exttools

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		local, latest, want string
	}{
		{"1.3.0", "v1.3.0", StatusUpToDate},   // tag carries a v prefix, version does not
		{"1.3", "1.3.0", StatusUpToDate},      // missing patch normalises to 0
		{"1.2.9", "v1.3.0", StatusOutdated},   // minor bump
		{"1.3.0", "v1.3.1", StatusOutdated},   // patch bump
		{"0.9.0", "v1.0.0", StatusOutdated},   // major bump
		{"2.0.0", "v1.9.9", StatusUpToDate},   // dev build ahead of release — never "downgrade available"
		{"", "v1.0.0", StatusUnknown},         // version unreadable
		{"1.0.0", "nightly", StatusUnknown},   // tag is not a version
		{"nightly", "nightly", StatusUnknown}, // neither side parses
	}
	for _, c := range cases {
		if got := Compare(c.local, c.latest); got != c.want {
			t.Fatalf("Compare(%q, %q) = %q, want %q", c.local, c.latest, got, c.want)
		}
	}
}
