import { defineCollection, z } from 'astro:content'
import { glob } from 'astro/loaders'

/**
 * Only `src/content/docs/**` is a content collection. The other `src/content/*.ts`
 * files are plain data modules imported directly by components, so the glob
 * pattern stays scoped to the docs subtree.
 */
const docs = defineCollection({
  loader: glob({ pattern: '**/*.md', base: './src/content/docs' }),
  schema: z.object({
    title: z.string(),
    description: z.string(),
    /** Position within the owning section. Display order itself comes from docsNav.ts. */
    order: z.number(),
  }),
})

export const collections = { docs }
