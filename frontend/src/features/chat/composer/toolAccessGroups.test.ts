import { describe, expect, it } from 'vitest'
import type { ToolAccessEntry } from '@/types'
import { filterTools, groupToolsByContext, toolsForServer } from './toolAccessGroups'

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

describe('groupToolsByContext', () => {
  it('splits a mix of built-in and MCP tools by context state, not by source', () => {
    const tools = [
      entry({ name: 'Bash', label: 'Bash', source: 'builtin', server: '', inContext: true }),
      entry({ name: 'mcp__x__read', label: 'read', inContext: true }),
      entry({ name: 'Grep', label: 'Grep', source: 'builtin', server: '', inContext: false }),
      entry({ name: 'mcp__x__write', label: 'write', inContext: false }),
    ]
    const groups = groupToolsByContext(tools)
    expect(groups.map((g) => g.key)).toEqual(['in-context', 'optional'])
    expect(groups[0].tools.map((t) => t.name)).toEqual(['Bash', 'mcp__x__read'])
    expect(groups[1].tools.map((t) => t.name)).toEqual(['Grep', 'mcp__x__write'])
  })

  it('drops empty groups', () => {
    const tools = [entry({ inContext: true })]
    expect(groupToolsByContext(tools).map((g) => g.key)).toEqual(['in-context'])
  })

  it('returns no groups for an empty list', () => {
    expect(groupToolsByContext([])).toEqual([])
  })

  it('sorts tools alphabetically by label within a group, in-context group first', () => {
    const tools = [
      entry({ name: 'z', label: 'zeta', inContext: true }),
      entry({ name: 'a', label: 'alpha', inContext: true }),
    ]
    expect(groupToolsByContext(tools)[0].tools.map((t) => t.label)).toEqual(['alpha', 'zeta'])
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
