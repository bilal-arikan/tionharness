package conversation

// CLI-wrapper (claude-cli / Claude Code) token overhead reference.
//
// When TionHarness delegates a turn to a CLI-wrapper provider (claude-cli), the CLI
// wraps TionHarness's appended payload inside its OWN harness before sending it to the
// model: a large built-in system prompt, its own built-in tool schemas, and the
// MCP bridge that re-serializes every exposed tool. That harness is invisible to
// TionHarness's segment estimator (EstimateTokens / the session-context TotalTokens)
// yet is fully billed as model input. The gap it creates is the "claude-cli tax":
// the measured per-call input runs ~3x TionHarness's own estimate on typical turns.
//
// These constants are EMPIRICALLY MEASURED (claude-cli 2.1.201, 2026-07-04) so any
// token-accounting code can PREDICT the tax before the first turn is even sent —
// not just report it after the fact from the debug journal. This is the single
// generic source the whole app should read for CLI-overhead projection.
//
// Measurement method (reproducible — read the real API `usage`):
//
//	claude -p "ok" --output-format stream-json --verbose \
//	  --strict-mcp-config --mcp-config '{"mcpServers":{}}' [--disallowedTools <all built-ins>]
//	totalInput = usage.input_tokens + cache_creation_input_tokens + cache_read_input_tokens
//
// Observed:
//   - pure system prompt, 0 tools (all built-ins disallowed):        17067
//   - + Claude Code built-in tool schemas (~15 tools):               26265  (built-ins ≈ 9198)
//   - avg bridged TionHarness tool JSON schema (name+desc+inputSchema):  ~215   (range 42–710)
//
// Numbers are approximate (±~15%): the CLI re-serializes MCP schemas more verbosely
// than TionHarness's ~4 chars/token heuristic, and the real tokenizer differs from the
// character-count estimate. Re-run the measurement when the claude-cli major version
// changes (its baseline system prompt grows over releases).
const (
	// CLIBaseSystemTokens is the pure Claude Code system prompt with ZERO tools
	// (all built-ins disallowed, no MCP). Measured: 17067.
	CLIBaseSystemTokens = 17000

	// CLIBuiltinToolsTokens is the added cost of Claude Code's own built-in tool
	// schemas (Read/Write/Edit/Bash/Glob/Grep/Task/WebFetch/…, ~15 tools).
	// Measured: 26265 - 17067 ≈ 9200.
	CLIBuiltinToolsTokens = 9200

	// CLIBaseTokens is the fixed floor a CLI-wrapper turn pays before ANY TionHarness
	// payload or bridged MCP tool: base system prompt + the CLI's own built-in tools.
	CLIBaseTokens = CLIBaseSystemTokens + CLIBuiltinToolsTokens // ~26200

	// CLIAvgBridgedToolTokens is the average JSON-schema cost of ONE TionHarness tool
	// exposed over the MCP bridge (name + description + inputSchema + the
	// mcp__<server>__<tool> namespacing). Measured across the live tool catalog:
	// mean ~215, range 42–710 (large external MCP tools can exceed 1600).
	CLIAvgBridgedToolTokens = 215
)

// PredictCLIOverhead estimates the CLI-wrapper harness token cost that TionHarness's
// own segment estimate (EstimateTokens / session-context TotalTokens) does NOT
// include, for a turn that eagerly exposes loadedTools bridged tools. This is the
// projected gap between "Tahmin" and "Gerçek" before the first turn is measured.
//
// Only EAGER (always-loaded) tools carry a full schema up front; deferred/lazy
// tools are name-only stubs until activated via ToolSearch, so pass the eager
// count here (over-counting slightly is safe — it is an upper bound).
func PredictCLIOverhead(loadedTools int) int {
	if loadedTools < 0 {
		loadedTools = 0
	}
	return CLIBaseTokens + loadedTools*CLIAvgBridgedToolTokens
}
