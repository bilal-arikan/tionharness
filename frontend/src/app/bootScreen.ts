// Which full-screen state the shell shows before (or instead of) the app.
export type BootScreen = 'splash' | 'unreachable' | 'onboarding' | 'app'

interface BootState {
  // A workspace has existed on this device before (returning user).
  hadSetupAtBoot: boolean
  // The first workspace-list request has not settled yet.
  wsLoading: boolean
  minSplashElapsed: boolean
  // Message of the failed workspace-list request, or null when it succeeded.
  loadError: string | null
  workspaceCount: number
}

// resolveBootScreen decides the first-run gating. The important distinction is
// "the request failed" versus "the server answered with zero workspaces": only
// the latter is a fresh install. A failed request must never fall through to
// onboarding — that screen reads as if the user's workspaces were gone.
export function resolveBootScreen({
  hadSetupAtBoot,
  wsLoading,
  minSplashElapsed,
  loadError,
  workspaceCount,
}: BootState): BootScreen {
  // Fresh install → splash while the list is loading AND until the minimum
  // duration elapses. Returning users skip it.
  if (!hadSetupAtBoot && (wsLoading || !minSplashElapsed)) return 'splash'
  if (loadError !== null && workspaceCount === 0) return 'unreachable'
  // List resolved and there are zero workspaces → onboarding.
  if (!wsLoading && workspaceCount === 0) return 'onboarding'
  return 'app'
}
