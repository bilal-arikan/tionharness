/**
 * The single source of truth for documentation order and labels. The sidebar,
 * the breadcrumb and the previous/next links all derive from this array; the
 * markdown frontmatter never decides where a page appears.
 *
 * A page listed here must have a matching `src/content/docs/<section>/<slug>.md`.
 * The mismatch is a build error, not a silent omission -- see `assertNavMatchesCollection`.
 */

export interface DocsPage {
  /** File name (without extension) inside the section directory. */
  slug: string
  label: string
}

export interface DocsSection {
  /** Directory name inside `src/content/docs`. */
  id: string
  label: string
  pages: DocsPage[]
}

export const docsNav: DocsSection[] = [
  {
    id: 'getting-started',
    label: 'Getting Started',
    pages: [
      { slug: 'introduction', label: 'Introduction' },
      { slug: 'installation', label: 'Installation' },
      { slug: 'quickstart', label: 'Quickstart' },
    ],
  },
  {
    id: 'concepts',
    label: 'Core Concepts',
    pages: [
      { slug: 'agents', label: 'Agents' },
      { slug: 'sessions', label: 'Sessions' },
      { slug: 'workspaces', label: 'Workspaces' },
    ],
  },
  {
    id: 'reference',
    label: 'Reference',
    pages: [{ slug: 'configuration', label: 'Configuration' }],
  },
]

/** One nav entry, flattened out of its section for order-based lookups. */
export interface DocsNavEntry {
  /** Collection id and URL tail: `<section>/<slug>`. */
  id: string
  label: string
  sectionId: string
  sectionLabel: string
  href: string
}

/** Reading order across every section, used for the previous/next footer. */
export const docsNavFlat: DocsNavEntry[] = docsNav.flatMap((section) =>
  section.pages.map((page) => ({
    id: `${section.id}/${page.slug}`,
    label: page.label,
    sectionId: section.id,
    sectionLabel: section.label,
    href: docsHref(`${section.id}/${page.slug}`),
  }))
)

export function docsHref(id: string): string {
  return `/docs/${id}`
}

/** The entry immediately before and after `id` in reading order. */
export function docsNeighbours(id: string): {
  prev: DocsNavEntry | null
  next: DocsNavEntry | null
} {
  const index = docsNavFlat.findIndex((entry) => entry.id === id)
  if (index === -1) {
    throw new Error(`docsNav: no navigation entry for "${id}"`)
  }
  return {
    prev: docsNavFlat[index - 1] ?? null,
    next: docsNavFlat[index + 1] ?? null,
  }
}

/**
 * Fails the build when the navigation and the markdown files disagree in either
 * direction. A missing file would render a dead sidebar link; an unlisted file
 * would be published with no way to reach it. Both are bugs, so neither is
 * swallowed.
 */
export function assertNavMatchesCollection(collectionIds: string[]): void {
  const present = new Set(collectionIds)
  const missing = docsNavFlat.filter((entry) => !present.has(entry.id)).map((entry) => entry.id)
  if (missing.length > 0) {
    throw new Error(
      `docsNav lists pages with no markdown file in src/content/docs: ${missing.join(', ')}`
    )
  }

  const listed = new Set(docsNavFlat.map((entry) => entry.id))
  const orphans = collectionIds.filter((id) => !listed.has(id))
  if (orphans.length > 0) {
    throw new Error(`src/content/docs has pages missing from docsNav: ${orphans.join(', ')}`)
  }
}
