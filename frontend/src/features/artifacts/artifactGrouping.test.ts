import { describe, expect, it } from 'vitest'
import type { Artifact } from '@/types'
import { compareText } from '@/shared/lib/intl'
import {
  ARTIFACTS_PAGE_SIZE,
  UNGROUPED,
  artifactGroupKey,
  artifactId,
  draftFromArtifact,
  sortArtifactGroups,
} from './artifactGrouping'

function artifact(over: Partial<Artifact> = {}): Artifact {
  return {
    id: 'ART1',
    sessionId: 'SES1',
    agentId: '',
    title: 'Başlık',
    kind: 'markdown',
    language: '',
    content: 'gövde',
    origin: 'manual',
    createdAt: 1,
    updatedAt: 1,
    ...over,
  }
}

describe('artifactGroupKey', () => {
  it('uses the trimmed group name when one is set', () => {
    expect(artifactGroupKey(artifact({ group: 'Raporlar' }))).toBe('Raporlar')
  })

  it('falls back to the ungrouped bucket for missing, empty and blank groups', () => {
    expect(artifactGroupKey(artifact())).toBe(UNGROUPED)
    expect(artifactGroupKey(artifact({ group: '' }))).toBe(UNGROUPED)
    expect(artifactGroupKey(artifact({ group: '   ' }))).toBe(UNGROUPED)
  })

  it('trims the returned key so padded spellings share one bucket', () => {
    expect(artifactGroupKey(artifact({ group: ' Raporlar ' }))).toBe('Raporlar')
    expect(artifactGroupKey(artifact({ group: 'Raporlar' }))).toBe(
      artifactGroupKey(artifact({ group: '\tRaporlar\n' })),
    )
  })
})

describe('artifactId', () => {
  it('returns the artifact id', () => {
    expect(artifactId(artifact({ id: 'ART9' }))).toBe('ART9')
  })
})

describe('sortArtifactGroups', () => {
  it('always orders the ungrouped bucket first', () => {
    // 'alfa' collates BEFORE 'Grupsuz', so an implementation that merely fell
    // through to compareText would order it the other way round — this is what
    // proves the dedicated ungrouped branches are doing the work.
    expect(compareText(UNGROUPED, 'alfa')).toBeGreaterThan(0)
    expect(sortArtifactGroups(UNGROUPED, 'alfa')).toBe(-1)
    expect(sortArtifactGroups('alfa', UNGROUPED)).toBe(1)
    expect(sortArtifactGroups(UNGROUPED, 'Raporlar')).toBe(-1)
    expect(sortArtifactGroups('Raporlar', UNGROUPED)).toBe(1)
  })

  it('orders named groups alphabetically with Turkish collation', () => {
    expect(sortArtifactGroups('alfa', 'beta')).toBeLessThan(0)
    expect(sortArtifactGroups('beta', 'alfa')).toBeGreaterThan(0)
    expect(sortArtifactGroups('alfa', 'alfa')).toBe(0)
    // 'ç' sorts right after 'c' in tr, before 'd' — plain code-point order would
    // push it past every ASCII letter.
    expect(sortArtifactGroups('çilek', 'dut')).toBeLessThan(0)
    expect(sortArtifactGroups('cam', 'çilek')).toBeLessThan(0)
  })

  it('sorts a full group list ungrouped-first, then alphabetically', () => {
    const names = ['zeta', UNGROUPED, 'alfa', 'çilek', 'cam']
    expect([...names].sort(sortArtifactGroups)).toEqual([UNGROUPED, 'alfa', 'cam', 'çilek', 'zeta'])
  })
})

describe('draftFromArtifact', () => {
  it('copies the editable fields verbatim', () => {
    const a = artifact({
      title: 'Notlar',
      kind: 'code',
      language: 'go',
      content: 'package main',
      group: 'Raporlar',
    })
    expect(draftFromArtifact(a)).toEqual({
      title: 'Notlar',
      kind: 'code',
      language: 'go',
      content: 'package main',
      group: 'Raporlar',
    })
  })

  it('maps a missing group to the empty string', () => {
    expect(draftFromArtifact(artifact()).group).toBe('')
  })

  it('keeps a whitespace-only group as typed so the dirty check can see it', () => {
    expect(draftFromArtifact(artifact({ group: '  ' })).group).toBe('  ')
  })

  it('does not carry non-editable fields into the draft', () => {
    // No cast: Object.keys takes any object and hands back string[], so the
    // assertion below reads the key list off a plain Draft. The Record cast this
    // used to carry was never needed for the check and only hid the mismatch it
    // was widening around.
    const draft = draftFromArtifact(artifact({ id: 'ART7' }))
    expect(Object.keys(draft).sort()).toEqual(['content', 'group', 'kind', 'language', 'title'])
  })
})

describe('ARTIFACTS_PAGE_SIZE', () => {
  it('stays within the API page cap of 100', () => {
    expect(ARTIFACTS_PAGE_SIZE).toBe(50)
    expect(ARTIFACTS_PAGE_SIZE).toBeLessThanOrEqual(100)
  })
})
