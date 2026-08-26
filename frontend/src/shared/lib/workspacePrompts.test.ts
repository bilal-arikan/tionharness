import { describe, expect, it } from 'vitest'
import type { WorkspacePromptMeta } from '@/types'
import { changedEditablePrompts, isSystemOwnedPrompt } from './workspacePrompts'

const promptMeta: Record<string, WorkspacePromptMeta> = {
  title: {
    label: 'Başlık',
    hint: '',
    ownedBySystemKey: 'titler',
  },
  handoff: {
    label: 'Devir',
    hint: '',
  },
}

describe('workspace prompt ownership', () => {
  it('makes system-owned prompts read-only while leaving other prompts editable', () => {
    expect(isSystemOwnedPrompt('title', promptMeta)).toBe(true)
    expect(isSystemOwnedPrompt('handoff', promptMeta)).toBe(false)
  })

  it('excludes system-owned prompts from the save payload', () => {
    expect(
      changedEditablePrompts(
        ['title', 'handoff'],
        { title: 'stale override', handoff: 'changed handoff' },
        { title: 'effective soul', handoff: 'old handoff' },
        promptMeta,
      ),
    ).toEqual({ handoff: 'changed handoff' })
  })
})
