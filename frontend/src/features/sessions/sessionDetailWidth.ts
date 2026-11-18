// Width settings of the session detail panel, shared by the panel itself and by
// the placeholder shown while its lazy chunk loads, so the placeholder occupies
// exactly the frame the panel will fill.
export const SESSION_DETAIL_WIDTH_KEY = 'tionharness.sessionInfoWidth'
export const SESSION_DETAIL_DEFAULT_WIDTH = 320
export const SESSION_DETAIL_MIN_WIDTH = 280
export const SESSION_DETAIL_MAX_WIDTH = 640

// storedSessionDetailWidth returns the user's persisted panel width, or the
// default when nothing valid is stored (or storage is blocked).
export function storedSessionDetailWidth(): number {
  try {
    const saved = Number(globalThis.localStorage?.getItem(SESSION_DETAIL_WIDTH_KEY))
    return saved >= SESSION_DETAIL_MIN_WIDTH && saved <= SESSION_DETAIL_MAX_WIDTH
      ? saved
      : SESSION_DETAIL_DEFAULT_WIDTH
  } catch {
    return SESSION_DETAIL_DEFAULT_WIDTH
  }
}
