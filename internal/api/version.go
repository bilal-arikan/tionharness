package api

import (
	"net/http"
	"runtime"
	"runtime/debug"
	"sync"
)

// BuildVersion, BuildDate, and BuildCommit are injected at build time via
//
//	go build -ldflags "-X github.com/bilal-arikan/tionharness/internal/api.BuildVersion=v1.2.3 ..."
//
// When building without ldflags (dev mode) all three default to "dev".
var (
	BuildVersion  = "dev"
	BuildDate     = "dev"
	BuildCommit   = "dev"
	buildInfoOnce sync.Once
)

// ResolveBuildInfo fills missing ldflags values from Go's embedded VCS metadata.
func ResolveBuildInfo() {
	buildInfoOnce.Do(func() {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				switch setting.Key {
				case "vcs.revision":
					if BuildCommit == "dev" && len(setting.Value) >= 7 {
						BuildCommit = setting.Value[:7]
					}
				case "vcs.time":
					if BuildDate == "dev" {
						BuildDate = setting.Value
					}
				}
			}
		}
	})
}

type versionResponse struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
	Module    string `json:"module"`
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	ResolveBuildInfo()
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
