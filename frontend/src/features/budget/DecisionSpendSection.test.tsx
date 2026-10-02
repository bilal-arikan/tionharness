// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { i18next } from '@/i18n'
import type { DecisionSpendReport, DecisionSpendTotals } from '@/types/decisionSpend'
import { DecisionSpendSection } from './DecisionSpendSection'

function totals(overrides: Partial<DecisionSpendTotals> = {}): DecisionSpendTotals {
  return {
    calls: 0,
    failures: 0,
    inputTokens: 0,
    outputTokens: 0,
    unknownCostCalls: 0,
    primaryCalls: 0,
    challengerCalls: 0,
    testCalls: 0,
    costUSD: 0,
    reportedCostUSD: 0,
    estimatedCostUSD: 0,
    ...overrides,
  }
}

function report(overrides: Partial<DecisionSpendReport> = {}): DecisionSpendReport {
  return {
    days: 7,
    startedAt: 1760000000000,
    updatedAt: 1760000001000,
    storageError: false,
    today: totals(),
    totals: totals(),
    models: [],
    ...overrides,
  }
}

function view(value: DecisionSpendReport) {
  const container = document.createElement('div')
  container.innerHTML = renderToStaticMarkup(<DecisionSpendSection report={value} />)
  return container
}

beforeEach(async () => {
  await i18next.changeLanguage('en')
})

describe('DecisionSpendSection', () => {
  it('preserves positive micro-dollar costs instead of displaying zero', () => {
    const measured = totals({ calls: 1, costUSD: 0.00000234, reportedCostUSD: 0.00000234 })
    const container = view(
      report({
        today: measured,
        totals: measured,
        models: [{ provider: 'openrouter', model: 'typesafe/jev-1.13', ...measured }],
      }),
    )
    expect(container.textContent).toContain('$0.00000234')
    expect(container.textContent).toContain('OpenRouter · typesafe/jev-1.13')
    expect(container.textContent).not.toContain('< $0.00000001')
  })

  it('shows a lower bound for positive amounts smaller than eight decimal places', () => {
    const measured = totals({ calls: 1, costUSD: 0.0000000001, estimatedCostUSD: 0.0000000001 })
    const container = view(report({ today: measured, totals: measured }))
    expect(container.textContent).toContain('< $0.00000001')
    const headlines = [...container.querySelectorAll('.text-lg')].map((node) => node.textContent)
    expect(headlines).toEqual(['< $0.00000001', '< $0.00000001'])
  })

  it('keeps reported, estimated and unpriced calls distinct and includes role counts', () => {
    const measured = totals({
      calls: 12,
      inputTokens: 1234,
      outputTokens: 50,
      costUSD: 0.04,
      reportedCostUSD: 0.01,
      estimatedCostUSD: 0.03,
      unknownCostCalls: 2,
      primaryCalls: 7,
      challengerCalls: 3,
      testCalls: 2,
    })
    const container = view(
      report({ totals: measured, models: [{ provider: 'typesafe', model: 'jev', ...measured }] }),
    )
    expect(container.textContent).toContain('Provider reported: $0.01')
    expect(container.textContent).toContain('Estimated: $0.03')
    expect(container.textContent).toContain('Cost unavailable for 2 calls')
    expect(container.textContent).toContain('7 primary / fallback · 3 comparison · 2 test calls')
    expect(container.textContent).toContain('12 calls · 1,234 input / 50 output tokens')
    expect(container.textContent).toContain('TypeSafe · jev')
    expect(container.textContent).toContain('Across all workspaces')
    expect(container.textContent).toContain('not added to workspace totals')
  })

  it('renders measured zero separately from unavailable cost and handles an empty window', () => {
    const container = view(report({ startedAt: 0 }))
    expect(container.textContent).toContain('$0.00')
    expect(container.textContent).toContain('No decision model calls in this period.')
    expect(container.textContent).not.toContain('Cost unavailable')
    expect(container.textContent).not.toContain('Tracking since')
  })

  it('discloses incomplete storage and preserves local or unknown provider labels', () => {
    const container = view(
      report({
        storageError: true,
        models: [
          { provider: 'local', model: 'local-jev', ...totals({ calls: 1 }) },
          {
            provider: 'other-provider',
            model: 'external-jev',
            ...totals({ calls: 1, unknownCostCalls: 1 }),
          },
        ],
      }),
    )
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('incomplete')
    expect(container.textContent).toContain('Local · local-jev')
    expect(container.textContent).toContain('other-provider · external-jev')
    expect(container.textContent).toContain('Tracking since')
  })

  it('uses the active locale for micro-dollar amounts', async () => {
    await i18next.changeLanguage('tr')
    const container = view(report({ totals: totals({ calls: 1, costUSD: 0.00000234 }) }))
    expect(container.textContent).toContain('0,00000234')
    expect(container.textContent).not.toContain('0.00000234')
  })

  it('does not render nonfinite or negative monetary values', () => {
    const container = view(
      report({
        totals: totals({
          costUSD: Number.NaN,
          reportedCostUSD: Number.POSITIVE_INFINITY,
          estimatedCostUSD: -1,
        }),
      }),
    )
    expect(container.textContent).not.toMatch(/NaN|Infinity|∞|\$-1/)
    expect(container.textContent).toContain('$0.00')
  })
})
