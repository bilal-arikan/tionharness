// zvec-grep wiring shared by the External Tools callout and the workspace
// recommendation cards. Both must recognise an already-added server by the SAME
// rule the backend uses (internal/agent/capabilities_zvecgrep.go isZvecGrepServer),
// otherwise the UI offers to add a server the runtime already treats as wired.
import type { MCPServer } from '@/types'

/** Catalog key of the zvec-grep CLI: the executable name, like `claude`. */
export const ZVEC_GREP_TOOL = 'zg'

/**
 * Server name used by one-click wiring. It is the name `zg install` gives the
 * server in Claude Code and Codex, so tool names read the same everywhere.
 */
export const ZVEC_GREP_SERVER_NAME = 'zvec_grep'

/** Arguments that turn the CLI into an MCP stdio bridge to the shared daemon. */
export const ZVEC_GREP_SERVER_ARGS: readonly string[] = ['server', '--stdio']

const PACKAGE_MARKER = 'zvec-grep'
const SHIM_EXT = /\.(cmd|exe|ps1|bat)$/

type ServerShape = Partial<Pick<MCPServer, 'transport' | 'command' | 'args'>>

/**
 * Whether a stored MCP server launches zvec-grep: the `zg` shim, matched by base
 * name ("zg" also occurs inside unrelated names such as zgrep), or a launcher
 * such as node or npx whose command or arguments name the package. Only stdio
 * servers qualify.
 */
export function isZvecGrepServer(server: ServerShape): boolean {
  if (server.transport && server.transport !== 'stdio') return false
  const command = (server.command ?? '').trim()
  const base = (command.replace(/\\/g, '/').split('/').pop() ?? '').toLowerCase()
  if (base.replace(SHIM_EXT, '') === 'zg') return true
  if (command.toLowerCase().includes(PACKAGE_MARKER)) return true
  try {
    const args: unknown = JSON.parse(server.args || '[]')
    return (
      Array.isArray(args) &&
      args.some((a) => typeof a === 'string' && a.toLowerCase().includes(PACKAGE_MARKER))
    )
  } catch {
    return false
  }
}
