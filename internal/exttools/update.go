package exttools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/proc"
)

// updateTimeout caps one update run. Package-manager installs pull from the
// network and can legitimately take a while (npm rebuilding mermaid-cli's
// puppeteer, winget downloading an ffmpeg build), so this is generous — but
// bounded, and the whole process tree is reaped when it expires.
const updateTimeout = 5 * time.Minute

// ErrManualUpdate is returned when an update is requested for a tool whose spec
// is UpdateManual. The UI already hides the button for those; this is the second
// gate, so a direct API call cannot make TionSwarm overwrite a binary.
var ErrManualUpdate = fmt.Errorf("bu araç otomatik güncellenmez")

// RunUpdate executes a tool's update command and returns its combined output.
//
// Only UpdateCommand specs run: the command comes from this package's own
// catalog, never from the request, so there is no path for a caller to inject an
// arbitrary command. The child gets the hardened non-interactive environment, so
// a package manager that would otherwise stop to ask a question fails fast
// instead of hanging forever with no one to answer it.
func RunUpdate(ctx context.Context, t Tool) (string, error) {
	if t.Update.Kind != UpdateCommand || t.Update.Command == "" {
		return "", ErrManualUpdate
	}
	if _, err := lookPath(t.Update.Command); err != nil {
		return "", fmt.Errorf("%q bu cihazda bulunamadı — %s güncellemesi bu paket yöneticisini gerektiriyor", t.Update.Command, t.Name)
	}

	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	cmd := proc.CommandContext(ctx, t.Update.Command, t.Update.Args...)
	cmd.Env = proc.HardenedEnv(nil)
	proc.TreeKill(cmd)

	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if ctx.Err() != nil {
		return text, fmt.Errorf("güncelleme zaman aşımına uğradı (%s)", updateTimeout)
	}
	if err != nil {
		return text, fmt.Errorf("güncelleme komutu başarısız: %w", err)
	}
	return text, nil
}

// UpdateCommandLine renders a spec as the shell line a user can copy and run
// themselves. Empty for manual specs, which carry a Note instead.
func (s UpdateSpec) UpdateCommandLine() string {
	if s.Kind != UpdateCommand || s.Command == "" {
		return ""
	}
	return strings.TrimSpace(s.Command + " " + strings.Join(s.Args, " "))
}
