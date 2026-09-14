package api

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestValidateCodexMarketplacesRejectsReserved(t *testing.T) {
	// The whole point of the import flow is that codex refuses its own names from
	// a user source; persisting one would fail every turn under --strict-config.
	err := validateCodexMarketplaces([]db.CodexMarketplace{
		{Name: "openai-bundled", Source: "/mp", SourceType: "local"},
	})
	if err == nil || !strings.Contains(err.Error(), "ayrılmış") {
		t.Fatalf("expected a reserved-name rejection, got %v", err)
	}
}

func TestValidateCodexMarketplacesRequiresSourceAndType(t *testing.T) {
	if err := validateCodexMarketplaces([]db.CodexMarketplace{
		{Name: "mp", Source: "", SourceType: "local"},
	}); err == nil {
		t.Error("an empty source must be rejected")
	}
	if err := validateCodexMarketplaces([]db.CodexMarketplace{
		{Name: "mp", Source: "/mp", SourceType: ""},
	}); err == nil {
		t.Error("a missing source type must be rejected")
	}
	if err := validateCodexMarketplaces([]db.CodexMarketplace{
		{Name: "mp", Source: "/mp", SourceType: "svn"},
	}); err == nil {
		t.Error("an unknown source type must be rejected")
	}
}

func TestValidateCodexMarketplacesRejectsDuplicateNames(t *testing.T) {
	err := validateCodexMarketplaces([]db.CodexMarketplace{
		{Name: "mp", Source: "/a", SourceType: "local"},
		{Name: "MP", Source: "/b", SourceType: "local"},
	})
	if err == nil {
		t.Fatal("two marketplaces cannot share a name")
	}
}

func TestValidateCodexMarketplacesAcceptsValid(t *testing.T) {
	if err := validateCodexMarketplaces([]db.CodexMarketplace{
		{Name: "local-openai", Source: `C:\mp`, SourceType: "local"},
		{Name: "team", Source: "org/repo", SourceType: "git", Ref: "main"},
	}); err != nil {
		t.Fatalf("valid marketplaces rejected: %v", err)
	}
}

func TestValidateCodexPluginSelectors(t *testing.T) {
	if err := validateCodexPluginSelectors([]string{"visualize@local-openai"}); err != nil {
		t.Fatalf("valid selector rejected: %v", err)
	}
	for _, bad := range []string{"", "visualize", "@mp", "visualize@"} {
		if err := validateCodexPluginSelectors([]string{bad}); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
	if err := validateCodexPluginSelectors([]string{"a@mp", "a@mp"}); err == nil {
		t.Error("a duplicate selector must be rejected")
	}
}

func TestSuggestedCodexMarketplaceName(t *testing.T) {
	if got := suggestedCodexMarketplaceName("openai-bundled", true); got == "openai-bundled" {
		t.Fatal("a reserved name must be renamed for the copy")
	}
	if !db.ValidCodexMarketplaceName(suggestedCodexMarketplaceName("openai-bundled", true)) {
		t.Fatal("the suggested name must itself be valid")
	}
	if got := suggestedCodexMarketplaceName("team-mp", false); got != "team-mp" {
		t.Fatalf("a usable name should pass through, got %q", got)
	}
}
