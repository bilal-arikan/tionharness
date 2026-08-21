// Tears down the static pre-JS boot splash defined in index.html. Call once
// after the first React render has been requested, so the removal happens
// only after real app content is ready to take its place.
export function removeBootSplash(): void {
  const win = window as unknown as { __bootSplashTimeout?: number }
  const clear = () => {
    if (win.__bootSplashTimeout !== undefined) {
      window.clearTimeout(win.__bootSplashTimeout)
    }
    document.getElementById('boot-splash')?.remove()
  }
  // Two rAFs: the first queues after React's commit, the second after the
  // browser has actually painted that commit — one rAF alone can still race
  // ahead of the paint on some WebView2 builds, leaving a one-frame flash.
  requestAnimationFrame(() => requestAnimationFrame(clear))
}
