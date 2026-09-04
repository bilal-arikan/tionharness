package api

import "strings"

// customizationName is the default name of a role-bound child derived from a
// locked built-in ("Özelleştir" in the UI).
//
// The workspace name is part of it on purpose: the customisation lives in THIS
// workspace's agent store only, so every other workspace keeps resolving the
// system role from the built-in definition. Carrying the workspace in the name
// makes that scope visible in the roster, where a bare "(özel)" suggested an
// app-wide change.
//
// A workspace with no name is only possible for a record written before names
// were required; it falls back to the generic marker rather than producing a
// dangling "Titler ()".
func customizationName(parentName, workspaceName string) string {
	if ws := strings.TrimSpace(workspaceName); ws != "" {
		return parentName + " (" + ws + ")"
	}
	return parentName + " (özel)"
}
