/// <reference types="astro/client" />

interface ImportMetaEnv {
  /**
   * Base URL of the release feed host, without a trailing slash. Overrides the
   * default in `site.config.ts` at build time, e.g.
   * `PUBLIC_FEED_URL=http://localhost:8080 npm run build` to preview against the
   * local `deploy/release-host` container.
   */
  readonly PUBLIC_FEED_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
