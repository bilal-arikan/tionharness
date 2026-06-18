package api

import (
	"net/http"
	"runtime"
	"runtime/debug"
)

// BuildVersion, BuildDate, and BuildCommit are injected at build time via
//
//	go build -ldflags "-X github.com/bilal/swarmgo/internal/api.BuildVersion=v1.2.3 ..."
//
// When building without ldflags (dev mode) all three default to "dev".
var (
	BuildVersion = "dev"
	BuildDate    = "dev"
	BuildCommit  = "dev"
)

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
	module := "github.com/bilal/swarmgo"
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Path != "" {
			module = info.Main.Path
		}
		// If version/commit were not injected via ldflags, try VCS settings from
		// the build info (populated by `go build` inside a git working tree).
		if BuildVersion == "dev" {
			for _, s := range info.Settings {
				switch s.Key {
				case "vcs.revision":
					if len(s.Value) >= 7 {
						BuildCommit = s.Value[:7]
					}
				case "vcs.time":
					if BuildDate == "dev" {
						BuildDate = s.Value
					}
				}
			}
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
