package api

// Artifact selection for the update check: latest.json lists one build per
// os/arch pair, and the banner must offer the one that actually runs here.
// Getting this wrong is worse than offering nothing, so a miss falls back to the
// generic releases page instead of handing out a foreign binary.

// updateArtifact is one downloadable build in latest.json. Only os/arch/file/url
// are relied on; every other field of the published entry is optional and is not
// read here.
type updateArtifact struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
	File string `json:"file"`
	URL  string `json:"url"`
}

// selectArtifact returns the artifact built for the given platform. goos/goarch
// are the running server's own runtime values: the binary the user would replace
// is this process, not whatever machine the browser runs on. An entry without a
// URL is unusable and is skipped rather than returned as a dead link.
func selectArtifact(list []updateArtifact, goos, goarch string) (updateArtifact, bool) {
	for _, a := range list {
		if a.OS == goos && a.Arch == goarch && a.URL != "" {
			return a, true
		}
	}
	return updateArtifact{}, false
}

// releasesURL is the generic landing page used when no artifact matches this
// platform — the user still gets somewhere useful, just without a direct file.
func releasesURL() string {
	return FeedURL() + "/releases"
}
