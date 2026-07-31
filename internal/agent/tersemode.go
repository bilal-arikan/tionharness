package agent

import "strings"

// Terse ("caveman") mode: a workspace-level reply-style prompt appended to every
// agent's STATIC system prefix when the workspace toggle is on.
//
// It is a registry prompt (key "terse", workspace override
// <workspace>/config/prompts/terse.md → embedded default) rather than a skill,
// because a reply-style rule only works when it is ALWAYS in force. As a skill it
// would sit in the Available Skills catalog as a one-line summary and reach the
// model only if the model itself decided to call use_skill — which in practice
// means it rarely fires. As a prompt it is unconditional, editable per workspace,
// and (being in the static prefix) paid for once per prompt-cache window.
//
// The default text (prompts/defaults/terse.md) adapts juliusbrussee/caveman (MIT)
// for its compression rules, the no-self-reference rule and the auto-clarity
// carve-outs, plus two rules that source lacks because it is a CHAT skill rather
// than an agent prompt: the style governs the reply only (never authored files),
// and it shrinks output but never the WORK (no skipping reads/tests to be brief).

// TersePromptKey is the registry key holding the terse reply-style instructions.
const TersePromptKey = "terse"

// TerseModeBlock returns the workspace's terse-mode prompt when the toggle is on,
// or "" when it is off (nothing is sent — no placeholder, no empty header).
//
// Exported and hung off Runtime rather than taking (wsDir, enabled) arguments so
// every prompt-assembly site — headless (systemPrompt) and api-side (chat turn,
// context preview, session info) — reads ONE source for both the toggle and the
// text. A blank or placeholder-breaking override file falls back to the embedded
// default inside readPrompt, so a bad edit cannot silently disable the mode.
func (r *Runtime) TerseModeBlock() string {
	if !r.TerseModeEnabled() {
		return ""
	}
	return strings.TrimSpace(r.readPrompt(TersePromptKey))
}
