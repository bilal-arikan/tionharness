// @vitest-environment jsdom

import { act, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { i18next } from '@/i18n'
import { PermissionPrompt } from './PermissionPrompt'
import { PlanPrompt } from './PlanPrompt'

const roots: Root[] = []

beforeEach(async () => {
  await i18next.changeLanguage('en')
})

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

function render(element: ReactNode) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() => root.render(element))
  return container
}

describe('localized approval choices', () => {
  it('shows translated permission labels while returning the backend value', () => {
    const onAnswer = vi.fn()
    const container = render(
      <PermissionPrompt
        ask={{
          kind: 'permission',
          question: '',
          options: ['İzin ver', 'Her zaman izin ver', 'Reddet'],
        }}
        onAnswer={onAnswer}
      />,
    )

    const allow = [...container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Allow once',
    )
    expect(allow).toBeDefined()
    act(() => allow?.click())
    expect(onAnswer).toHaveBeenCalledWith('İzin ver')
  })

  it('shows a translated plan choice while returning the backend value', () => {
    const onAnswer = vi.fn()
    const container = render(
      <PlanPrompt
        ask={{ kind: 'plan', question: '', options: ['Planı onayla', 'Reddet'] }}
        onAnswer={onAnswer}
      />,
    )

    const approve = [...container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Approve plan',
    )
    expect(approve).toBeDefined()
    act(() => approve?.click())
    expect(onAnswer).toHaveBeenCalledWith('Planı onayla')
  })
})
