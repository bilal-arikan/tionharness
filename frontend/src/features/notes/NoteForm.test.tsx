// @vitest-environment jsdom

import { act } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { setLocale } from '@/i18n'
import { render, query } from '@/test/render'
import { EMPTY_NOTE_WRITE } from './notesHelpers'
import { NoteForm } from './NoteForm'

function type(input: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement, value: string) {
  act(() => {
    const proto =
      input instanceof HTMLTextAreaElement
        ? window.HTMLTextAreaElement.prototype
        : input instanceof HTMLSelectElement
          ? window.HTMLSelectElement.prototype
          : window.HTMLInputElement.prototype
    Object.getOwnPropertyDescriptor(proto, 'value')?.set?.call(input, value)
    input.dispatchEvent(
      new Event(input instanceof HTMLSelectElement ? 'change' : 'input', {
        bubbles: true,
      }),
    )
  })
}

describe('NoteForm validation', () => {
  beforeEach(async () => {
    await setLocale('en')
  })
  afterEach(async () => {
    await setLocale('tr')
  })

  it('refuses an empty submit, shows the required-field errors and submits once fixed', () => {
    const onSubmit = vi.fn()
    const { container } = render(
      <NoteForm
        mode="create"
        initial={EMPTY_NOTE_WRITE}
        submitting={false}
        onSubmit={onSubmit}
        onCancel={() => {}}
      />,
    )
    // Nothing is flagged before the first attempt.
    expect(container.querySelectorAll('[role="alert"]')).toHaveLength(0)

    act(() => query<HTMLButtonElement>(container, '[data-testid="note-form-submit"]').click())
    const alerts = [...container.querySelectorAll('[role="alert"]')].map((a) => a.textContent)
    expect(alerts).toEqual(['A title is required.', 'The body cannot be empty.'])
    expect(onSubmit).not.toHaveBeenCalled()

    type(query<HTMLInputElement>(container, '[data-testid="note-form-title"]'), '  Use the cache ')
    type(query<HTMLTextAreaElement>(container, '[data-testid="note-form-body"]'), 'Always warm it.')
    expect(container.querySelectorAll('[role="alert"]')).toHaveLength(0)

    act(() => query<HTMLButtonElement>(container, '[data-testid="note-form-submit"]').click())
    expect(onSubmit).toHaveBeenCalledTimes(1)
    expect(onSubmit.mock.calls[0][0]).toMatchObject({
      kind: 'lesson',
      title: 'Use the cache',
      body: 'Always warm it.',
      scope: 'workspace',
      agents: [],
      projects: [],
      confidence: 'inferred',
      verification: '',
      private: false,
    })
  })

  it('requires the reach list and the verification the scope / confidence call for', () => {
    const onSubmit = vi.fn()
    const { container } = render(
      <NoteForm
        mode="create"
        initial={{ ...EMPTY_NOTE_WRITE, title: 'T', body: 'B' }}
        submitting={false}
        onSubmit={onSubmit}
        onCancel={() => {}}
      />,
    )
    type(query<HTMLSelectElement>(container, '[data-testid="note-form-scope"]'), 'agent')
    type(query<HTMLSelectElement>(container, '[data-testid="note-form-confidence"]'), 'verified')
    act(() => query<HTMLButtonElement>(container, '[data-testid="note-form-submit"]').click())
    expect(onSubmit).not.toHaveBeenCalled()
    const alerts = [...container.querySelectorAll('[role="alert"]')].map((a) => a.textContent)
    expect(alerts).toEqual([
      'An agent-scoped note must name at least one agent.',
      'Say how a verified note was checked.',
    ])

    type(query<HTMLInputElement>(container, '[data-testid="note-form-agents"]'), 'AGT1, AGT2')
    type(
      query<HTMLInputElement>(container, '[data-testid="note-form-verification"]'),
      'ran the suite',
    )
    act(() => query<HTMLButtonElement>(container, '[data-testid="note-form-submit"]').click())
    expect(onSubmit).toHaveBeenCalledTimes(1)
    expect(onSubmit.mock.calls[0][0]).toMatchObject({
      scope: 'agent',
      agents: ['AGT1', 'AGT2'],
      confidence: 'verified',
      verification: 'ran the suite',
    })
  })
})
