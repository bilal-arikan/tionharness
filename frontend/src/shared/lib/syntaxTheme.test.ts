/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const css = readFileSync(new URL('../../index.css', import.meta.url), 'utf8')

describe('syntax highlight theme contract', () => {
  it('loads a dark palette and supplies explicit light-theme overrides', () => {
    expect(css).toContain("@import 'highlight.js/styles/github-dark.css'")
    expect(css).toMatch(/\[data-theme='light'\] \.hljs\s*\{/)
    expect(css).toMatch(/\[data-theme='light'\] \.hljs-keyword/)
    expect(css).toMatch(/\[data-theme='light'\] \.hljs-string/)
    expect(css).toMatch(/\[data-theme='light'\] \.hljs-comment/)
  })
})
