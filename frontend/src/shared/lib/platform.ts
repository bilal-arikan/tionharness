// Platform helpers. Two different machines matter to the UI:
//
//  - the SERVER (where the Go backend runs commands and reads files): decides
//    shell semantics (PowerShell vs /bin/sh) and what a filesystem path looks
//    like, so path placeholders and shell hints follow it. Loaded from
//    GET /api/version by useServerOS (shared/hooks/useServerOS.ts).
//  - the BROWSER (where keys are pressed): decides whether the shortcut modifier
//    is shown as ⌘ or Ctrl.
//
// Kept free of any api import so it stays unit-testable in the node env.

/** The server OS families the UI distinguishes. */
export type ServerOS = 'windows' | 'darwin' | 'linux'

// Other Unix GOOS values behave like Linux for everything the UI cares about
// (POSIX paths, /bin/sh), so they fold onto 'linux'.
const UNIX_LIKE = new Set([
  'linux',
  'freebsd',
  'openbsd',
  'netbsd',
  'dragonfly',
  'solaris',
  'illumos',
  'aix',
])

/** Map a Go runtime.GOOS value to the UI's OS family (undefined when unknown). */
export function normalizeServerOS(goos: string | undefined | null): ServerOS | undefined {
  const v = (goos ?? '').trim().toLowerCase()
  if (v === 'windows') return 'windows'
  if (v === 'darwin' || v === 'ios') return 'darwin'
  if (UNIX_LIKE.has(v)) return 'linux'
  return undefined
}

/**
 * Pick a value by server OS. `other` covers the not-yet-loaded / unknown case
 * and should be neutral (e.g. "~/Projects/my-app").
 */
export function pickByOS<T>(
  os: ServerOS | undefined,
  choices: { windows: T; darwin: T; linux: T; other: T },
): T {
  return os ? choices[os] : choices.other
}

/** True when the BROWSER runs on an Apple platform (⌘ is the shortcut modifier). */
export function isApplePlatform(): boolean {
  if (typeof navigator === 'undefined') return false
  const nav = navigator as Navigator & { userAgentData?: { platform?: string } }
  const platform = nav.userAgentData?.platform || nav.platform || ''
  return /Mac|iPhone|iPad|iPod/i.test(platform)
}

/** The shortcut modifier label for the browser's platform: "⌘" or "Ctrl". */
export function modKeyLabel(): string {
  return isApplePlatform() ? '⌘' : 'Ctrl'
}

// Example filesystem paths for input placeholders, per server OS. Paths are not
// language-specific, so they live here rather than in the locale catalogs.
// Windows entries keep the original Windows-era examples; unknown OS gets a
// neutral "~/..." form.
const EXAMPLE_PATHS = {
  // A project / working directory (workspace create).
  project: {
    windows: 'C:\\Users\\...\\Desktop\\Projects\\my-app',
    darwin: '/Users/<you>/Projects/my-app',
    linux: '/home/<you>/projects/my-app',
    other: '~/Projects/my-app',
  },
  // A local plugin marketplace directory (Codex plugins).
  marketplace: {
    windows: 'C:\\...\\marketplace',
    darwin: '/Users/<you>/.../marketplace',
    linux: '/home/<you>/.../marketplace',
    other: '~/.../marketplace',
  },
  // The TionHarness source checkout (insight app-fix repo path).
  appRepo: {
    windows: 'C:/Users/.../TionHarness',
    darwin: '/Users/<you>/.../TionHarness',
    linux: '/home/<you>/.../TionHarness',
    other: '~/.../TionHarness',
  },
  // A local repository directory (skill import).
  repo: {
    windows: 'C:\\path\\to\\repo',
    darwin: '/path/to/repo',
    linux: '/path/to/repo',
    other: '/path/to/repo',
  },
} as const

export type ExamplePathKind = keyof typeof EXAMPLE_PATHS

/** A placeholder path of the given kind that looks native on the server OS. */
export function examplePath(os: ServerOS | undefined, kind: ExamplePathKind): string {
  return pickByOS<string>(os, EXAMPLE_PATHS[kind])
}
