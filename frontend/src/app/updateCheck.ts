// Pure logic behind the "a newer version is available" banner.
//
// Dismissal is stored PER VERSION rather than as a single "hidden" flag: hiding
// 0.2.0 must not also hide 0.3.0 six weeks later, or the user would silently
// stop hearing about releases after dismissing once.
import type { UpdateStatus } from '@/types'

export const UPDATE_DISMISS_KEY = 'tionharness.updateDismissedVersion'

// readDismissedVersion returns the version the user last dismissed, or '' when
// none was (or storage is unavailable, e.g. private mode).
export function readDismissedVersion(key: string = UPDATE_DISMISS_KEY): string {
  try {
    return localStorage.getItem(key) ?? ''
  } catch {
    return ''
  }
}

export function writeDismissedVersion(version: string, key: string = UPDATE_DISMISS_KEY): void {
  try {
    localStorage.setItem(key, version)
  } catch {
    // Quota / private-mode write failure: the banner still hides for this
    // session, it just reappears after a reload. Not worth failing the app over.
  }
}

// shouldShowUpdate decides whether the banner renders. Everything other than a
// successful check that found a NEWER version and was not dismissed is a "no" —
// a 'skipped' (dev build) or 'unknown' (feed unreachable) result must stay
// invisible rather than showing a degraded banner.
export function shouldShowUpdate(
  status: UpdateStatus | null,
  dismissedVersion: string,
): status is UpdateStatus {
  if (!status) return false
  if (status.state !== 'ok') return false
  if (!status.updateAvailable) return false
  if (!status.latest) return false
  return status.latest !== dismissedVersion
}
