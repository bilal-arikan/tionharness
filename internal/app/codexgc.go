package app

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/settings"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// The first pass waits out boot (workspaces open, recovered turns resume); later
// passes are rare because homes only pile up as prompts, models and sessions change.
const (
	codexResumeGCFirstDelay = 5 * time.Minute
	codexResumeGCInterval   = 6 * time.Hour
)

var errDegradedWorkspace = errors.New("a workspace failed to open; its sessions cannot be read")

// codexResumeGC periodically removes codex resume homes no session can resume
// (providers.PruneCodexResumeHomes). Without it every prompt or model change
// and every deleted session leaves a ~70MB home behind for good.
type codexResumeGC struct {
	stop chan struct{}
	done chan struct{}
}

func startCodexResumeGC(dataDir string, manager *workspace.Manager, store *settings.ProviderStore, logger *slog.Logger) *codexResumeGC {
	g := &codexResumeGC{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(g.done)
		timer := time.NewTimer(codexResumeGCFirstDelay)
		defer timer.Stop()
		for {
			select {
			case <-g.stop:
				return
			case <-timer.C:
			}
			sweepCodexResumeHomes(codexHomes(dataDir, store.List()), manager, logger)
			timer.Reset(codexResumeGCInterval)
		}
	}()
	return g
}

// Stop ends the loop and waits for a running pass, so Shutdown closes the
// workspace stores only after the sweep stopped reading them.
func (g *codexResumeGC) Stop() {
	close(g.stop)
	<-g.done
}

func sweepCodexResumeHomes(homes []string, manager *workspace.Manager, logger *slog.Logger) {
	live, err := liveCLIThreads(manager)
	if err != nil {
		logger.Info("codex resume home sweep skipped", "reason", err)
		return
	}
	now := time.Now()
	for _, home := range homes {
		res, err := providers.PruneCodexResumeHomes(home, live, providers.CodexResumeHomeGrace, now)
		if err != nil {
			logger.Warn("codex resume home sweep failed", "home", home, "err", err)
		}
		if res.Removed > 0 || res.FreedBytes > 0 {
			logger.Info("codex resume homes pruned", "home", home,
				"scanned", res.Scanned, "removed", res.Removed, "freed_mb", res.FreedBytes>>20)
		}
	}
}

// codexHomes lists every CODEX_HOME a turn can run in: the app-global one and
// each codex-cli instance that names its own config dir.
func codexHomes(dataDir string, instances []settings.ProviderInstance) []string {
	seen := map[string]bool{}
	var out []string
	add := func(dir string) {
		if dir = strings.TrimSpace(dir); dir == "" {
			return
		}
		dir = filepath.Clean(dir)
		if !seen[dir] {
			seen[dir] = true
			out = append(out, dir)
		}
	}
	if home, err := agent.ResolveCLIHomeDir(dataDir, "codex-cli", ""); err == nil {
		add(home)
	}
	for _, inst := range instances {
		if inst.KindID == "codex-cli" {
			add(inst.Config[providers.FieldKeyConfigDir])
		}
	}
	return out
}

// liveCLIThreads collects the CLI thread ids sessions still hold, archived ones
// included. A degraded workspace aborts the pass: its sessions may point at
// homes, and an unreadable store must never read as "no references".
func liveCLIThreads(manager *workspace.Manager) (map[string]struct{}, error) {
	live := map[string]struct{}{}
	for _, entry := range manager.ListWithDegraded() {
		if entry.Degraded {
			return nil, errDegradedWorkspace
		}
		ws, err := manager.Get(entry.ID)
		if err != nil {
			return nil, err
		}
		sessions, err := ws.DB.ListSessions(context.Background(), "")
		if err != nil {
			return nil, err
		}
		for _, s := range sessions {
			if id := strings.TrimSpace(s.CLISessionID); id != "" {
				live[id] = struct{}{}
			}
		}
	}
	return live, nil
}
