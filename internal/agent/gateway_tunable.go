package agent

// gateway_tunable.go — accessors for the Doc 52 gateway "dynamic extended surface".
//
// When enabled, the Interaction MCP extended tier starts EMPTY and grows only when
// the model calls activate_tools — the backend registers the tool and pushes
// tools/list_changed so the CLI re-lists and can call it the same turn (the gateway
// pattern; spike-verified). When disabled (the default), the extended tier advertises
// its full set up front and the CLI defers via its own ToolSearch (historical
// behaviour). Kept self-contained + trivially revertible, mirroring clibridge_tunable.go.

// SetGatewayDynamicExtended toggles the dynamic extended surface (default false).
func (t *Tunables) SetGatewayDynamicExtended(enabled bool) {
	t.mu.Lock()
	t.gatewayDynamicExtended = enabled
	t.mu.Unlock()
}

// GatewayDynamicExtended reports whether the extended tier grows on demand via
// activate_tools + tools/list_changed (gateway pattern) rather than advertising its
// full set up front. Default false until validated live.
func (t *Tunables) GatewayDynamicExtended() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.gatewayDynamicExtended
}
