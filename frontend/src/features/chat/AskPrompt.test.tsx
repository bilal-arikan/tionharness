// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AskPrompt } from './AskPrompt'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

function renderPrompt(props: Parameters<typeof AskPrompt>[0]) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() => root.render(<AskPrompt {...props} />))
  return container
}

function option(container: HTMLElement, text: string) {
  const button = [...container.querySelectorAll('button')].find(
    (candidate) => candidate.textContent === text,
  )
  if (!button) throw new Error(`Option not found: ${text}`)
  return button
}

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

describe('AskPrompt option tones', () => {
  it('classifies single-panel options and submits the clicked value', () => {
    const onAnswer = vi.fn()
    const container = renderPrompt({
      ask: { question: 'Devam?', options: ['  Onayla  ', 'İptal', 'Daha sonra'] },
      onAnswer,
    })

    expect(option(container, '  Onayla  ').className).toContain('var(--color-success)')
    expect(option(container, 'İptal').className).toContain('var(--color-danger)')
    expect(option(container, 'Daha sonra').className).toContain('var(--color-border)')

    act(() => option(container, '  Onayla  ').click())
    expect(onAnswer).toHaveBeenCalledWith('Onayla')
  })

  it('keeps differently-cased option text neutral', () => {
    const container = renderPrompt({
      ask: { question: 'Devam?', options: ['ONAYLA', 'İpTaL'] },
      onAnswer: vi.fn(),
    })

    expect(option(container, 'ONAYLA').className).toContain('var(--color-border)')
    expect(option(container, 'ONAYLA').className).not.toContain('var(--color-success)')
    expect(option(container, 'İpTaL').className).toContain('var(--color-border)')
    expect(option(container, 'İpTaL').className).not.toContain('var(--color-danger)')
  })

  it('classifies multi-panel options and submits clicked choices', () => {
    const onAnswer = vi.fn()
    const container = renderPrompt({
      ask: {
        question: '',
        questions: [
          { question: 'Uygula?', options: ['Onayla', 'Başka'] },
          { question: 'Emin misin?', options: ['İptal'] },
        ],
      },
      onAnswer,
    })

    expect(option(container, 'Onayla').className).toContain('var(--color-success)')
    expect(option(container, 'İptal').className).toContain('var(--color-danger)')
    expect(option(container, 'Başka').className).toContain('var(--color-border)')

    act(() => option(container, 'Onayla').click())
    act(() => option(container, 'İptal').click())
    act(() => option(container, 'Gönder').click())
    expect(onAnswer).toHaveBeenCalledWith(JSON.stringify(['Onayla', 'İptal']))
  })
})
