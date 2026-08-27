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
  feedUrl: import.meta.env.PUBLIC_FEED_URL ?? 'https://dl.tionharness.com',

  // TODO(placeholder): public repository is not published yet.
  repoUrl: null,
  releasesUrl: null,
  issuesUrl: null,
  docsUrl: null,
  demoVideoUrl: null,

  // TODO(placeholder): no published release artifacts yet.
  downloads: {
    windows: null,
    linux: null,
    macos: null,
  },

  // TODO(placeholder): license not chosen yet.
  license: null,
  version: null,

  // Measured from a local `scripts\build.ps1` output.
  binarySizeMb: 12,
}

/** True when a placeholder link should render as disabled rather than as an anchor. */
export function isPending(value: Maybe<string>): value is null {
  return value === null || value === ''
}
