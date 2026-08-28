import { describe, expect, it } from 'vitest'
import type { Artifact, ArtifactKind } from '@/types'
import { pickCardImage } from './cardImage'

function artifact(id: string, kind: ArtifactKind, createdAt = 0): Artifact {
  return {
    id,
    sessionId: '',
    agentId: '',
    title: id,
    kind,
    language: '',
    content: '',
    sourcePath: `uploads/${id}.png`,
    createdAt,
    updatedAt: createdAt,
  }
}

const index = (list: Artifact[]) => new Map(list.map((a) => [a.id, a]))

describe('pickCardImage', () => {
  it('returns null when the card has no artifact refs', () => {
    expect(pickCardImage(undefined, index([]))).toBeNull()
    expect(pickCardImage([], index([]))).toBeNull()
  })

  it('returns null when no linked artifact is an image', () => {
    const arts = index([artifact('a1', 'markdown'), artifact('a2', 'file')])
    expect(pickCardImage(['a1', 'a2'], arts)).toBeNull()
  })

  it('picks the LAST image in ref order, not the newest by createdAt', () => {
    const arts = index([
      artifact('old', 'image', 100), // created later…
      artifact('new', 'image', 1), // …but attached to the card last
    ])
    expect(pickCardImage(['old', 'new'], arts)?.id).toBe('new')
  })

  it('skips non-image refs that come after the last image', () => {
    const arts = index([
      artifact('img', 'image'),
      artifact('doc', 'markdown'),
      artifact('vid', 'video'),
    ])
    expect(pickCardImage(['img', 'doc', 'vid'], arts)?.id).toBe('img')
  })

  it('skips refs whose artifact no longer exists', () => {
    const arts = index([artifact('kept', 'image')])
    expect(pickCardImage(['kept', 'deleted'], arts)?.id).toBe('kept')
  })
})
