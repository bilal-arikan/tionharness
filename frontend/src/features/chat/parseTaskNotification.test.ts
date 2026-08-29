import { describe, expect, it } from 'vitest'
import { notificationChanges, parseTaskNotification } from './parseTaskNotification'

describe('parseTaskNotification', () => {
  it('reads worker identity fields used by AgentIdentity', () => {
    const parsed = parseTaskNotification(`<task-notification>
<task-id>SES9</task-id>
<agent-id>A7</agent-id>
<agent>Scout</agent>
<model>opus-5</model>
<status>completed</status>
</task-notification>`)

    expect(parsed).toMatchObject({
      taskId: 'SES9',
      agentId: 'A7',
      agent: 'Scout',
      model: 'opus-5',
      status: 'completed',
    })
  })

  it('keeps legacy notifications parseable with empty identity details', () => {
    expect(
      parseTaskNotification(
        '<task-notification><agent>Scout</agent><status>completed</status></task-notification>',
      ),
    ).toMatchObject({ agentId: '', agent: 'Scout', model: '' })
  })
})

describe('notificationChanges', () => {
  it('extracts every top-level and nested file diff using normal message rules', () => {
    const steps = JSON.stringify([
      { kind: 'diff', tool: 'Edit', path: 'a.ts', patch: '-old\n+new', added: 1, removed: 1 },
      {
        kind: 'subagent',
        subSteps: [{ kind: 'diff', tool: 'Write', path: 'b.ts', patch: '+new', added: 1 }],
      },
      { kind: 'tool', tool: 'Read', output: 'ignored' },
    ])

    expect(notificationChanges(steps).map((step) => step.path)).toEqual(['a.ts', 'b.ts'])
  })

  it('does not hide malformed persisted steps', () => {
    expect(() => notificationChanges('{')).toThrow()
  })
})
