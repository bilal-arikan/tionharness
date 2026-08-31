package api

import "github.com/bilal-arikan/tionharness/internal/db"

// explicitThinkingLevel resolves the reasoning tier a definition-driven agent
// creation should carry when the definition itself names none.
//
// The empty string is no longer a valid stored ThinkingLevel (see
// db.Agent.ThinkingLevel). The HTTP create/update path rejects it outright, but
// the definition-driven paths — market pack install, workspace-template seeding,
// template/workspace export — read the level from a payload that may predate the
// field. Those callers resolve it HERE, so which level a pack or template lands
// on is readable at the call site instead of being decided inside
// db.CreateAgent's safety net (which stays, as the last line of defence).
//
// The rule is db.LegacyThinkingLevelFor's, reused rather than re-implemented, so
// a levelless definition installs with exactly the behaviour it had before the
// field existed.
func explicitThinkingLevel(level, providerKind string) string {
	if level != "" {
		return level
	}
	return db.LegacyThinkingLevelFor(providerKind)
}
