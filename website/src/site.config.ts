/**
 * Single source of truth for everything the project does not have yet.
 *
 * Placeholder policy: a value of `null` means "does not exist yet". Components
 * never emit a dead link for a null value -- `CTAButton` and `SmartLink` render
 * a disabled control with a "Coming soon" badge instead. When the repo goes
 * public, fill these in and the whole site becomes live; nothing else changes.
 */

export type Maybe<T> = T | null

export interface DownloadTargets {
  windows: Maybe<string>
  linux: Maybe<string>
  macos: Maybe<string>
}

export interface SiteConfig {
  name: string
  tagline: string
  description: string
  /** Canonical origin of this site. The domain is registered; DNS may lag. */
  url: string
  /**
   * Base URL of the release feed, without a trailing slash. `<feedUrl>/latest.json`
   * is fetched in the browser at runtime, so the build never depends on it being
   * live. Override with `PUBLIC_FEED_URL` to point at a local test host.
   */
  feedUrl: string
  repoUrl: Maybe<string>
  releasesUrl: Maybe<string>
  issuesUrl: Maybe<string>
  docsUrl: Maybe<string>
  demoVideoUrl: Maybe<string>
  downloads: DownloadTargets
  license: Maybe<string>
  version: Maybe<string>
  binarySizeMb: number
}

export const site: SiteConfig = {
  name: 'TionHarness',
  tagline: 'A multi-agent AI runtime that runs on your own machine.',
  description:
    'Self-hosted multi-agent AI workspace and control plane. One binary, no database, no API key.',

  url: 'https://tionharness.com',
  feedUrl: import.meta.env.PUBLIC_FEED_URL ?? 'https://tionharness.com',

  repoUrl: 'https://github.com/bilal-arikan/tionharness',
  issuesUrl: 'https://github.com/bilal-arikan/tionharness/issues',

  // TODO(placeholder): no tagged release exists yet, so the GitHub releases page
  // is empty -- linking it would send visitors to a blank list. `/releases` on
  // this site fills that role until the first tag ships.
  releasesUrl: null,
  docsUrl: '/docs/getting-started/introduction',
  demoVideoUrl: null,

  // TODO(placeholder): no published release artifacts yet. Artifacts now come
  // from the release feed (`lib/releaseFeedBuild.ts`), not from these fields.
  downloads: {
    windows: null,
    linux: null,
    macos: null,
  },

  license: 'Apache-2.0',
  // The advertised version comes from the release feed, not from this file, so
  // that a release never requires a config edit. Kept as the fallback for a
  // build with no feed at all.
  version: null,

  // Measured from a local `scripts\build.ps1` output.
  binarySizeMb: 12,
}

/** True when a placeholder link should render as disabled rather than as an anchor. */
export function isPending(value: Maybe<string>): value is null {
  return value === null || value === ''
}
