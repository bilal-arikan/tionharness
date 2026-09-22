import { describe, expect, it } from 'vitest'
import { isZvecGrepServer } from './zvecGrep'

type Server = Parameters<typeof isZvecGrepServer>[0]

describe('isZvecGrepServer', () => {
  const matches: Array<[string, Server]> = [
    [
      'windows nvm shim',
      {
        transport: 'stdio',
        command: 'C:\\Users\\u\\.nvm\\versions\\node\\v24.18.1\\bin\\zg.cmd',
        args: '["server","--stdio"]',
      },
    ],
    ['bare shim', { transport: 'stdio', command: 'zg', args: '["server","--stdio"]' }],
    ['unix shim', { transport: 'stdio', command: '/usr/local/bin/zg', args: '' }],
    [
      'node entry point',
      {
        transport: 'stdio',
        command: 'node',
        args: '["C:/npm/node_modules/@zvec/zvec-grep/dist/cli/index.js","server","--stdio"]',
      },
    ],
    [
      'npx package',
      { transport: 'stdio', command: 'npx', args: '["-y","@zvec/zvec-grep","server","--stdio"]' },
    ],
  ]
  it.each(matches)('matches the %s', (_, server) => {
    expect(isZvecGrepServer(server)).toBe(true)
  })

  const rejects: Array<[string, Server]> = [
    ['zgrep', { transport: 'stdio', command: '/usr/bin/zgrep', args: '' }],
    ['a zg-prefixed tool', { transport: 'stdio', command: 'C:\\tools\\zg-helper.exe', args: '' }],
    ['an http server', { transport: 'http', command: 'zg', args: '' }],
    ['malformed args', { transport: 'stdio', command: 'node', args: 'not json' }],
    ['an unrelated server', { transport: 'stdio', command: 'npx', args: '["@playwright/mcp"]' }],
  ]
  it.each(rejects)('rejects %s', (_, server) => {
    expect(isZvecGrepServer(server)).toBe(false)
  })
})
