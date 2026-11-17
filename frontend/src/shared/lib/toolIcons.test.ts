import { Globe, Search } from 'lucide-react'
import { describe, expect, it } from 'vitest'
import { toolIcon } from './toolIcons'

describe('toolIcon', () => {
  it('gives every CLI web tool a web icon', () => {
    for (const name of ['WebFetch', 'webfetch']) expect(toolIcon(name)).toBe(Globe)
    for (const name of ['WebSearch', 'websearch', 'web_search']) expect(toolIcon(name)).toBe(Search)
  })
})
