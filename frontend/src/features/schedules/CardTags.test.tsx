// @vitest-environment jsdom

import { act } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { setLocale } from '@/i18n'
import { query, render } from '@/test/render'
import { CardTags } from './CardTags'

const addButton = (c: HTMLElement) =>
  c.querySelector<HTMLButtonElement>('[data-testid="card-tags-add"]')
const input = (c: HTMLElement) => c.querySelector<HTMLInputElement>('input')

function press(el: Element, key: string) {
  act(() => {
    el.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))
  })
}

beforeEach(async () => {
  await setLocale('en')
})

describe('CardTags', () => {
  it('shows only the add action for a card without tags', () => {
    const { container } = render(<CardTags tags={[]} onChange={vi.fn()} />)

    expect(addButton(container)?.textContent).toContain('tag')
    expect(addButton(container)?.getAttribute('aria-label')).toBe('Add tag')
    expect(input(container)).toBeNull()
  })

  it('reveals and focuses the input when the action is used', () => {
    const { container } = render(<CardTags tags={[]} onChange={vi.fn()} />)

    act(() => addButton(container)!.click())

    expect(addButton(container)).toBeNull()
    expect(document.activeElement).toBe(query(container, 'input'))
  })

  it('folds again on Escape while empty and returns focus to the action', () => {
    const { container } = render(<CardTags tags={[]} onChange={vi.fn()} />)
    act(() => addButton(container)!.click())

    press(query(container, 'input'), 'Escape')

    expect(input(container)).toBeNull()
    expect(document.activeElement).toBe(addButton(container))
  })

  it('folds again when the empty input loses focus', () => {
    const { container } = render(<CardTags tags={[]} onChange={vi.fn()} />)
    act(() => addButton(container)!.click())

    act(() => query<HTMLInputElement>(container, 'input').blur())

    expect(input(container)).toBeNull()
    expect(addButton(container)).not.toBeNull()
  })

  it('keeps existing tags and their editor visible at rest', () => {
    const { container } = render(<CardTags tags={['weekend']} onChange={vi.fn()} />)

    expect(addButton(container)).toBeNull()
    expect(container.textContent).toContain('weekend')
    expect(input(container)).not.toBeNull()
  })
})
