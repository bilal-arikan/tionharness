package providers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bilal-arikan/tionharness/internal/proc"
	"github.com/bilal-arikan/tionharness/internal/textutil"
)

var cliPreflightSuccess sync.Map

func runCLIPreflight(ctx context.Context, kind, binPath, configDir, providerConfig string, env []string, configFiles ...string) error {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s", kind, binPath, configDir, providerConfig)
	for _, name := range configFiles {
		if configDir == "" {
			continue
		}
		path := filepath.Join(configDir, name)
		data, err := os.ReadFile(path)
		if err == nil {
			fmt.Fprintf(h, "\x00%s\x00", name)
			_, _ = h.Write(data)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("%s CLI preflight could not read config %q: %w", kind, path, err)
		}
	}
	key := fmt.Sprintf("%x", h.Sum(nil))
	if _, ok := cliPreflightSuccess.Load(key); ok {
		return nil
	}

	cmd := proc.CommandContext(ctx, binPath, "--version")
	// The CLI shells out even for --version (node launcher, update check); reap the
	// tree on cancellation so no survivor holds the output pipe.
	proc.TreeKill(cmd)
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if output, err := cmd.Output(); err != nil {
		detail := truncateCLIDiagnostic(stderr.String(), string(output))
		return fmt.Errorf("%s CLI preflight failed: %v%s", kind, err, detail)
	}
	cliPreflightSuccess.Store(key, struct{}{})
	return nil
}

func truncateCLIDiagnostic(stderr, stdout string) string {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		detail = strings.TrimSpace(stdout)
	}
	if detail == "" {
		return ""
	}
	const max = 4096
	if len(detail) > max {
		detail = "…" + textutil.TailBytes(detail, max)
	}
	return ": " + detail
}
