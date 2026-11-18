// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Schedule } from '@/types'
import { setLocale } from '@/i18n'
import { query, render } from '@/test/render'
import { ScheduleCard } from './ScheduleCard'

function schedule(patch: Partial<Schedule> = {}): Schedule {
  return {
    id: 'SCH1',
    agentId: 'AGT1',
    cronExpr: '0 0 * * *',
    prompt: 'find weekend events',
    nextRunAt: 0,
    lastRunAt: 0,
    lastDeliveryStatus: '',
    lastDeliveryError: '',
    enabled: true,
    createdAt: 0,
    ...patch,
  }
}

function renderCard(s: Schedule) {
  return render(
    <ScheduleCard
      schedule={s}
      agents={[]}
      highlighted={false}
      running={false}
      onToggle={vi.fn()}
      onRunNow={vi.fn()}
      onEdit={vi.fn()}
      onTags={vi.fn()}
    />,
  ).container
}

beforeEach(async () => {
  await setLocale('en')
})

describe('ScheduleCard', () => {
  it('gives an unnamed rule a fallback title and keeps the cron on its own line', () => {
    const c = renderCard(schedule())

    expect(query(c, '[data-testid="schedule-title"]').textContent).toBe('Untitled schedule')
    expect(query(c, '[data-testid="schedule-cron"]').textContent).toBe('0 0 * * *')
  })

  it('renders the cron line the same way for named and unnamed rules', () => {
    const named = query(renderCard(schedule({ name: 'Nightly' })), '[data-testid="schedule-cron"]')
    const unnamed = query(renderCard(schedule()), '[data-testid="schedule-cron"]')

    expect(named.className).toBe(unnamed.className)
    expect(named.className).not.toContain('--color-accent')
  })

  it('shows the name when there is one', () => {
    const c = renderCard(schedule({ name: 'Nightly' }))

    expect(query(c, '[data-testid="schedule-title"]').textContent).toBe('Nightly')
  })

  it('offers a tag action instead of an empty tag input', () => {
    const c = renderCard(schedule())

    expect(c.querySelector('[data-testid="card-tags-add"]')).not.toBeNull()
    expect(c.querySelector('input')).toBeNull()
  })

  it('keeps the enable switch addressable and reflecting state', () => {
    const toggle = query(renderCard(schedule({ enabled: false })), '[role="switch"]')

    expect(toggle.getAttribute('data-testid')).toBe('schedule-enable-toggle')
    expect(toggle.getAttribute('data-schedule-id')).toBe('SCH1')
    expect(toggle.getAttribute('aria-checked')).toBe('false')
  })
})
