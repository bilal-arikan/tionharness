package exttools

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/proc"
	"github.com/bilal-arikan/tionswarm/internal/textutil"
)

// lookPath is exec.LookPath behind a var so tests can stub PATH resolution.
var lookPath = exec.LookPath

// versionTimeout caps one version probe. Deliberately short: a `--version` call
// that has not answered in three seconds is not going to. It also bounds the
// damage when a tool ignores the flag and starts its real work instead — most
// notably codebase-memory-mcp, which with no recognised flag would sit as a
// stdio MCP server waiting on stdin forever.
const versionTimeout = 3 * time.Second

// semverRe matches the first version-looking token in a tool's output, e.g.
// "rtk 0.9.0", "ffmpeg version 7.1-full_build", "v1.3.0". The patch component is
// optional so a two-part version still parses.
var semverRe = regexp.MustCompile(`\bv?(\d+)\.(\d+)(?:\.(\d+))?`)

// LocalVersion runs the tool at path with its version flag and returns the
// version it printed (normalised, without a leading "v").
//
// This is the one place exttools EXECUTES a detected tool — the rest of the
// package only resolves paths. A version flag is side-effect free, the call is
// context-bounded, and the whole process tree is reaped on timeout, so the cost
// of being wrong is a killed child rather than a hung request.
//
// Errors are returned, never swallowed: the caller surfaces "version unreadable"
// instead of quietly pretending the tool is up to date.
func LocalVersion(ctx context.Context, path string, args []string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("araç yolu boş")
	}
	if len(args) == 0 {
		return "", fmt.Errorf("bu araç sürüm sorgusunu desteklemiyor")
	}

	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()

	cmd := proc.CommandContext(ctx, path, args...)
	cmd.Env = proc.HardenedEnv(nil)
	proc.TreeKill(cmd)

	// Many CLIs print the version banner on stderr, so both streams are read.
	// A non-zero exit is NOT fatal on its own: `ffmpeg -version` exits 0 but some
	// tools answer an unknown flag with usage text on stderr and exit 1 while
	// still naming their version. Parse first, report the exit error only when
	// nothing version-shaped came back.
	out, runErr := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))

	if m := semverRe.FindStringSubmatch(text); m != nil {
		return normalizeVersion(m), nil
	}
	if ctx.Err() != nil {
		return "", fmt.Errorf("sürüm sorgusu zaman aşımına uğradı (%s)", versionTimeout)
	}
	if runErr != nil {
		return "", fmt.Errorf("sürüm sorgusu başarısız: %w (%s)", runErr, truncate(text, 160))
	}
	return "", fmt.Errorf("çıktıda sürüm bulunamadı: %s", truncate(text, 160))
}

// normalizeVersion renders a semverRe match as "major.minor.patch", defaulting a
// missing patch to 0 so "1.3" and "1.3.0" compare equal.
func normalizeVersion(m []string) string {
	patch := m[3]
	if patch == "" {
		patch = "0"
	}
	return m[1] + "." + m[2] + "." + patch
}

// ParseVersion extracts the numeric components of a version string. ok is false
// when the string carries nothing version-shaped.
func ParseVersion(s string) (major, minor, patch int, ok bool) {
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, 0, false
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	if m[3] != "" {
		patch, _ = strconv.Atoi(m[3])
	}
	return major, minor, patch, true
}

func truncate(s string, n int) string {
	return textutil.TruncBytesEllipsis(strings.Join(strings.Fields(s), " "), n)
}
