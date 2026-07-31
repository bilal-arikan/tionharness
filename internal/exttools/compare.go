package exttools

// Update status values reported to the UI.
const (
	StatusUpToDate = "up-to-date"
	StatusOutdated = "outdated"
	StatusUnknown  = "unknown"
)

// Compare reports whether local is behind latest.
//
// It returns StatusUnknown whenever either side cannot be parsed — never a
// guess. Claiming "outdated" on an unparseable string would push the user into a
// pointless (and, for the manual tools, risky) reinstall; claiming "up-to-date"
// would hide a real update. Unknown is the only honest answer.
//
// A local version AHEAD of the published one is reported as up-to-date: that is
// a dev/nightly build, not something to downgrade.
func Compare(local, latest string) string {
	lMaj, lMin, lPatch, lOK := ParseVersion(local)
	rMaj, rMin, rPatch, rOK := ParseVersion(latest)
	if !lOK || !rOK {
		return StatusUnknown
	}
	for _, p := range [][2]int{{lMaj, rMaj}, {lMin, rMin}, {lPatch, rPatch}} {
		switch {
		case p[0] < p[1]:
			return StatusOutdated
		case p[0] > p[1]:
			return StatusUpToDate
		}
	}
	return StatusUpToDate
}
