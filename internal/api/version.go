package api

import (
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
)

// BuildVersion, BuildDate, and BuildCommit are injected at build time via
//
//	go build -ldflags "-X github.com/bilal-arikan/tionharness/internal/api.BuildVersion=v1.2.3 ..."
//
// When building without ldflags, the values below identify a development build.
var (
	BuildVersion = "dev"
	BuildCommit  = "unknown"
	BuildDate    = "unknown"
	FeedBaseURL  = "https://tionharness.com"
)

// FeedURL returns the update feed base URL, with an environment override for
// local and self-hosted update feeds.
func FeedURL() string {
	if value := os.Getenv("TIONHARNESS_FEED_URL"); value != "" {
		return strings.TrimRight(value, "/")
	}
	return strings.TrimRight(FeedBaseURL, "/")
}

// ResolveBuildInfo is retained for callers that initialize build metadata.
// Release metadata is supplied exclusively through linker flags.
func ResolveBuildInfo() {}

type versionResponse struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
	Module    string `json:"module"`
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	goVer := runtime.Version()

	// ReadBuildInfo fills in the module path and, when built with ldflags, VCS
	// metadata. We extract the module name so the UI can link to the source.
	module := "github.com/bilal-arikan/tionharness"
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Path != "" {
			module = info.Main.Path
		}
	}

	writeJSON(w, http.StatusOK, versionResponse{
		Version:   BuildVersion,
		Commit:    BuildCommit,
		BuildDate: BuildDate,
		GoVersion: goVer,
		Module:    module,
	})
}
