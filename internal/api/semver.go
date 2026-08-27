package api

import (
	"fmt"
	"strconv"
	"strings"
)

// Minimal semantic-version comparison.
//
// The update check only ever needs "is the feed's version newer than mine?", so
// a full SemVer library would be a new module dependency for one comparison.
// The rules implemented here are the ones that actually decide that answer:
// numeric major/minor/patch ordering, and the prerelease rule — a prerelease
// sorts BEFORE its own release, so 0.0.1-test < 0.0.1. Without that rule a
// tester running 0.0.1-test would be told 0.0.1 is not an update.

type semver struct {
	major, minor, patch int
	// prerelease holds the dot-separated identifiers after '-', empty for a
	// final release. Build metadata (after '+') is dropped: SemVer says it takes
	// no part in precedence.
	prerelease []string
}

// parseSemver accepts an optional leading "v" and requires the three numeric
// core fields. Anything else is an error rather than a silent zero value: a
// version we cannot read must make the check "unknown", not "you are up to
// date".
func parseSemver(s string) (semver, error) {
	v := strings.TrimSpace(s)
	v = strings.TrimPrefix(v, "v")
	if v == "" {
		return semver{}, fmt.Errorf("empty version")
	}
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	var pre string
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v, pre = v[:i], v[i+1:]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("version %q: want major.minor.patch", s)
	}
	out := semver{}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, fmt.Errorf("version %q: field %d is not a number", s, i+1)
		}
		switch i {
		case 0:
			out.major = n
		case 1:
			out.minor = n
		case 2:
			out.patch = n
		}
	}
	if pre != "" {
		out.prerelease = strings.Split(pre, ".")
	}
	return out, nil
}

// compareSemver returns -1 if a < b, 0 if equal, +1 if a > b.
func compareSemver(a, b semver) int {
	for _, pair := range [][2]int{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if pair[0] != pair[1] {
			if pair[0] < pair[1] {
				return -1
			}
			return 1
		}
	}
	return comparePrerelease(a.prerelease, b.prerelease)
}

// comparePrerelease implements the SemVer precedence rules for the prerelease
// segment: having one makes a version LOWER than the same core without one, and
// identifiers are compared left to right (numeric ones numerically, and a
// numeric identifier always sorts below an alphanumeric one).
func comparePrerelease(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return 1 // release > prerelease
	case len(b) == 0:
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := compareIdentifier(a[i], b[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

func compareIdentifier(a, b string) int {
	an, aErr := strconv.Atoi(a)
	bn, bErr := strconv.Atoi(b)
	switch {
	case aErr == nil && bErr == nil:
		switch {
		case an < bn:
			return -1
		case an > bn:
			return 1
		}
		return 0
	case aErr == nil:
		return -1 // numeric identifiers have lower precedence
	case bErr == nil:
		return 1
	}
	return strings.Compare(a, b)
}

// versionIsNewer reports whether `latest` is a strictly newer version than
// `current`. An unparsable version on either side is an error, so the caller can
// report "unknown" instead of guessing.
func versionIsNewer(latest, current string) (bool, error) {
	l, err := parseSemver(latest)
	if err != nil {
		return false, err
	}
	c, err := parseSemver(current)
	if err != nil {
		return false, err
	}
	return compareSemver(l, c) > 0, nil
}
