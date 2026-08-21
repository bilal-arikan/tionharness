package providers

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// This file renders the `config.toml` codex-cli reads from its CODEX_HOME. The
// surface TionSwarm needs is small and fully known (see the codex provider
// contract), so it is hand-rendered rather than pulled in through a TOML
// dependency: correct escaping of prompt text and Windows paths matters far
// more here than generality.

// Timeouts applied to every MCP server block. Codex defaults are far too tight
// for a bridge that starts an in-process HTTP endpoint and can run long tools.
// A long sync run_subagent call (a delegated agent that edits files, runs the
// test suite and reports back) routinely outlives 10 minutes, so the tool
// timeout matches claude-cli's 30 minutes. The caller's ctx still bounds the
// call, so a high ceiling only decides when a LIVE call is killed for slowness.
const (
	codexMCPStartupTimeoutSec = 30
	codexMCPToolTimeoutSec    = 1800
)

// codexConfig is one turn's rendered codex configuration.
type codexConfig struct {
	// DeveloperInstructions becomes `developer_instructions`, an additive
	// developer-role message. It is the codex analogue of claude-cli's
	// --append-system-prompt and is rendered as a multi-line basic string.
	DeveloperInstructions string
	// ReasoningEffort becomes `model_reasoning_effort` when non-empty. Codex
	// accepts none|minimal|low|medium|high|xhigh|max|ultra.
	ReasoningEffort string
	// DisableWebSearch emits `web_search = false` under [tools].
	DisableWebSearch bool
	// DisableUpdatePlan emits `update_plan = { enabled = false }` under [tools].
	DisableUpdatePlan bool
	// DisableRequestUserInput emits `experimental_request_user_input =
	// { enabled = false }` under [tools].
	DisableRequestUserInput bool
	// Servers renders one [mcp_servers.<key>] block each, in sorted key order.
	Servers map[string]CLIMCPServer
}

// renderCodexConfig renders cfg as the text of a codex `config.toml`.
//
// Output is deterministic: map-backed values (servers, env, headers) are
// emitted in sorted key order so identical input always produces byte-identical
// output. Launch fingerprints — which decide whether a persistent CLI process
// can be reused across turns — hash this text, so ordering is load-bearing, not
// cosmetic.
func renderCodexConfig(cfg codexConfig) string {
	var b strings.Builder

	if cfg.DeveloperInstructions != "" {
		b.WriteString("developer_instructions = ")
		b.WriteString(tomlMultilineString(cfg.DeveloperInstructions))
		b.WriteString("\n")
	}
	if cfg.ReasoningEffort != "" {
		fmt.Fprintf(&b, "model_reasoning_effort = %s\n", tomlString(cfg.ReasoningEffort))
	}

	if cfg.DisableWebSearch || cfg.DisableUpdatePlan || cfg.DisableRequestUserInput {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("[tools]\n")
		if cfg.DisableWebSearch {
			// web_search is the one toggle that accepts a bare bool.
			b.WriteString("web_search = false\n")
		}
		if cfg.DisableUpdatePlan {
			// MUST stay an inline table. A bare bool is rejected by codex with
			// "invalid type: boolean `false`, expected struct UpdatePlanToolConfig".
			b.WriteString("update_plan = { enabled = false }\n")
		}
		if cfg.DisableRequestUserInput {
			b.WriteString("experimental_request_user_input = { enabled = false }\n")
		}
	}

	for _, key := range sortedKeys(cfg.Servers) {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(renderCodexServer(key, cfg.Servers[key]))
	}

	return b.String()
}

// renderCodexServer renders a single [mcp_servers.<key>] block.
//
// CLIMCPServer.AlwaysLoad is deliberately ignored: it exempts a server from
// claude-cli's tool-search deferral, and codex-cli has no equivalent mechanism
// (it always loads every configured server's tools).
func renderCodexServer(key string, s CLIMCPServer) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[mcp_servers.%s]\n", tomlKey(key))

	remote := s.Transport == "sse" || s.Transport == "http" || s.URL != ""
	if remote {
		fmt.Fprintf(&b, "url = %s\n", tomlString(s.URL))
		if len(s.Headers) > 0 {
			fmt.Fprintf(&b, "http_headers = %s\n", tomlInlineTable(s.Headers))
		}
	} else {
		fmt.Fprintf(&b, "command = %s\n", tomlString(s.Command))
		b.WriteString("args = ")
		b.WriteString(tomlStringArray(s.Args))
		b.WriteString("\n")
		if len(s.Env) > 0 {
			fmt.Fprintf(&b, "env = %s\n", tomlInlineTable(s.Env))
		}
	}

	fmt.Fprintf(&b, "startup_timeout_sec = %d\n", codexMCPStartupTimeoutSec)
	fmt.Fprintf(&b, "tool_timeout_sec = %d\n", codexMCPToolTimeoutSec)
	// MANDATORY on every server block. Without it EVERY MCP tool call fails with
	// "user cancelled MCP tool call" and the whole TionSwarm bridge is dead.
	// "auto" is NOT sufficient — codex only auto-approves unconditionally for
	// AppToolApproval::Approve. Do not remove this as a "default".
	b.WriteString("default_tools_approval_mode = \"approve\"\n")
	// MANDATORY. Without it the server defaults to optional, and codex only
	// waits OPTIONAL_MCP_STARTUP_GRACE (1s) for an optional server's handshake +
	// tools/list before silently dropping its tools for the whole turn — no
	// error, no retry, just an empty tool set (codex-rs
	// codex-mcp/src/connection_manager/tool_catalog.rs:35, 171-224).
	b.WriteString("required = true\n")

	return b.String()
}

// writeCodexConfig renders cfg and writes it as `config.toml` inside dir, which
// is the caller's CODEX_HOME. It returns the written path and a cleanup func.
//
// The cleanup is a no-op: config.toml is the CODEX_HOME's real config, not a
// temp file, so removing it would delete state the next turn expects. The func
// exists so callers can defer it uniformly alongside temp-file writers.
func writeCodexConfig(dir string, cfg codexConfig) (string, func(), error) {
	if dir == "" {
		return "", nil, fmt.Errorf("codex config: empty config dir")
	}
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(renderCodexConfig(cfg)), 0o644); err != nil {
		return "", nil, fmt.Errorf("codex config: write %s: %w", path, err)
	}
	return path, func() {}, nil
}

// tomlString renders s as a TOML basic string, escaping the characters the spec
// requires. Backslashes matter most here: a Windows path such as
// C:\Users\x\y.js is otherwise read as escape sequences and either corrupts the
// value or fails to parse.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	b.WriteString(tomlEscapeBasic(s))
	b.WriteByte('"')
	return b.String()
}

// tomlMultilineString renders s as a TOML multi-line basic string. Newlines and
// quotes stay literal, which keeps a rendered prompt readable in the file, but
// any embedded `"""` would terminate the string early — so the last quote of
// every such run is escaped. Backslashes are escaped for the same reason they
// are in tomlString: the value must survive a round-trip verbatim.
func tomlMultilineString(s string) string {
	esc := strings.ReplaceAll(s, `\`, `\\`)
	// Escaping the final quote of a run of three or more is enough to stop the
	// delimiter from matching, and the parser reads \" back as a plain quote.
	esc = strings.ReplaceAll(esc, `"""`, `""\"`)
	// A value ending in a quote would otherwise abut the closing delimiter and
	// form a fourth quote.
	if strings.HasSuffix(esc, `"`) {
		esc = esc[:len(esc)-1] + `\"`
	}
	// A leading newline immediately after the opening delimiter is trimmed by
	// TOML, so start the value on the same line to preserve it verbatim.
	return `"""` + esc + `"""`
}

// tomlEscapeBasic escapes the control and delimiter characters a TOML basic
// string cannot carry literally.
func tomlEscapeBasic(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// tomlKey renders a table-key segment. Keys that are not bare-key safe
// ([A-Za-z0-9_-]) are quoted, so a server name carrying a dot or a space still
// produces a single valid table path.
func tomlKey(k string) string {
	if k == "" {
		return `""`
	}
	for _, r := range k {
		bare := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-'
		if !bare {
			return tomlString(k)
		}
	}
	return k
}

// tomlStringArray renders a string slice as a TOML array. A nil or empty slice
// renders as [], which codex reads as "no arguments".
func tomlStringArray(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, tomlString(it))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// tomlInlineTable renders a string map as an inline table with sorted keys, so
// output stays deterministic across runs.
func tomlInlineTable(m map[string]string) string {
	keys := sortedKeys(m)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, tomlKey(k)+" = "+tomlString(m[k]))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

// sortedKeys returns m's keys in ascending order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
