/**
 * Client for the release feed served at `<feedUrl>/latest.json`.
 *
 * The feed is fetched at RUNTIME in the browser, never at build time. The site
 * has to build and deploy before any release host exists, so a build-time fetch
 * would make every deploy depend on a live third host. The feed is served with
 * `Access-Control-Allow-Origin: *` for exactly this reason.
 *
 * Every failure path -- offline host, timeout, non-200, malformed payload --
 * collapses to `null`. The UI keeps its server-rendered "Coming soon" state in
 * that case: a broken download button is worse than no download button. Nothing
 * here is logged, because an unreachable feed is the expected state today, not
 * an error the visitor can act on.
 */

export type ReleaseOS = 'linux' | 'windows' | 'darwin'
export type ReleaseArch = 'amd64' | 'arm64'

export interface ReleaseArtifact {
  os: ReleaseOS
  arch: ReleaseArch
  file: string
  url: string
  sha256: string
  size: number
}

export interface Release {
  version: string
  released_at: string
  notesUrl: string | null
  artifacts: ReleaseArtifact[]
}

export interface Platform {
  os: ReleaseOS | null
  arch: ReleaseArch
}

/** Long enough for a cold CDN, short enough that the section never feels stuck. */
const FETCH_TIMEOUT_MS = 6000

const OS_LABELS: Record<ReleaseOS, string> = {
  windows: 'Windows',
  darwin: 'macOS',
  linux: 'Linux',
}

/** Presentation order, independent of the order the feed happens to use. */
const OS_ORDER: ReleaseOS[] = ['windows', 'darwin', 'linux']

export function osLabel(os: ReleaseOS): string {
  return OS_LABELS[os]
}

export function artifactLabel(artifact: ReleaseArtifact): string {
  return `${OS_LABELS[artifact.os]} ${artifact.arch}`
}

function isHttpUrl(value: unknown): value is string {
  if (typeof value !== 'string') return false
  try {
    const parsed = new URL(value)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

function parseArtifact(raw: unknown): ReleaseArtifact | null {
  if (typeof raw !== 'object' || raw === null) return null
  const value = raw as Record<string, unknown>
  const os = value.os
  const arch = value.arch
  if (os !== 'linux' && os !== 'windows' && os !== 'darwin') return null
  if (arch !== 'amd64' && arch !== 'arm64') return null
  if (typeof value.file !== 'string' || value.file === '') return null
  if (typeof value.sha256 !== 'string' || value.sha256 === '') return null
  if (typeof value.size !== 'number' || !Number.isFinite(value.size)) return null
  // A non-http url would turn into a dead or unsafe link, so the whole entry drops.
  if (!isHttpUrl(value.url)) return null
  return {
    os,
    arch,
    file: value.file,
    url: value.url,
    sha256: value.sha256,
    size: value.size,
  }
}

export function parseRelease(raw: unknown): Release | null {
  if (typeof raw !== 'object' || raw === null) return null
  const value = raw as Record<string, unknown>
  if (typeof value.version !== 'string' || value.version === '') return null
  if (typeof value.released_at !== 'string' || value.released_at === '') return null
  if (!Array.isArray(value.artifacts)) return null

  const artifacts = value.artifacts
    .map(parseArtifact)
    .filter((artifact): artifact is ReleaseArtifact => artifact !== null)
  // A release with no usable artifact is indistinguishable from no release.
  if (artifacts.length === 0) return null

  artifacts.sort((a, b) => {
    const byOs = OS_ORDER.indexOf(a.os) - OS_ORDER.indexOf(b.os)
    return byOs !== 0 ? byOs : a.arch.localeCompare(b.arch)
  })

  return {
    version: value.version,
    released_at: value.released_at,
    notesUrl: isHttpUrl(value.notes_url) ? value.notes_url : null,
    artifacts,
  }
}

export async function fetchLatestRelease(feedUrl: string): Promise<Release | null> {
  const base = feedUrl.replace(/\/+$/, '')
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS)
  try {
    const response = await fetch(`${base}/latest.json`, {
      signal: controller.signal,
      cache: 'no-store',
    })
    if (!response.ok) return null
    return parseRelease(await response.json())
  } catch {
    return null
  } finally {
    clearTimeout(timer)
  }
}

/**
 * Best-effort guess of what the visitor is running. The OS is reliable; the
 * architecture is not, so amd64 stays the default and every other artifact is
 * still listed next to the highlighted one.
 */
export function detectPlatform(userAgent: string): Platform {
  const ua = userAgent.toLowerCase()
  const arch: ReleaseArch = /arm64|aarch64/.test(ua) ? 'arm64' : 'amd64'

  let os: ReleaseOS | null = null
  if (ua.includes('windows')) os = 'windows'
  else if (ua.includes('mac os') || ua.includes('macintosh')) os = 'darwin'
  else if (ua.includes('linux') || ua.includes('android') || ua.includes('x11')) os = 'linux'

  // Apple silicon reports "Intel Mac OS X" in the UA string; the arm64 build is
  // the right default for a modern Mac, and Rosetta covers the misses.
  if (os === 'darwin' && !/intel mac os x 10_(9|1[0-4])/.test(ua)) {
    return { os, arch: 'arm64' }
  }
  return { os, arch }
}

export function pickPrimaryArtifact(
  release: Release,
  platform: Platform
): ReleaseArtifact | null {
  if (platform.os === null) return null
  const forOs = release.artifacts.filter((artifact) => artifact.os === platform.os)
  if (forOs.length === 0) return null
  return forOs.find((artifact) => artifact.arch === platform.arch) ?? forOs[0]
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  const mb = bytes / (1024 * 1024)
  if (mb < 1) return `${(bytes / 1024).toFixed(0)} KB`
  return `${mb.toFixed(1)} MB`
}

export function formatDate(isoDate: string): string {
  const parsed = new Date(isoDate)
  if (Number.isNaN(parsed.getTime())) return isoDate
  return parsed.toLocaleDateString('en-GB', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}
