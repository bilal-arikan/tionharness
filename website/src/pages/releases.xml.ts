/**
 * RSS 2.0 feed of releases, emitted as a static file by `astro build`.
 *
 * The release feed only ever describes the CURRENT release, so this channel
 * carries at most one item. That is deliberate: an empty-but-valid channel is
 * what a reader should see before the first tag ships, and inventing a history
 * out of data the project does not keep would be worse than one item.
 */

import type { APIRoute } from 'astro'
import { site } from '../site.config'
import { loadReleaseAtBuild } from '../lib/releaseFeedBuild'
import { artifactLabel, formatSize, type Release } from '../lib/releaseFeed'

export const GET: APIRoute = async () => {
  const release = await loadReleaseAtBuild()
  const xml = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">
  <channel>
    <title>${escapeXml(`${site.name} releases`)}</title>
    <link>${escapeXml(absolute('/releases'))}</link>
    <description>${escapeXml(`Release announcements for ${site.name}.`)}</description>
    <language>en</language>
    <atom:link href="${escapeXml(absolute('/releases.xml'))}" rel="self" type="application/rss+xml" />
${release ? item(release) : ''}  </channel>
</rss>
`

  return new Response(xml, {
    headers: { 'Content-Type': 'application/rss+xml; charset=utf-8' },
  })
}

function item(release: Release): string {
  const title = `${site.name} v${release.version}`
  // The release notes are the better target when they exist; otherwise the
  // on-site page, which lists the same artifacts and checksums.
  const link = release.notesUrl ?? absolute('/releases')
  const lines = release.artifacts.map(
    (artifact) => `${artifactLabel(artifact)}: ${artifact.file} (${formatSize(artifact.size)})`
  )

  return `    <item>
      <title>${escapeXml(title)}</title>
      <link>${escapeXml(link)}</link>
      <guid isPermaLink="false">${escapeXml(`${site.url}/releases#v${release.version}`)}</guid>
${pubDate(release.released_at)}      <description>${escapeXml(lines.join('\n'))}</description>
    </item>
`
}

/** Omitted rather than guessed when the feed carries an unparsable timestamp. */
function pubDate(isoDate: string): string {
  const parsed = new Date(isoDate)
  if (Number.isNaN(parsed.getTime())) return ''
  return `      <pubDate>${parsed.toUTCString()}</pubDate>\n`
}

function absolute(path: string): string {
  return new URL(path, site.url).href
}

function escapeXml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;')
}
