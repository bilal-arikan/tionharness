package providers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

// Codex plugin installation.
//
// Enabling a plugin takes TWO independent things, both measured on codex
// 0.153.3 and neither sufficient alone:
//
//   - the config keys ([marketplaces.<name>] + [plugins."<sel>"]), rewritten by
//     renderCodexConfig on every turn, and
//   - the installed cache under <CODEX_HOME>/plugins/cache/<marketplace>/…,
//     which only `codex plugin add` creates.
//
// With the keys but no cache, `codex plugin list` reports "not installed" and the
// plugin's skills never reach the model. With the cache but no keys, the plugin
// goes dark the moment config.toml is rewritten. This file owns the second half.
//
// Installation is per conversation home. Codex keys its cache by CODEX_HOME, and
// TionHarness gives every scoped chat its own durable home, so each one installs
// once (~600 ms measured for one marketplace + one plugin) and every later turn
// in that conversation finds it already there. Disposable shadow homes — used by
// title/compaction/insight calls — are deliberately skipped: they are deleted
// after the turn, so installing into them would pay the cost every single time
// for auxiliary calls that have no use for plugin skills.

// codexPluginInstallTimeout bounds one `codex plugin …` invocation. Installing a
// local marketplace is a directory copy, but a git-sourced one fetches, so the
// ceiling is generous; the caller's ctx still bounds the whole turn.
const codexPluginInstallTimeout = 90 * time.Second

// codexPluginStamp records which configuration a home was provisioned for, so a
// home is only re-provisioned when the set actually changes.
const codexPluginStamp = ".tionharness-plugins"

// ensureCodexPlugins makes home's installed plugin set match c's configuration.
//
// It is best-effort by design: a marketplace that no longer resolves, or a
// plugin whose install fails, must not fail the turn — the turn simply runs
// without that plugin's skills, exactly as it did before the feature existed.
// Failures are returned as human-readable notes so the caller can surface them
// in the trace rather than swallowing them.
func (c *CodexCLI) ensureCodexPlugins(ctx context.Context, home string) []string {
	if home == "" || len(c.plugins) == 0 && len(c.marketplaces) == 0 {
		return nil
	}
	want := codexPluginFingerprint(c.marketplaces, c.plugins)
	stamp := filepath.Join(home, codexPluginStamp)
	if got, err := os.ReadFile(stamp); err == nil && string(got) == want {
		return nil
	}

	var notes []string
	for _, m := range sortedCodexMarketplaces(c.marketplaces) {
		if strings.TrimSpace(m.Name) == "" || strings.TrimSpace(m.Source) == "" {
			continue
		}
		args := []string{"plugin", "marketplace", "add", m.Source}
		if m.SourceType == "git" && strings.TrimSpace(m.Ref) != "" {
			args = append(args, "--ref", m.Ref)
		}
		if err := c.runCodexPluginCmd(ctx, home, args); err != nil {
			// Already-registered is the steady state on a home that was provisioned
			// before, not a problem worth reporting.
			if !codexPluginAlreadyPresent(err.Error()) {
				notes = append(notes, fmt.Sprintf("marketplace %q eklenemedi: %s", m.Name, codexPluginErrLine(err.Error())))
			}
		}
	}
	for _, selector := range sortedStrings(c.plugins) {
		if strings.TrimSpace(selector) == "" {
			continue
		}
		if err := c.runCodexPluginCmd(ctx, home, []string{"plugin", "add", selector}); err != nil {
			if !codexPluginAlreadyPresent(err.Error()) {
				notes = append(notes, fmt.Sprintf("plugin %q kurulamadı: %s", selector, codexPluginErrLine(err.Error())))
			}
		}
	}
	// Stamp even on partial failure: retrying a broken marketplace on every turn
	// would add its timeout to each one. Editing the lists changes the
	// fingerprint, which is the user-visible way to retry.
	_ = os.WriteFile(stamp, []byte(want), 0o600)
	return notes
}

// runCodexPluginCmd runs one `codex plugin …` invocation against home.
func (c *CodexCLI) runCodexPluginCmd(ctx context.Context, home string, args []string) error {
	ctx, cancel := context.WithTimeout(ctx, codexPluginInstallTimeout)
	defer cancel()

	cmd := proc.CommandContext(ctx, c.binPath, args...)
	cmd.Env = append(codexBaseEnv(), "CODEX_HOME="+home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// codexPluginFingerprint is a stable digest of the configured set, used to skip
// re-provisioning a home whose configuration has not changed.
func codexPluginFingerprint(ms []CodexMarketplace, plugins []string) string {
	parts := make([]string, 0, len(ms)+len(plugins))
	for _, m := range sortedCodexMarketplaces(ms) {
		parts = append(parts, "m:"+m.Name+"|"+m.SourceType+"|"+m.Source+"|"+m.Ref)
	}
	parts = append(parts, func() []string {
		p := sortedStrings(plugins)
		for i := range p {
			p[i] = "p:" + p[i]
		}
		return p
	}()...)
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

// codexPluginAlreadyPresent reports whether a failure just means the marketplace
// or plugin was already registered in this home.
func codexPluginAlreadyPresent(msg string) bool {
	msg = strings.ToLower(msg)
	return strings.Contains(msg, "already") && (strings.Contains(msg, "added") ||
		strings.Contains(msg, "installed") || strings.Contains(msg, "exists") ||
		strings.Contains(msg, "registered"))
}

// codexPluginErrLine reduces a codex CLI failure to its most useful single line,
// so a trace note stays readable instead of carrying a full usage dump.
func codexPluginErrLine(msg string) string {
	for _, line := range strings.Split(msg, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "error") || strings.Contains(line, "reserved") {
			return line
		}
	}
	if first, _, ok := strings.Cut(strings.TrimSpace(msg), "\n"); ok {
		return first
	}
	return strings.TrimSpace(msg)
}
