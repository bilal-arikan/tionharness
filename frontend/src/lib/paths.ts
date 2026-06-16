// Helpers for detecting file paths in chat text and resolving local media so
// the renderer can make paths clickable and show images inline.

const IMAGE_EXT = /\.(png|jpe?g|gif|webp|svg|bmp|ico|avif)$/i

// Matches Windows (C:\...) and POSIX (/abs/..., ./rel/...) paths, plus bare
// dotted file names like internal/agent/titler.go. Kept deliberately strict to
// avoid turning ordinary prose into links.
const PATH_RE =
  /(?:[A-Za-z]:\\[^\s"'`<>()]+|(?:\.{0,2}\/)[^\s"'`<>()]+|[\w.-]+\/[\w./-]+\.\w+|[\w-]+\.(?:go|ts|tsx|js|jsx|py|rs|md|json|sql|css|html|sh|yaml|yml|toml))/g

export function isImagePath(p: string): boolean {
  return IMAGE_EXT.test(p)
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

/** A short, tail-end display form for long paths (…/dir/file.ext). */
export function shortPath(p: string, segments = 3): string {
  const parts = p.split(/[\\/]/).filter(Boolean)
  if (parts.length <= segments) return p
  return '…/' + parts.slice(-segments).join('/')
}
