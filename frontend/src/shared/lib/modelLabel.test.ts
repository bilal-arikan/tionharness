import { describe, expect, it } from 'vitest'
import type { CatalogEntry } from '@/types'
import { formatModelVersion, modelDisplayName, resolveModelLabel } from './modelLabel'

describe('formatModelVersion', () => {
  it('names the current id shape', () => {
    expect(formatModelVersion('claude-opus-5')).toBe('Opus 5')
    expect(formatModelVersion('claude-opus-4-8')).toBe('Opus 4.8')
    expect(formatModelVersion('claude-fable-5')).toBe('Fable 5')
  })

  it('drops the trailing date stamp', () => {
    expect(formatModelVersion('claude-sonnet-4-5-20250929')).toBe('Sonnet 4.5')
  })

  // Legacy ids put the version BEFORE the family; collecting digits separately
  // from the family token handles both orders without a per-scheme branch.
  it('handles the legacy version-first shape', () => {
    expect(formatModelVersion('claude-3-5-haiku-20241022')).toBe('Haiku 3.5')
  })

  // Never guess: an unrecognised family yields nothing so the caller can fall
  // back to the raw id rather than print a wrong name.
  it('returns empty for unknown families', () => {
    expect(formatModelVersion('gpt-5.5-pro')).toBe('')
    expect(formatModelVersion('')).toBe('')
  })
})

describe('modelDisplayName', () => {
  it('names a recognised model', () => {
    expect(modelDisplayName('claude-opus-5')).toBe('Opus 5')
  })

  // A spend row must never lie about what was billed, so an id we cannot name is
  // shown as-is rather than dropped or approximated.
  it('keeps an unrecognised id verbatim', () => {
    expect(modelDisplayName('MiniMax-M3')).toBe('MiniMax-M3')
  })

  it('marks a row with no model', () => {
    expect(modelDisplayName('')).toBe('(model belirtilmemiş)')
  })
})

const catalog: CatalogEntry[] = [
  {
    id: 'claude-cli',
    label: 'Claude CLI',
    needsKey: false,
    allowCustomModel: true,
    appliesToolHooks: true,
    available: true,
    models: [
      { id: '', label: 'claude oturum modeli', resolvedModel: 'claude-opus-5' },
      { id: 'sonnet', label: 'Sonnet — dengeli' },
    ],
  },
]

// A provider whose catalog offers no empty-id entry: leaving the model empty is
// then unexplained, and the label must not borrow another entry's name.
const catalogWithoutSessionEntry: CatalogEntry[] = [
  {
    id: 'openai',
    label: 'OpenAI',
    needsKey: true,
    allowCustomModel: true,
    appliesToolHooks: false,
    available: true,
    models: [{ id: 'gpt-5.5-pro', label: 'GPT-5.5 Pro' }],
  },
]

describe('resolveModelLabel', () => {
  it('prefers the observed resolution over the curated label', () => {
    expect(resolveModelLabel(catalog, 'claude-cli', '')).toBe('Opus 5')
  })

  it('falls back to the label when the alias has not resolved yet', () => {
    expect(resolveModelLabel(catalog, 'claude-cli', 'sonnet')).toBe('Sonnet')
  })

  it('prettifies a custom concrete id', () => {
    expect(resolveModelLabel(catalog, 'claude-cli', 'claude-haiku-4-5')).toBe('Haiku 4.5')
  })

  it('keeps an unrecognised custom id verbatim', () => {
    expect(resolveModelLabel(catalog, 'claude-cli', 'my-local-model')).toBe('my-local-model')
  })

  // Before anything has resolved, the empty-id entry still names the mode
  // honestly ("session model"), never the word "default".
  it('names the session-model entry when nothing has resolved yet', () => {
    const unresolved: CatalogEntry[] = [
      { ...catalog[0], models: [{ id: '', label: 'claude oturum modeli' }] },
    ]
    expect(resolveModelLabel(unresolved, 'claude-cli', '')).toBe('claude oturum modeli')
  })

  it('admits an unexplained empty model instead of borrowing another entry', () => {
    expect(resolveModelLabel(catalogWithoutSessionEntry, 'openai', '')).toBe(
      '(model belirtilmemiş)',
    )
  })
})
