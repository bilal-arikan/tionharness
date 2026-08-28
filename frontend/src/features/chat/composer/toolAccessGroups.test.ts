import { describe, expect, it } from 'vitest'
import type { ToolAccessEntry } from '@/types'
import { filterTools, sortTools, toolsForServer } from './toolAccessGroups'

function entry(over: Partial<ToolAccessEntry>): ToolAccessEntry {
  return {
    name: 'mcp__x__a',
    label: 'a',
    description: '',
    source: 'mcp',
    server: 'x',
    visibility: 'full',
    inContext: true,
    ...over,
  }
}

describe('toolsForServer', () => {
  it('keeps only the given server and puts eager tools before lazy ones', () => {
    const eager = [
      entry({ name: 'mcp__x__zeta', label: 'zeta' }),
      entry({ name: 'mcp__x__alpha', label: 'alpha' }),
      entry({ name: 'mcp__y__beta', label: 'beta', server: 'y' }),
    ]
    const lazy = [
      entry({ name: 'mcp__x__gamma', label: 'gamma', inContext: false }),
      entry({ name: 'mcp__x__beta', label: 'beta', inContext: false }),
    ]
    expect(toolsForServer({ eager, lazy }, 'x').map((t) => t.label)).toEqual([
      'alpha',
      'zeta',
      'beta',
      'gamma',
    ])
  })

  it('ignores built-in tools that happen to carry a server name', () => {
    const eager = [entry({ name: 'Bash', label: 'Bash', source: 'builtin', server: 'x' })]
    expect(toolsForServer({ eager, lazy: [] }, 'x')).toEqual([])
  })

  it('returns an empty list for a server with no tools', () => {
    expect(toolsForServer({ eager: [entry({})], lazy: [] }, 'other')).toEqual([])
  })

  it('does not mutate the input arrays', () => {
    const eager = [entry({ label: 'z' }), entry({ label: 'a', name: 'mcp__x__a2' })]
    toolsForServer({ eager, lazy: [] }, 'x')
    expect(eager.map((t) => t.label)).toEqual(['z', 'a'])
  })
})

describe('sortTools', () => {
  it('orders by label, mixing built-in and MCP tools in one list', () => {
    const tools = [
      entry({ name: 'mcp__x__zeta', label: 'zeta' }),
      entry({ name: 'Bash', label: 'Bash', source: 'builtin', server: '' }),
      entry({ name: 'mcp__x__alpha', label: 'alpha' }),
    ]
    expect(sortTools(tools).map((t) => t.label)).toEqual(['alpha', 'Bash', 'zeta'])
  })

  it('does not mutate the input array', () => {
    const tools = [entry({ label: 'zeta' }), entry({ name: 'mcp__x__a2', label: 'alpha' })]
    sortTools(tools)
    expect(tools.map((t) => t.label)).toEqual(['zeta', 'alpha'])
  })

  it('returns an empty list unchanged', () => {
    expect(sortTools([])).toEqual([])
  })
})

describe('filterTools', () => {
  it('matches label, server and description case-insensitively', () => {
    const tools = [
      entry({ name: 'mcp__x__read', label: 'read', description: 'Read a file' }),
      entry({ name: 'mcp__y__write', label: 'write', server: 'y', description: 'Write' }),
    ]
    expect(filterTools(tools, 'FILE').map((t) => t.label)).toEqual(['read'])
    expect(filterTools(tools, 'y').map((t) => t.label)).toEqual(['write'])
    expect(filterTools(tools, '  ')).toHaveLength(2)
  })
})
