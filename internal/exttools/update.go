package exttools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/proc"
	"github.com/bilal-arikan/tionharness/internal/procwatch"
)

// updateTimeout caps one update run. Package-manager installs pull from the
// network and can legitimately take a while (npm rebuilding mermaid-cli's
// puppeteer, winget downloading an ffmpeg build), so this is generous — but
// bounded, and the whole process tree is reaped when it expires.
const updateTimeout = 5 * time.Minute

// ErrManualUpdate is returned when an update is requested for a tool whose spec
// is UpdateManual. The UI already hides the button for those; this is the second
// gate, so a direct API call cannot make TionHarness overwrite a binary.
var ErrManualUpdate = fmt.Errorf("bu araç otomatik güncellenmez")

// ErrToolBusy is returned when the tool's binary is in use by a process
// TionHarness is running (see Tool.ProcLabelPrefix).
var ErrToolBusy = errors.New("araç şu an kullanımda")

// RunningProcesses counts the running ledger entries that execute this tool's
// binary. Zero for tools without a ProcLabelPrefix. The ledger only sees
// processes TionHarness itself spawned — a codex the user runs in their own
// terminal is invisible here.
func RunningProcesses(t Tool) int {
	if t.ProcLabelPrefix == "" {
		return 0
	}
	n := 0
	for _, e := range procwatch.Default().List(procwatch.Filter{
		Statuses: []procwatch.Status{procwatch.StatusRunning},
		Kinds:    []procwatch.Kind{procwatch.KindProvider},
	}) {
		if strings.HasPrefix(e.Label, t.ProcLabelPrefix) {
			n++
		}
	}
	return n
}

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
	if n := RunningProcesses(t); n > 0 {
		return "", fmt.Errorf("%w: %d çalışan %s süreci var — turlar bitince (veya Süreçler panelinden durdurunca) tekrar dene", ErrToolBusy, n, t.Name)
	}
	if _, err := lookPath(t.Update.Command); err != nil {
		return "", fmt.Errorf("%q bu cihazda bulunamadı — %s güncellemesi bu paket yöneticisini gerektiriyor", t.Update.Command, t.Name)
	}

	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	cmd := proc.CommandContext(ctx, t.Update.Command, t.Update.Args...)
	cmd.Env = proc.HardenedEnv(nil)
	proc.TreeKill(cmd)

	h := procwatch.Default().Begin(ctx, procwatch.Meta{
		Kind: procwatch.KindExternal, Label: t.Name + " update",
		Command: t.Update.Command + " " + strings.Join(t.Update.Args, " "),
	})
	// A failed installer can also have changed files before returning an error.
	defer InvalidateVersionCache()
	out, err := cmd.CombinedOutput()
	h.Started(cmd)
	h.AppendOutput(string(out))
	h.Finish(err)
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
	parts := []string{s.Command}
	for _, a := range s.Args {
		// An npm --prefix can contain spaces (C:\Users\Ad Soyad\…); quote it so
		// the copied line still works when pasted into a shell.
		if strings.ContainsAny(a, " \t") {
			a = `"` + a + `"`
		}
		parts = append(parts, a)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}
