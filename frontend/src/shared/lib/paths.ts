// Helpers for detecting file paths in chat text and resolving local media so
// the renderer can make paths clickable and show images inline.

const IMAGE_EXT = /\.(png|jpe?g|gif|webp|svg|bmp|ico|avif)$/i
const VIDEO_EXT = /\.(mp4|webm|ogg|ogv|mov|m4v)$/i

// Matches Windows (C:\...) and POSIX (/abs/..., ./rel/...) paths, plus bare
// dotted file names like internal/agent/titler.go. Kept deliberately strict to
// avoid turning ordinary prose into links.
const PATH_RE =
  /(?:[A-Za-z]:\\[^\s"'`<>()]+|(?:\.{0,2}\/)[^\s"'`<>()]+|[\w.-]+\/[\w./-]+\.\w+|[\w-]+\.(?:go|ts|tsx|js|jsx|py|rs|md|json|sql|css|html|sh|yaml|yml|toml))/g

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

/** Build the backend URL that streams a local image for inline display. */
export function mediaUrl(path: string): string {
  const clean = path.replace(/^file:\/\//, '')
  return `/api/files?path=${encodeURIComponent(clean)}`
}

export interface PathSegment {
  text: string
  isPath: boolean
}

/**
 * Split a plain-text string into alternating prose / file-path segments so the
 * caller can render paths as clickable chips. Used for tool summaries and any
 * non-markdown text where react-markdown isn't in play.
 */
export function splitPaths(text: string): PathSegment[] {
  const out: PathSegment[] = []
  let last = 0
  for (const m of text.matchAll(PATH_RE)) {
    const idx = m.index ?? 0
    if (idx > last) out.push({ text: text.slice(last, idx), isPath: false })
    out.push({ text: m[0], isPath: true })
    last = idx + m[0].length
  }
  if (last < text.length) out.push({ text: text.slice(last), isPath: false })
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
