package providers

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

// opencode changed its CLI and config surface incompatibly between 1.x and 2.x
// (run flags, config schema, tool names, stream shape), so the transport keys
// its argv, opencode.json and parser behaviour off the binary's major version.
//
//	1.x (verified on 1.19.0): `run --dir`, `--variant`, legacy `tools`/`agent`/
//	    `permission` map config, tool parts stream pending/running states.
//	2.x (verified on 2.2.5): no `--dir`/`--variant` (variant rides `-m
//	    provider/model#variant`), `run` talks to a shared background service
//	    unless `--standalone`, ordered `permissions` rules, `agents`/`system`,
//	    `mcp.servers`, the subagent tool is `subagent` (was `task`), and a tool
//	    part is streamed once, only when it finished.

// opencodeDefaultMajor is assumed when the version cannot be read: the current
// release line. A 1.x binary given a 2.x config fails loudly on its first turn.
const opencodeDefaultMajor = 2

// opencodeVersionTimeout bounds one `--version` probe. The bundled desktop CLI
// is a large single-file binary whose cold start can take several seconds.
const opencodeVersionTimeout = 30 * time.Second

var opencodeVersionRe = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

type opencodeVersionKey struct {
	path  string
	size  int64
	mtime int64
}

// opencodeVersionCache maps a binary (path + size + mtime, so an in-place
// upgrade is re-probed) to its major version. Failures are not cached.
var opencodeVersionCache sync.Map

// parseOpenCodeMajor extracts the major version from `opencode --version`
// output ("opencode v2.2.5" on 2.x, a bare "1.19.0" on 1.x).
func parseOpenCodeMajor(out string) (int, bool) {
	m := opencodeVersionRe.FindStringSubmatch(out)
	if m == nil {
		return 0, false
	}
	major, err := strconv.Atoi(m[1])
	if err != nil || major <= 0 {
		return 0, false
	}
	return major, true
}

// detectOpenCodeMajor returns the major version of the opencode binary at
// binPath, probing `--version` once per binary build.
func detectOpenCodeMajor(ctx context.Context, binPath string, env []string) (int, error) {
	fi, err := os.Stat(binPath)
	if err != nil {
		return 0, err
	}
	key := opencodeVersionKey{path: binPath, size: fi.Size(), mtime: fi.ModTime().UnixNano()}
	if v, ok := opencodeVersionCache.Load(key); ok {
		return v.(int), nil
	}
	ctx, cancel := context.WithTimeout(ctx, opencodeVersionTimeout)
	defer cancel()
	cmd := proc.CommandContext(ctx, binPath, "--version")
	proc.TreeKill(cmd)
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("opencode --version: %w", err)
	}
	major, ok := parseOpenCodeMajor(string(out))
	if !ok {
		if len(out) > 200 {
			out = out[:200]
		}
		return 0, fmt.Errorf("opencode --version: unrecognised output %q", out)
	}
	opencodeVersionCache.Store(key, major)
	return major, nil
}

// apiMajor resolves the opencode API generation this provider drives: the
// probed binary version, else opencodeDefaultMajor.
func (c *OpenCodeCLI) apiMajor(ctx context.Context) int {
	if major, err := detectOpenCodeMajor(ctx, c.binPath, c.env("")); err == nil {
		return major
	}
	return opencodeDefaultMajor
}
