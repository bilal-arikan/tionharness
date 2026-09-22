package changelog

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

// git runs git in dir and returns trimmed stdout.
func git(dir string, args ...string) (string, error) {
	cmd := proc.Command("git", append([]string{"-c", "core.quotepath=false"}, args...)...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("changelog: git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// recordSep and fieldSep delimit `git log` output. ASCII unit/record separators
// are used instead of a newline because a commit BODY contains newlines, and a
// subject can contain very nearly anything.
const (
	recordSep = "\x1e"
	fieldSep  = "\x1f"
)

// PreviousTag returns the version tag that precedes tag, or "" when tag is the
// first one. Only tags matching the v<semver> shape are considered: a marker
// tag such as `before-rename` is not a release boundary and must not become the
// start of a changelog range.
//
// The empty result is NOT an error — the first release legitimately has no
// predecessor and its changelog covers the whole history.
func PreviousTag(dir, tag string) (string, error) {
	// --merged <tag> keeps the walk on the tagged commit's own ancestry, so a tag
	// on a parallel branch cannot become "the previous release".
	out, err := git(dir, "tag", "--list", "v*", "--sort=-v:refname", "--merged", tag)
	if err != nil {
		return "", err
	}
	var seen bool
	for line := range strings.SplitSeq(out, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || !isVersionTag(t) {
			continue
		}
		if t == tag {
			seen = true
			continue
		}
		if seen {
			return t, nil
		}
	}
	return "", nil
}

// isVersionTag reports whether t looks like v<major>.<minor>.<patch>[suffix].
// The check is deliberately loose about the suffix (prereleases are real) and
// strict about the leading v and the three numeric components.
func isVersionTag(t string) bool {
	if !strings.HasPrefix(t, "v") {
		return false
	}
	core, _, _ := strings.Cut(strings.TrimPrefix(t, "v"), "-")
	core, _, _ = strings.Cut(core, "+")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// Commits returns the commits in (from, to]. An empty from walks the whole
// history up to to, which is what the first release needs.
//
// Merge commits are excluded (--no-merges): a merge carries no change of its
// own, and "Merge branch 'x'" subjects would fill the Other section with noise.
func Commits(dir, from, to string) ([]Commit, error) {
	spec := to
	if from != "" {
		spec = from + ".." + to
	}
	format := "--pretty=format:%h" + fieldSep + "%B" + recordSep
	out, err := git(dir, "log", "--no-merges", format, spec)
	if err != nil {
		return nil, err
	}

	var commits []Commit
	for rec := range strings.SplitSeq(out, recordSep) {
		rec = strings.TrimLeft(rec, "\n")
		if strings.TrimSpace(rec) == "" {
			continue
		}
		hash, msg, ok := strings.Cut(rec, fieldSep)
		if !ok {
			// A record with no separator means the format did not survive; failing
			// loudly beats emitting a changelog that silently lost commits.
			return nil, fmt.Errorf("changelog: malformed git log record %q", rec)
		}
		commits = append(commits, parseCommit(strings.TrimSpace(hash), msg))
	}
	return commits, nil
}

// TagDate returns tag's commit date as YYYY-MM-DD, for the release header.
func TagDate(dir, tag string) (string, error) {
	return git(dir, "log", "-1", "--format=%cd", "--date=short", tag)
}
