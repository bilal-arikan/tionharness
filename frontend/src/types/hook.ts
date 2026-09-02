// Hooks (Phase P4 + lifecycle parity) — mirrors db.Hook. A hook runs an external
// command around a native tool call (PreToolUse/PostToolUse) OR at a turn/session
// lifecycle point (the rest). Tool hooks may rewrite input/output, auto-approve
// or block; lifecycle hooks inject context or observe, and some may block the
// turn (UserPromptSubmit). The command speaks the Claude Code hook contract
// (JSON on stdin, JSON on stdout, exit 2 = block).

export type HookEvent =
  | 'PreToolUse'
  | 'PostToolUse'
  | 'UserPromptSubmit'
  | 'SessionStart'
  | 'Stop'
  | 'SubagentStop'
  | 'PreCompact'
  | 'Notification'
  | 'SessionEnd'

export interface Hook {
  id: string
  event: HookEvent
  matcher: string // tool-name glob, "" = all tools
  type: string // "command"
  command: string
  timeoutSec: number
  enabled: boolean
  // Archived: hidden from the default list and never run, restorable.
  archived?: boolean
  // Pinned: exempt from the curator's automatic passes (Rota F3).
  pinned?: boolean
  // Usage telemetry: how many times the command ran and when it last did.
  fireCount?: number
  lastFiredAt?: number
  createdBy?: string
  createdAt: number
}

// BuiltinHook is one of TionHarness's automatic, non-editable tool behaviours
// (freshness guard, CLI native-tool bridging, hook passthrough, ...) surfaced
// read-only in the Hooks screen. Mirrors api.builtinHook.
export interface BuiltinHook {
  name: string
  scope: 'native' | 'cli' | 'both'
  event: string // "PreToolUse" | "PostToolUse" | "System" | combinations
  description: string
  enabled: boolean
  setting?: string // settings.json key that toggles it (absent = always on)
}
