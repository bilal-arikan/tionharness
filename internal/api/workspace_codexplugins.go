package api

import (
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Validation for the codex plugin settings.
//
// These values are rendered verbatim into the codex config.toml TionHarness
// writes before every turn, and that turn runs with --strict-config: a name codex
// refuses, or a selector it cannot parse, does not degrade the plugin — it fails
// config loading and therefore the whole turn. So the edge rejects what codex
// would reject, with a message that says which entry is at fault.

// validateCodexMarketplaces checks every configured marketplace source.
func validateCodexMarketplaces(ms []db.CodexMarketplace) error {
	seen := make(map[string]bool, len(ms))
	for _, m := range ms {
		name := strings.TrimSpace(m.Name)
		if name == "" {
			return fmt.Errorf("marketplace adı boş olamaz")
		}
		if db.CodexMarketplaceNameReserved(name) {
			return fmt.Errorf("%q marketplace adı Codex tarafından ayrılmıştır ve eklenemez; kopyanıza farklı bir ad verin", name)
		}
		if !db.ValidCodexMarketplaceName(name) {
			return fmt.Errorf("%q geçersiz bir marketplace adı: yalnız harf, rakam, _ ve - kullanılabilir", name)
		}
		if seen[strings.ToLower(name)] {
			return fmt.Errorf("%q marketplace adı birden fazla kez tanımlanmış", name)
		}
		seen[strings.ToLower(name)] = true

		if strings.TrimSpace(m.Source) == "" {
			return fmt.Errorf("%q marketplace'inin kaynağı (source) boş olamaz", name)
		}
		switch m.SourceType {
		case db.CodexMarketplaceLocal, db.CodexMarketplaceGit:
		case "":
			return fmt.Errorf("%q marketplace'i için kaynak türü (local/git) belirtilmeli", name)
		default:
			return fmt.Errorf("%q marketplace'inin kaynak türü %q geçersiz: local veya git olmalı", name, m.SourceType)
		}
	}
	return nil
}

// validateCodexPluginSelectors checks the "<plugin>@<marketplace>" selectors.
func validateCodexPluginSelectors(selectors []string) error {
	seen := make(map[string]bool, len(selectors))
	for _, sel := range selectors {
		sel = strings.TrimSpace(sel)
		if sel == "" {
			return fmt.Errorf("plugin seçicisi boş olamaz")
		}
		if _, _, ok := db.SplitCodexPluginSelector(sel); !ok {
			return fmt.Errorf("%q geçersiz bir plugin seçicisi: \"<plugin>@<marketplace>\" biçiminde olmalı", sel)
		}
		if seen[sel] {
			return fmt.Errorf("%q plugin'i birden fazla kez tanımlanmış", sel)
		}
		seen[sel] = true
	}
	return nil
}
