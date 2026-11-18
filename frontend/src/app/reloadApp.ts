// reloadApp restarts the client from a clean slate. Used when the backend comes
// back after the boot requests failed: every hook already fired (and failed)
// against the dead server, so a full reload is the one reliable way to bring
// sessions, settings and live streams up together. Its own module so tests can
// replace it.
export function reloadApp(): void {
  window.location.reload()
}
