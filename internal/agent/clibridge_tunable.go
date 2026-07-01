package agent

// clibridge_tunable.go — POC accessors for the hidden-tier CLI-bridge exclusion.
//
// The field lives on Tunables (tunables.go); its getter/setter are kept here to
// keep the POC self-contained and trivially revertible. See
// tools.Registry.BridgeableDefsFiltered for the filter this gate drives.

// SetCLIBridgeSkipHidden toggles the POC: when enabled (the default), hidden-tier
// lazy built-ins (the self-management suite) are excluded from the claude-cli
// Interaction MCP bridge so their full schemas never reach the CLI process.
// Disable it to reproduce the historical behaviour (hidden tools bridged).
func (t *Tunables) SetCLIBridgeSkipHidden(enabled bool) {
	t.mu.Lock()
	t.cliBridgeSkipHidden = enabled
	t.mu.Unlock()
}

// CLIBridgeSkipHidden reports whether hidden-tier tools are withheld from the
// claude-cli bridge (POC gate; default false).
func (t *Tunables) CLIBridgeSkipHidden() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.cliBridgeSkipHidden
}
