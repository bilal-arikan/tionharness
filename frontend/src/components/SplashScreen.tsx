// SplashScreen is the minimal first-run loading screen. It is shown ONLY on a
// fresh install (no setup yet) while the initial workspace list is still
// resolving AND for a brief minimum duration so it never flickers — returning
// users skip it entirely and land straight in the app. Self-contained: the only
// asset is the brand logo served from /favicon.svg (present in both the Vite dev
// server and the embedded single-binary build), and everything else is theme-
// variable driven so it matches the active appearance before any workspace loads.
export function SplashScreen() {
  return (
    <div
      data-testid="splash-screen"
      className="flex h-full w-full flex-col items-center justify-center gap-6 bg-[var(--color-bg)] text-[var(--color-text)]"
      style={{ animation: 'tionswarm-splash-fade 280ms ease-out' }}
    >
      {/* Brand logo with a spinning accent ring around it. */}
      <div className="relative flex h-24 w-24 items-center justify-center">
        <span
          className="absolute inset-0 animate-spin rounded-full border-2 border-transparent border-t-[var(--color-accent)]"
          style={{ animationDuration: '900ms' }}
        />
        <img
          src="/favicon.svg"
          alt="TionSwarm"
          className="h-12 w-12"
          style={{ animation: 'tionswarm-splash-pulse 1600ms ease-in-out infinite' }}
        />
      </div>

      <div className="flex flex-col items-center gap-1">
        <span className="text-lg font-semibold tracking-wide">TionSwarm</span>
        <span className="text-xs text-[var(--color-text-dim)]">Yükleniyor…</span>
      </div>

      {/* Keyframes are scoped here so the splash stays fully self-contained (no
          dependency on global CSS being loaded yet on a cold first paint). */}
      <style>{`
        @keyframes tionswarm-splash-fade {
          from { opacity: 0; }
          to { opacity: 1; }
        }
        @keyframes tionswarm-splash-pulse {
          0%, 100% { transform: scale(1); opacity: 1; }
          50% { transform: scale(1.08); opacity: 0.85; }
        }
      `}</style>
    </div>
  )
}
