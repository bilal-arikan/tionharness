import { beforeEach, describe, expect, it, vi } from 'vitest'

// The rules only *reference* the api modules inside their `act` callbacks, but
// importing recommendations.ts pulls them in, so stub both to keep this a pure
// unit test with no network surface.
const createMCPServer = vi.fn()
const updateWorkspaceSettings = vi.fn()
const createHook = vi.fn()

vi.mock('@/api', () => ({
  api: {
    createMCPServer: (...a: unknown[]) => createMCPServer(...a),
    updateWorkspaceSettings: (...a: unknown[]) => updateWorkspaceSettings(...a),
    createHook: (...a: unknown[]) => createHook(...a),
  },
}))
vi.mock('@/api/system', () => ({ systemApi: {} }))

const { CBM_TOOL, NOOP_NAV, RULES, runRules } = await import('./recommendations')
type RecContext = Parameters<typeof runRules>[0]

// A workspace with nothing to complain about: agents exist, a working dir is
// set, an MCP server is wired, rtk is installed AND hooked, backup is on, and no
// bare-CLI tools are present. Every test starts here and breaks exactly one
// thing, so a fired card is unambiguously attributable.
function healthyCtx(over: Partial<RecContext> = {}): RecContext {
  return {
    tools: [{ name: 'rtk', found: true, path: 'C:/rtk.exe', wire: 'hook' }],
    servers: [{ command: 'some-other-mcp' }],
    hooks: [{ enabled: true, command: 'rtk hook claude' }],
    ws: { defaultWorkingDir: 'C:/work', codebaseMemoryEnabled: true, shellOutputCompression: 'on' },
    settings: { backupEnabled: true },
    agentsCount: 3,
    nav: NOOP_NAV,
    ...over,
  } as unknown as RecContext
}

const keysOf = (ctx: RecContext) => runRules(ctx).map((r) => r.key)

beforeEach(() => {
  createMCPServer.mockReset()
  updateWorkspaceSettings.mockReset()
  createHook.mockReset()
})

describe('runRules — baseline', () => {
  it('returns no cards for a fully-configured workspace', () => {
    expect(keysOf(healthyCtx())).toEqual([])
  })

  it('gives every rule a unique key so the ignore-list cannot collide', () => {
    const keys = RULES.map((r) => r.meta.key)
    expect(new Set(keys).size).toBe(keys.length)
  })

  it('gives every rule a title and summary so it can be listed without a probe', () => {
    for (const r of RULES) {
      expect(r.meta.title.trim(), `title for ${r.meta.key}`).not.toBe('')
      expect(r.meta.summary.trim(), `summary for ${r.meta.key}`).not.toBe('')
    }
  })
})

describe('token-optimizer rules', () => {
  it('warns when rtk and sqz both rewrite the command', () => {
    const recs = runRules(
      healthyCtx({
        hooks: [
          { enabled: true, command: 'rtk hook claude' },
          { enabled: true, command: 'sqz hook claude' },
        ],
      } as unknown as Partial<RecContext>),
    )
    const conflict = recs.find((r) => r.key === 'token-conflict')
    expect(conflict).toBeDefined()
    expect(conflict?.variant).toBe('warning')
  })

  // A disabled hook does not rewrite anything, so it must not count toward the
  // conflict — otherwise turning one off would leave the warning stuck on.
  it('does not warn when one of the two hooks is disabled', () => {
    const keys = keysOf(
      healthyCtx({
        hooks: [
          { enabled: true, command: 'rtk hook claude' },
          { enabled: false, command: 'sqz hook claude' },
        ],
      } as unknown as Partial<RecContext>),
    )
    expect(keys).not.toContain('token-conflict')
  })

  it('offers to wire an installed-but-unhooked optimizer', () => {
    const keys = keysOf(healthyCtx({ hooks: [] } as unknown as Partial<RecContext>))
    expect(keys).toContain('token')
  })

  it('stays quiet when the optimizer is not installed at all', () => {
    const keys = keysOf(healthyCtx({ tools: [], hooks: [] } as unknown as Partial<RecContext>))
    expect(keys).not.toContain('token')
  })

  it('wires rtk (not sqz) when rtk is the installed one', async () => {
    const recs = runRules(healthyCtx({ hooks: [] } as unknown as Partial<RecContext>))
    await recs.find((r) => r.key === 'token')?.act()
    expect(createHook).toHaveBeenCalledTimes(1)
    expect(createHook.mock.calls[0][0]).toMatchObject({ command: expect.stringContaining('rtk') })
  })
})

describe('shell-output compression rule', () => {
  const sqzInstalled = { name: 'sqz', found: true, path: 'C:/sqz.exe', wire: 'hook' }

  it('offers to turn on in-process compression when sqz is installed but unhooked and unset', () => {
    const keys = keysOf(
      healthyCtx({
        tools: [sqzInstalled],
        hooks: [],
        ws: { defaultWorkingDir: 'C:/work', codebaseMemoryEnabled: true, shellOutputCompression: 'auto' },
      } as unknown as Partial<RecContext>),
    )
    expect(keys).toContain('shell-compress')
  })

  // 'off' is an explicit user choice and 'on' already forces it — nagging in
  // either case would make the card un-dismissible by doing the obvious thing.
  it.each(['on', 'off'])('respects an explicit shellOutputCompression=%s', (mode) => {
    const keys = keysOf(
      healthyCtx({
        tools: [sqzInstalled],
        hooks: [],
        ws: { defaultWorkingDir: 'C:/work', codebaseMemoryEnabled: true, shellOutputCompression: mode },
      } as unknown as Partial<RecContext>),
    )
    expect(keys).not.toContain('shell-compress')
  })

  it('stays quiet when a sqz hook is already live', () => {
    const keys = keysOf(
      healthyCtx({
        tools: [sqzInstalled],
        hooks: [{ enabled: true, command: 'sqz hook claude' }],
        ws: { defaultWorkingDir: 'C:/work', codebaseMemoryEnabled: true, shellOutputCompression: 'auto' },
      } as unknown as Partial<RecContext>),
    )
    expect(keys).not.toContain('shell-compress')
  })
})

describe('codebase-memory rules', () => {
  const cbmInstalled = { name: CBM_TOOL, found: true, path: `C:/${CBM_TOOL}.exe`, wire: 'mcp' }

  it('offers to add the MCP server when the tool is on PATH but unwired', () => {
    const keys = keysOf(
      healthyCtx({ tools: [cbmInstalled], servers: [] } as unknown as Partial<RecContext>),
    )
    expect(keys).toContain('cbm-add')
    // The generic "no MCP source" nudge must yield to the specific one rather
    // than stacking a second card about the same empty server list.
    expect(keys).not.toContain('no-mcp')
  })

  it('matches an already-added server case-insensitively', () => {
    const keys = keysOf(
      healthyCtx({
        tools: [cbmInstalled],
        servers: [{ command: `C:/Progs/Codebase-Memory-MCP/${CBM_TOOL.toUpperCase()}.exe` }],
      } as unknown as Partial<RecContext>),
    )
    expect(keys).not.toContain('cbm-add')
  })

  it('offers to enable the workspace toggle once the server is added', () => {
    const keys = keysOf(
      healthyCtx({
        servers: [{ command: `C:/${CBM_TOOL}.exe` }],
        ws: { defaultWorkingDir: 'C:/work', codebaseMemoryEnabled: false, shellOutputCompression: 'on' },
      } as unknown as Partial<RecContext>),
    )
    expect(keys).toContain('cbm-enable')
    expect(keys).not.toContain('cbm-add')
  })

  it('adds the server with the probed path, falling back to the bare name', async () => {
    const recs = runRules(
      healthyCtx({
        tools: [{ name: CBM_TOOL, found: true, wire: 'mcp' }],
        servers: [],
      } as unknown as Partial<RecContext>),
    )
    await recs.find((r) => r.key === 'cbm-add')?.act()
    expect(createMCPServer).toHaveBeenCalledWith(
      expect.objectContaining({ name: CBM_TOOL, transport: 'stdio', command: CBM_TOOL }),
    )
  })
})

describe('workspace-setup rules', () => {
  it('flags an empty workspace', () => {
    expect(keysOf(healthyCtx({ agentsCount: 0 }))).toContain('no-agents')
  })

  it.each(['', '   ', undefined])('flags a blank default working dir (%p)', (dir) => {
    const keys = keysOf(
      healthyCtx({
        ws: { defaultWorkingDir: dir, codebaseMemoryEnabled: true, shellOutputCompression: 'on' },
      } as unknown as Partial<RecContext>),
    )
    expect(keys).toContain('workdir')
  })

  it('flags an empty MCP server list', () => {
    expect(keysOf(healthyCtx({ servers: [] } as unknown as Partial<RecContext>))).toContain('no-mcp')
  })

  it('flags disabled backups', () => {
    const keys = keysOf(healthyCtx({ settings: { backupEnabled: false } } as unknown as Partial<RecContext>))
    expect(keys).toContain('backup-off')
  })

  it('lists installed bare-CLI tools by name', () => {
    const recs = runRules(
      healthyCtx({
        tools: [
          { name: 'rtk', found: true, path: 'C:/rtk.exe', wire: 'hook' },
          { name: 'mmdc', found: true, wire: 'cli' },
          { name: 'crabbox', found: true, wire: 'cli' },
          { name: 'absent-cli', found: false, wire: 'cli' },
        ],
      } as unknown as Partial<RecContext>),
    )
    const cli = recs.find((r) => r.key === 'cli-tools')
    expect(cli?.desc).toContain('mmdc')
    expect(cli?.desc).toContain('crabbox')
    expect(cli?.desc).not.toContain('absent-cli')
  })
})

describe('card ordering', () => {
  // Cards stack top-to-bottom, so the conflict warning has to outrank the
  // routine setup nudges it would otherwise be buried under.
  it('puts the conflict warning above the setup suggestions', () => {
    const keys = keysOf(
      healthyCtx({
        agentsCount: 0,
        settings: { backupEnabled: false },
        hooks: [
          { enabled: true, command: 'rtk hook claude' },
          { enabled: true, command: 'sqz hook claude' },
        ],
      } as unknown as Partial<RecContext>),
    )
    expect(keys[0]).toBe('token-conflict')
    expect(keys.indexOf('token-conflict')).toBeLessThan(keys.indexOf('backup-off'))
  })
})
