package agent

// codemode_tunable.go — POC accessors for code-execution mode (_Docs/44).
//
// The field lives on Tunables (tunables.go); its getter/setter are kept here to
// keep the POC self-contained and trivially revertible, mirroring
// clibridge_tunable.go. See tools.RunCodeTool for the tool this gate registers.

// SetCodeMode toggles code-execution mode: when enabled (and the shell gate is
// on), the MCP catalog is exposed as generated Python bindings behind the
// run_code tool so MCP schemas stay out of the model's context window. Driven
// live from the Settings screen (enableCodeMode) via applySettings; the
// TIONSWARM_CODE_MODE env var seeds the setting once at boot.
func (t *Tunables) SetCodeMode(enabled bool) {
	t.mu.Lock()
	t.codeMode = enabled
	t.mu.Unlock()
}

// CodeModeEnabled reports whether code-execution mode is on (settings-driven;
// default false).
func (t *Tunables) CodeModeEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.codeMode
}
