// @vitest-environment jsdom

import { describe, expect, it, vi } from 'vitest'
import { Clock } from 'lucide-react'
import { query, render } from '@/test/render'
import { BoardColumn } from './BoardColumn'

const DESCRIPTION = 'Cron / time based — the agent or flow runs when the expression is due.'

function renderColumn() {
  return render(
    <BoardColumn
      title="Schedules"
      icon={Clock}
      accent="#6b8e23"
      count={5}
      description={DESCRIPTION}
      onAdd={vi.fn()}
      addLabel="New schedule"
      loading={false}
      loadingLabel="Loading"
      emptyLabel="Nothing here"
    >
      <div />
    </BoardColumn>,
  ).container
}

describe('BoardColumn', () => {
  it('gives the title its own untruncated heading', () => {
    const title = query(renderColumn(), 'h2')

    expect(title.textContent).toBe('Schedules')
    expect(title.className).not.toContain('truncate')
  })

  it('puts the description on its own line, outside the title row', () => {
    const c = renderColumn()
    const description = query(c, '[data-testid="board-column-description"]')
    const titleRow = query(c, 'h2').parentElement

    expect(description.textContent).toBe(DESCRIPTION)
    expect(description.tagName).toBe('P')
    expect(titleRow?.contains(description)).toBe(false)
    // Count and add button stay with the title.
    expect(titleRow?.contains(query(c, '[data-testid="board-column-count"]'))).toBe(true)
    expect(titleRow?.contains(query(c, 'button[aria-label="New schedule"]'))).toBe(true)
  })

  it('keeps the lane frame neutral: the accent only tints the icon', () => {
    const c = renderColumn()
    const header = query<HTMLElement>(c, 'header')

    expect(header.style.borderTopColor).toBe('')
    expect(header.className).not.toContain('border-t-2')
    expect(query<SVGElement>(c, 'header svg').style.color).not.toBe('')
  })
})
