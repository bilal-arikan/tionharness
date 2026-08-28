// Helpers for detecting file paths in chat text and resolving local media so
// the renderer can make paths clickable and show images inline.

const IMAGE_EXT = /\.(png|jpe?g|gif|webp|svg|bmp|ico|avif)$/i
const VIDEO_EXT = /\.(mp4|webm|ogg|ogv|mov|m4v)$/i

// Matches Windows (C:\...) and POSIX (/abs/..., ./rel/...) paths, plus bare
// dotted file names like internal/agent/titler.go. Kept deliberately strict to
// avoid turning ordinary prose into links.
const PATH_SRC =
  /(?:[A-Za-z]:\\[^\s"'`<>()]+|(?:\.{0,2}\/)[^\s"'`<>()]+|[\w.-]+\/[\w./-]+\.\w+|[\w-]+\.(?:go|ts|tsx|js|jsx|py|rs|md|json|sql|css|html|sh|yaml|yml|toml))/
    .source

// An absolute http(s) URL, or a scheme-less "www.host/..." one. Must be tried
// BEFORE PATH_SRC: the path pattern's "(?:\.{0,2}\/)" branch happily matches the
// "//host/path" tail of a URL, which is what used to turn a WebSearch/WebFetch
// URL into a bogus, unopenable file-path chip with its scheme cut off.
const URL_SRC = /(?:https?:\/\/|www\.)[^\s"'`<>]+/.source

const URL_ONLY_RE = new RegExp(`^${URL_SRC}$`, 'i')
const LINK_RE = new RegExp(`${URL_SRC}|${PATH_SRC}`, 'gi')

// Sentence punctuation that follows a URL in prose far more often than it is
// part of it — "see https://x.dev/a." must not link the trailing dot. Closing
// brackets are only dropped when unbalanced, so wiki-style ...(a_(b)) survives.
function trimUrlTail(url: string): { url: string; tail: string } {
  let end = url.length
  for (; end > 0; end--) {
    const ch = url[end - 1]
    if ('.,;:!?'.includes(ch)) continue
    if (ch === ')' || ch === ']' || ch === '}') {
      const open = ch === ')' ? '(' : ch === ']' ? '[' : '{'
      const head = url.slice(0, end)
      const balanced =
        head.split(open).length - 1 >= head.split(ch).length - 1 && head.includes(open)
      if (balanced) break
      continue
    }
    break
  }
  return { url: url.slice(0, end), tail: url.slice(end) }
}

/** True for an absolute http(s) URL — never a local path. */
export function isExternalUrl(s: string): boolean {
  return /^https?:\/\//i.test(s)
}

/** True when the whole string is a link (http(s):// or www.), nothing else. */
export function isUrl(s: string): boolean {
  return URL_ONLY_RE.test(s)
}

/** The href to navigate to: a scheme-less "www.host" link needs https://. */
export function urlHref(url: string): string {
  return isExternalUrl(url) ? url : `https://${url}`
}

export function isImagePath(p: string): boolean {
  return IMAGE_EXT.test(p)
}

export function isVideoPath(p: string): boolean {
  return VIDEO_EXT.test(p)
}

/** True for any inline-displayable media (image or video) by extension. */
export function isMediaPath(p: string): boolean {
  return IMAGE_EXT.test(p) || VIDEO_EXT.test(p)
}

/** Build the backend URL that streams a local image for inline display. A
 *  remote http(s) URL is already loadable and passes through untouched —
 *  wrapping it in /api/files would ask the backend for a file named
 *  "https://…" and 404. */
export function mediaUrl(path: string): string {
  if (isExternalUrl(path)) return path
  const clean = path.replace(/^file:\/\//, '')
  return `/api/files?path=${encodeURIComponent(clean)}`
}

export type SegmentKind = 'text' | 'path' | 'url'

export interface PathSegment {
  text: string
  kind: SegmentKind
}

/**
 * Split a plain-text string into alternating prose / file-path / URL segments so
 * the caller can render paths as clickable chips and URLs as real links. Used
 * for tool summaries and any non-markdown text where react-markdown isn't in
 * play.
 */
export function splitPaths(text: string): PathSegment[] {
  const out: PathSegment[] = []
  let last = 0
  for (const m of text.matchAll(LINK_RE)) {
    const idx = m.index ?? 0
    let hit = m[0]
    let tail = ''
    if (isUrl(hit)) {
      const trimmed = trimUrlTail(hit)
      hit = trimmed.url
      tail = trimmed.tail
    }
    if (idx > last) out.push({ text: text.slice(last, idx), kind: 'text' })
    out.push({ text: hit, kind: isUrl(hit) ? 'url' : 'path' })
    if (tail) out.push({ text: tail, kind: 'text' })
    last = idx + m[0].length
  }
  if (last < text.length) out.push({ text: text.slice(last), kind: 'text' })
  return out
}

/** Replace the current user's home directory prefix with ~ for display. */
const WIN_HOME_RE = /^[A-Za-z]:\\Users\\[^\\]+\\/i
const POSIX_HOME_RE = /^\/(?:home|Users)\/[^/]+\//
// Git Bash mounts Windows drives at /c/... (no \Users\ literal), so a shown
// Bash command path like /c/Users/bilal/Desktop/... needs its own pattern —
// it would not match WIN_HOME_RE (backslashes) or POSIX_HOME_RE (/home|/Users).
const GITBASH_HOME_RE = /^\/[a-z]\/Users\/[^/]+\//i

export function displayPath(p: string): string {
  if (WIN_HOME_RE.test(p)) return '~\\' + p.replace(WIN_HOME_RE, '')
  if (POSIX_HOME_RE.test(p)) return '~/' + p.replace(POSIX_HOME_RE, '')
  if (GITBASH_HOME_RE.test(p)) return '~/' + p.replace(GITBASH_HOME_RE, '')
  return p
}

/** A short, tail-end display form for long paths (…/dir/file.ext). */
export function shortPath(p: string, segments = 3): string {
  const tilde = displayPath(p)
  const parts = tilde.split(/[\\/]/).filter(Boolean)
  if (parts.length <= segments) return tilde
  return '…/' + parts.slice(-segments).join('/')
}
