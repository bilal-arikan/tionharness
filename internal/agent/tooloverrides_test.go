package agent

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// A legacy agent (denylist populated, override map empty/absent) must read back
// as "blocked" overrides — that migration is what unifies the two models.
func TestParseToolOverridesMigratesLegacyDenylist(t *testing.T) {
	got := ParseToolOverrides(db.Agent{BlockedTools: `["Bash","mcp__linear__*"]`})
	if got["Bash"] != TierBlocked || got["mcp__linear__*"] != TierBlocked {
		t.Fatalf("legacy denylist should migrate to blocked overrides, got %v", got)
	}
	if len(got) != 2 {
		t.Fatalf("unexpected extra entries: %v", got)
	}
}

// An explicit override wins over the derived BlockedTools mirror, so an agent
// that deliberately UNBLOCKS a tool the stale mirror still lists is honoured.
func TestParseToolOverridesExplicitBeatsLegacyMirror(t *testing.T) {
	got := ParseToolOverrides(db.Agent{
		ToolOverrides: `{"Bash":"hidden"}`,
		BlockedTools:  `["Bash","Write"]`,
	})
	if got["Bash"] != tools.VisibilityHidden {
		t.Fatalf("explicit override must win, got %q", got["Bash"])
	}
	if got["Write"] != TierBlocked {
		t.Fatalf("unmirrored legacy entry should still block, got %q", got["Write"])
	}
}

// A corrupt override document must not make the agent unrunnable — it degrades
// to "no overrides" rather than failing the whole registry build. Unknown tiers
// are dropped for the same reason.
func TestParseToolOverridesIgnoresGarbage(t *testing.T) {
	got := ParseToolOverrides(db.Agent{ToolOverrides: `not json`, BlockedTools: `also not json`})
	if len(got) != 0 {
		t.Fatalf("malformed input should yield no overrides, got %v", got)
	}
	got = ParseToolOverrides(db.Agent{ToolOverrides: `{"Read":"bogus-tier","Write":"full"}`})
	if _, ok := got["Read"]; ok {
		t.Fatalf("unknown tier must be dropped, got %v", got)
	}
	if got["Write"] != tools.VisibilityFull {
		t.Fatalf("valid tier alongside a bad one must survive, got %v", got)
	}
}

func TestBlockedPatternsIsSortedBlockedSlice(t *testing.T) {
	got := blockedPatterns(map[string]string{
		"Write": TierBlocked,
		"Bash":  TierBlocked,
		"Read":  tools.VisibilityHidden,
	})
	if len(got) != 2 || got[0] != "Bash" || got[1] != "Write" {
		t.Fatalf("blockedPatterns = %v, want sorted [Bash Write]", got)
	}
}

func TestVisibilityOverridesExcludesBlocked(t *testing.T) {
	got := visibilityOverrides(map[string]string{"Bash": TierBlocked, "Read": tools.VisibilityFull})
	if _, ok := got["Bash"]; ok {
		t.Fatalf("blocked must not reach the registry: %v", got)
	}
	if got["Read"] != tools.VisibilityFull {
		t.Fatalf("visibility override lost: %v", got)
	}
}

// A "prefix*" override key is expanded across the catalog, since SetVisibility
// takes one exact name. The longer (more specific) pattern must win.
func TestApplyVisibilityOverridesExpandsPatterns(t *testing.T) {
	reg := tools.NewRegistry()
	names := []string{"mcp__linear__issue", "mcp__linear__team", "mcp__github__pr", "Read"}
	applyVisibilityOverrides(reg, map[string]string{
		"mcp__*":         tools.VisibilityHidden,
		"mcp__linear__":  tools.VisibilityFull, // exact key, matches nothing → no-op
		"mcp__linear__*": tools.VisibilityFull,
	}, names)

	if reg.VisibilityOf("mcp__github__pr") != tools.VisibilityHidden {
		t.Fatalf("broad pattern not applied: %q", reg.VisibilityOf("mcp__github__pr"))
	}
	for _, n := range []string{"mcp__linear__issue", "mcp__linear__team"} {
		if reg.VisibilityOf(n) != tools.VisibilityFull {
			t.Fatalf("specific pattern must win for %s: %q", n, reg.VisibilityOf(n))
		}
	}
	if reg.VisibilityOf("Read") != tools.VisibilityFull {
		t.Fatalf("unmatched tool should keep its default (eager), got %q", reg.VisibilityOf("Read"))
	}
}
