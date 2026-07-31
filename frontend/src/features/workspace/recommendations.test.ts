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
    // No release check performed / nothing behind. Also the shape an OFFLINE
    // probe produces, which must stay silent rather than claim anything.
    updates: [],
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
  // rtk + sqz together is the BEST configuration, not a conflict: they act at
  // opposite ends of the call and stack (git log -30: 6595 raw -> sqz 2027 ->
  // rtk 2157 -> rtk+sqz 1167 tokens). The old 'token-conflict' card told users to
  // disable one of them, so it was removed; this locks it stays removed.
  it('does not flag rtk + sqz as a conflict', () => {
    const keys = keysOf(
      healthyCtx({
        hooks: [
          { enabled: true, command: 'rtk hook claude' },
          { enabled: true, command: 'sqz hook claude' },
        ],
      } as unknown as Partial<RecContext>),
    )
    expect(keys).not.toContain('token-conflict')
  })

  const sqzTool = { name: 'sqz', found: true, path: 'C:/sqz.exe', wire: 'hook' }

  it('offers to wire sqz when it is installed but unhooked', () => {
    const keys = keysOf(
      healthyCtx({ tools: [sqzTool], hooks: [] } as unknown as Partial<RecContext>),
    )
    expect(keys).toContain('token')
  })

  it('stays quiet when the optimizer is not installed at all', () => {
    const keys = keysOf(healthyCtx({ tools: [], hooks: [] } as unknown as Partial<RecContext>))
    expect(keys).not.toContain('token')
  })

  // rtk is NOT offered as a hook any more. Its PowerShell one-liner template was
  // executed by claude-cli through bash, failed on the first `|`, and a failing
  // PreToolUse hook BLOCKS the tool — so this card used to brick Bash for the whole
  // workspace (2026-07-31, WS10/SES63). rtk is wired by the shellCommandRewrite
  // setting instead. This test locks the card away from rtk for good.
  it('never offers an rtk HOOK, even when rtk is the only optimizer installed', async () => {
    const ctx = healthyCtx({ hooks: [] } as unknown as Partial<RecContext>) // tools: rtk only
    const recs = runRules(ctx)
    const token = recs.find((r) => r.key === 'token')
    expect(token).toBeUndefined()
    expect(createHook).not.toHaveBeenCalled()
  })

  it('wires sqz — never rtk — when the card does fire', async () => {
    const recs = runRules(
      healthyCtx({ tools: [sqzTool], hooks: [] } as unknown as Partial<RecContext>),
    )
    await recs.find((r) => r.key === 'token')?.act()
    expect(createHook).toHaveBeenCalledTimes(1)
    const cmd = createHook.mock.calls[0][0].command as string
    expect(cmd).toContain('sqz')
    // The removed template's signature — if it ever comes back, fail here.
    expect(cmd).not.toContain('ReadToEnd')
  })
})

describe('shell-output compression rule', () => {
  const sqzInstalled = { name: 'sqz', found: true, path: 'C:/sqz.exe', wire: 'hook' }

  it('offers to turn on in-process compression when sqz is installed but unhooked and unset', () => {
    const keys = keysOf(
      healthyCtx({
        tools: [sqzInstalled],
        hooks: [],
        ws: {
          defaultWorkingDir: 'C:/work',
          codebaseMemoryEnabled: true,
          shellOutputCompression: 'auto',
        },
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
        ws: {
          defaultWorkingDir: 'C:/work',
          codebaseMemoryEnabled: true,
          shellOutputCompression: mode,
        },
      } as unknown as Partial<RecContext>),
    )
    expect(keys).not.toContain('shell-compress')
  })

  it('stays quiet when a sqz hook is already live', () => {
    const keys = keysOf(
      healthyCtx({
        tools: [sqzInstalled],
        hooks: [{ enabled: true, command: 'sqz hook claude' }],
        ws: {
          defaultWorkingDir: 'C:/work',
          codebaseMemoryEnabled: true,
          shellOutputCompression: 'auto',
        },
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
        ws: {
          defaultWorkingDir: 'C:/work',
          codebaseMemoryEnabled: false,
          shellOutputCompression: 'on',
        },
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
    expect(keysOf(healthyCtx({ servers: [] } as unknown as Partial<RecContext>))).toContain(
      'no-mcp',
    )
  })

  it('flags disabled backups', () => {
    const keys = keysOf(
      healthyCtx({ settings: { backupEnabled: false } } as unknown as Partial<RecContext>),
    )
    expect(keys).toContain('backup-off')
  })

  it('lists installed bare-CLI tools by name', () => {
    const recs = runRules(
      healthyCtx({
        tools: [
          { name: 'rtk', found: true, path: 'C:/rtk.exe', wire: 'hook' },
          { name: 'mmdc', found: true, wire: 'cli' },
          { name: 'ffmpeg', found: true, wire: 'cli' },
          { name: 'absent-cli', found: false, wire: 'cli' },
        ],
      } as unknown as Partial<RecContext>),
    )
    const cli = recs.find((r) => r.key === 'cli-tools')
    expect(cli?.desc).toContain('mmdc')
    expect(cli?.desc).toContain('ffmpeg')
    expect(cli?.desc).not.toContain('absent-cli')
  })
})

describe('tool-update', () => {
  // The rule's whole job: surface a real "newer version published" without ever
  // inventing one. Every case below is about that boundary.
  const withUpdates = (updates: unknown[], tools?: unknown[]) =>
    healthyCtx({
      updates,
      ...(tools ? { tools } : {}),
    } as unknown as Partial<RecContext>)

  it('fires for an outdated tool and names the published version', () => {
    const rec = runRules(
      withUpdates([{ name: 'piper', status: 'outdated', latest: 'v1.6.0' }]),
    ).find((r) => r.key === 'tool-update')
    expect(rec?.desc).toContain('piper → v1.6.0')
    expect(rec?.variant).toBe('warning')
  })

  it('stays silent when everything is current', () => {
    expect(
      keysOf(withUpdates([{ name: 'rtk', status: 'up-to-date', latest: 'v0.44.1' }])),
    ).not.toContain('tool-update')
  })

  // 'unknown' means a version could not be parsed on one side. Nudging an
  // upgrade on that guess would, for the manual tools, push the user into a
  // risky binary swap they did not need.
  it('does not treat an unparseable comparison as an update', () => {
    expect(
      keysOf(withUpdates([{ name: 'rtk', status: 'unknown', error: 'sürüm okunamadı' }])),
    ).not.toContain('tool-update')
  })

  // The probe catches its own network failure and yields [], so an offline
  // machine must simply produce no card — not a false "all up to date" claim
  // and not a crash that takes every other recommendation down with it.
  it('stays silent when the release check could not run', () => {
    expect(keysOf(withUpdates([]))).not.toContain('tool-update')
  })

  it('counts how many of the outdated tools can be updated in one click', () => {
    const rec = runRules(
      withUpdates(
        [
          { name: 'mmdc', status: 'outdated', latest: '12.0.0' },
          { name: 'piper', status: 'outdated', latest: 'v1.6.0' },
        ],
        [
          { name: 'mmdc', found: true, wire: 'cli', updateKind: 'command' },
          { name: 'piper', found: true, wire: '', updateKind: 'manual' },
        ],
      ),
    ).find((r) => r.key === 'tool-update')
    expect(rec?.desc).toContain('1 tanesi tek tıkla')
  })

  it('says everything is manual when none is package-manager backed', () => {
    const rec = runRules(
      withUpdates(
        [{ name: 'piper', status: 'outdated', latest: 'v1.6.0' }],
        [{ name: 'piper', found: true, wire: '', updateKind: 'manual' }],
      ),
    ).find((r) => r.key === 'tool-update')
    expect(rec?.desc).toContain('elle güncellenir')
    expect(rec?.desc).not.toContain('tek tıkla')
  })
})

describe('card ordering', () => {
  // Cards stack top-to-bottom, so a BLOCKING gap has to outrank the routine
  // nudges it would otherwise be buried under: a workspace with no agents cannot
  // run anything at all, while backup being off is a preference.
  it('puts a blocking gap above the setup suggestions', () => {
    const keys = keysOf(
      healthyCtx({
        agentsCount: 0,
        settings: { backupEnabled: false },
      } as unknown as Partial<RecContext>),
    )
    expect(keys[0]).toBe('no-agents')
    expect(keys.indexOf('no-agents')).toBeLessThan(keys.indexOf('backup-off'))
  })
})
