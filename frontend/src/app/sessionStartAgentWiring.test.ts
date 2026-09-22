import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

function source(relative: string): string {
  return readFileSync(fileURLToPath(new URL(relative, import.meta.url)), 'utf8')
}

// The filter only protects the user if the session-start surfaces actually
// consume it. These assertions pin the wiring: a refactor that hands the raw
// roster back to the empty state would otherwise silently re-open the bug
// (TSK979) with every unit test still green.
describe('session-start agent wiring', () => {
  it('derives the startable subset in the controller and exports it', () => {
    const controller = source('./useSessionsController.ts')
    expect(controller).toContain("import { startableAgents } from './startableAgents'")
    expect(controller).toContain('const sessionStartAgents = useMemo(() => startableAgents(agents)')
    expect(controller).toContain('sessionStartAgents,')
  })

  it('opens a new session from the startable subset, not the raw roster', () => {
    const controller = source('./useSessionsController.ts')
    expect(controller).toContain('const aid = defaultAgentId ?? sessionStartAgents[0]?.id')
    expect(controller).not.toContain('const aid = defaultAgentId ?? agents[0]?.id')
  })

  it('feeds the empty state the startable subset', () => {
    const chatView = source('../features/chat/ChatView.tsx')
    expect(chatView).toContain('agents={sessionStartAgents}')
  })

  it('gates the sidebar new-chat button on a startable agent existing', () => {
    const app = source('./App.tsx')
    expect(app).toContain('newDisabled={ctl.sessionStartAgents.length === 0}')
    expect(app).toContain('sessionStartAgents={ctl.sessionStartAgents}')
  })
})
