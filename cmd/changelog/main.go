// Command changelog turns conventional commits into a release changelog
// (see internal/changelog).
//
//	changelog render <tag>    print the release section as Markdown
//	changelog json <tag>      print the release as release.json
//	changelog write <tag>     prepend the section to _Docs/CHANGELOG.md and
//	                          write release.json next to it
//
// The range is <previous version tag>..<tag>; the first release covers the
// whole history. Run it from inside the repository. Errors exit 1.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/changelog"
)

// changelogPath and releasePath are where `write` puts its output, relative to
// the repository root.
const (
	changelogPath = "_Docs/CHANGELOG.md"
	releasePath   = "_Docs/release.json"
)

func main() {
	if len(os.Args) != 3 {
		usage()
	}
	action, tag := os.Args[1], os.Args[2]

	release, err := changelog.Build(".", tag)
	if err != nil {
		fail(err)
	}

	switch action {
	case "render":
		fmt.Print(changelog.RenderMarkdown(release))
	case "json":
		out, err := json.MarshalIndent(release, "", "  ")
		if err != nil {
			fail(err)
		}
		fmt.Println(string(out))
	case "write":
		if err := write(release); err != nil {
			fail(err)
		}
		fmt.Printf("wrote %s and %s for %s (%d commits since %s)\n",
			changelogPath, releasePath, release.Tag, release.Count, previousLabel(release))
	default:
		usage()
	}
}

// write prepends the rendered section to the changelog and replaces
// release.json with this release. release.json always describes the LATEST
// release only — it is the machine-readable handoff to the announcement
// fan-out, not an archive; the archive is the changelog itself.
func write(r changelog.Release) error {
	existing, err := os.ReadFile(changelogPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	body := changelog.Prepend(string(existing), changelog.RenderMarkdown(r), r.Tag)
	if err := os.MkdirAll(filepath.Dir(changelogPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(changelogPath, []byte(body), 0o644); err != nil {
		return err
	}

	out, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(releasePath, append(out, '\n'), 0o644)
}

// previousLabel names the range start for the summary line.
func previousLabel(r changelog.Release) string {
	if r.Previous == "" {
		return "the beginning of history"
	}
	return r.Previous
}

func usage() {
	fmt.Fprintf(os.Stderr, `usage: changelog <render|json|write> <tag>

  render <tag>   print the release section as Markdown
  json   <tag>   print the release as release.json
  write  <tag>   prepend to %s and write %s

Recognised conventional types: %s.
Anything else is grouped under "Other" rather than dropped.
`, changelogPath, releasePath, strings.Join(changelog.SortedTypes(), ", "))
	os.Exit(2)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "changelog:", err)
	os.Exit(1)
}
