/**
 * Build-time resolution of the current release.
 *
 * The feed a visitor polls at `https://tionharness.com/latest.json` is the very
 * file this build deploys (`website/public/latest.json`, copied verbatim by
 * Astro). Reading it from disk during the build is therefore both the freshest
 * and the only non-circular source: fetching the live site would return the
 * PREVIOUS deploy's feed. `PUBLIC_FEED_URL` overrides the source with a real
 * HTTP fetch, which is what the local release-host container is for.
 *
 * Failure never breaks the build -- an unreachable host, a malformed payload or
 * a missing file all collapse to `null`, and the components fall back to their
 * "Coming soon" state. Every failure is written to the build log, though: the
 * one outcome worse than no version is a wrong version shown silently.
 *
 * Node-only. Import this from `.astro` frontmatter, never from a client
 * `<script>` -- those keep using `fetchLatestRelease` in `releaseFeed.ts`.
 */

import { readFile } from 'node:fs/promises'
import { parseRelease, type Release } from './releaseFeed'

/** Resolved against this module, so the build's cwd does not matter. */
const LOCAL_FEED = new URL('../../public/latest.json', import.meta.url)

/** Long enough for a cold CDN, short enough not to stall a build. */
const FETCH_TIMEOUT_MS = 6000

let pending: Promise<Release | null> | undefined

/**
 * The release this build advertises, or `null` when there is none. Memoised:
 * every component that needs the version shares one read.
 */
export function loadReleaseAtBuild(): Promise<Release | null> {
  pending ??= resolveRelease()
  return pending
}

async function resolveRelease(): Promise<Release | null> {
  const override = import.meta.env.PUBLIC_FEED_URL
  const raw = override ? await readRemoteFeed(override) : await readLocalFeed()
  if (raw === null) return null

  const release = parseRelease(raw)
  if (release === null) {
    // Reached the feed but it advertises nothing installable (the pre-release
    // placeholder, or a payload whose artifacts all failed validation).
    warn('feed carries no usable release artifact; rendering the "Coming soon" state')
  }
  return release
}

async function readLocalFeed(): Promise<unknown> {
  let text: string
  try {
    text = await readFile(LOCAL_FEED, 'utf8')
  } catch (error) {
    warn(`cannot read ${LOCAL_FEED.pathname}: ${message(error)}`)
    return null
  }
  return parseJson(text, LOCAL_FEED.pathname)
}

async function readRemoteFeed(feedUrl: string): Promise<unknown> {
  const url = `${feedUrl.replace(/\/+$/, '')}/latest.json`
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS)
  try {
    const response = await fetch(url, { signal: controller.signal, cache: 'no-store' })
    if (!response.ok) {
      warn(`${url} answered HTTP ${response.status}`)
      return null
    }
    return parseJson(await response.text(), url)
  } catch (error) {
    warn(`${url} is unreachable: ${message(error)}`)
    return null
  } finally {
    clearTimeout(timer)
  }
}

function parseJson(text: string, source: string): unknown {
  try {
    return JSON.parse(text)
  } catch (error) {
    warn(`${source} is not valid JSON: ${message(error)}`)
    return null
  }
}

function message(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function warn(detail: string): void {
  console.warn(`[release-feed] ${detail}`)
}
