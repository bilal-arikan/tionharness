import { describe, expect, it } from 'vitest'
import { resolveBootScreen } from './bootScreen'

const base = {
  hadSetupAtBoot: true,
  wsLoading: false,
  minSplashElapsed: true,
  loadError: null as string | null,
  workspaceCount: 0,
}

describe('resolveBootScreen', () => {
  it('shows the connection error, not onboarding, when the workspace list request failed', () => {
    const failed = { ...base, loadError: 'Backend unavailable (502)' }
    expect(resolveBootScreen(failed)).toBe('unreachable')
    // Same for a browser that has never completed setup: a 502 is not a fresh install.
    expect(resolveBootScreen({ ...failed, hadSetupAtBoot: false })).toBe('unreachable')
  })

  it('shows onboarding only when the server answered with an empty list', () => {
    expect(resolveBootScreen(base)).toBe('onboarding')
    expect(resolveBootScreen({ ...base, wsLoading: true })).toBe('app')
  })

  it('holds the splash on a fresh install until the list settles and the minimum elapses', () => {
    const fresh = { ...base, hadSetupAtBoot: false }
    expect(resolveBootScreen({ ...fresh, wsLoading: true })).toBe('splash')
    expect(resolveBootScreen({ ...fresh, minSplashElapsed: false })).toBe('splash')
    expect(
      resolveBootScreen({ ...fresh, minSplashElapsed: false, loadError: 'network error' }),
    ).toBe('splash')
    expect(resolveBootScreen(fresh)).toBe('onboarding')
  })

  it('opens the app once workspaces exist', () => {
    expect(resolveBootScreen({ ...base, workspaceCount: 2 })).toBe('app')
  })
})
