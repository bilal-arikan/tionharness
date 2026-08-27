package api

import "testing"

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.1.0", "1.0.9", 1},
		{"2.0.0", "1.99.99", 1},
		{"0.1.0", "0.2.0", -1},
		{"v1.2.3", "1.2.3", 0},        // leading v is cosmetic
		{"1.2.3+build.9", "1.2.3", 0}, // build metadata does not affect precedence
		{"0.0.1-test", "0.0.1", -1},   // a prerelease sorts BEFORE its release
		{"0.0.1", "0.0.1-test", 1},    // ...and the release after it
		{"0.0.1-test", "0.0.1-test", 0},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha", 1},     // more identifiers = higher
		{"1.0.0-alpha.2", "1.0.0-alpha.10", -1}, // numeric identifiers compare numerically
		{"1.0.0-1", "1.0.0-alpha", -1},          // numeric < alphanumeric
		{"0.0.1-test", "0.0.0", 1},              // prerelease still beats a lower core
	}
	for _, tc := range cases {
		a, err := parseSemver(tc.a)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.a, err)
		}
		b, err := parseSemver(tc.b)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.b, err)
		}
		if got := compareSemver(a, b); got != tc.want {
			t.Errorf("compare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestParseSemverRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"", "dev", "1", "1.2", "1.2.3.4", "1.x.0", "-1.0.0", "banana"} {
		if _, err := parseSemver(bad); err == nil {
			t.Errorf("parseSemver(%q) succeeded, want an error", bad)
		}
	}
}

func TestVersionIsNewer(t *testing.T) {
	newer, err := versionIsNewer("0.0.1", "0.0.1-test")
	if err != nil || !newer {
		t.Fatalf("0.0.1 over 0.0.1-test: newer=%v err=%v, want true/nil", newer, err)
	}
	if _, err := versionIsNewer("0.2.0", "dev"); err == nil {
		t.Fatalf("comparing against an unparsable current version must error")
	}
}
