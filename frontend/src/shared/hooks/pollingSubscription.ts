// Schedule polling without an initial fetch. Callers own their fetch state and
// callback identity; this helper owns the timer and visibility subscription.
export function createPollingSubscription(
  tick: () => void,
  intervalMs: number,
  pauseWhenHidden = true,
): () => void {
  if (!pauseWhenHidden) {
    const timer = setInterval(tick, intervalMs)
    return () => clearInterval(timer)
  }

  let timer: ReturnType<typeof setInterval> | null = null
  const stop = () => {
    if (timer !== null) {
      clearInterval(timer)
      timer = null
    }
  }
  const start = () => {
    if (timer === null) timer = setInterval(tick, intervalMs)
  }
  const onVisibility = () => {
    if (document.visibilityState === 'hidden') {
      stop()
      return
    }
    tick()
    start()
  }
  if (document.visibilityState !== 'hidden') start()
  document.addEventListener('visibilitychange', onVisibility)
  return () => {
    stop()
    document.removeEventListener('visibilitychange', onVisibility)
  }
}
