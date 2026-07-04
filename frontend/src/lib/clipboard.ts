// copyText writes `text` to the clipboard, working in BOTH secure and insecure
// contexts. The async Clipboard API (navigator.clipboard) is only exposed on
// secure origins (HTTPS or localhost); when the app is served over a plain-HTTP
// LAN IP (e.g. http://192.168.1.4:5173) it is `undefined`, so a direct
// `navigator.clipboard.writeText` silently no-ops. This helper falls back to the
// legacy `document.execCommand('copy')` via a hidden textarea in that case.
//
// Returns true on success, false when neither path could copy (caller decides how
// to surface the failure).
export async function copyText(text: string): Promise<boolean> {
  if (!text) return false

  // Preferred path: async Clipboard API (secure contexts).
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // Permission denied or unavailable — fall through to the legacy path.
    }
  }

  // Legacy fallback for insecure contexts (LAN IP over HTTP). execCommand is
  // deprecated but remains the only synchronous copy available without a secure
  // origin. It requires the source node to be in the document and selected.
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    // Keep it off-screen but focusable; readOnly avoids the mobile keyboard.
    ta.setAttribute('readonly', '')
    ta.style.position = 'fixed'
    ta.style.top = '-9999px'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    ta.setSelectionRange(0, ta.value.length)
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch {
    return false
  }
}

// copyToClipboard is the single, app-wide entry point for "copy this text".
// Every copy button should call this instead of touching navigator.clipboard
// directly. It attempts a real programmatic copy (copyText) and, when that is
// impossible — some Chrome builds block BOTH the Clipboard API and
// execCommand('copy') on insecure origins like a plain-HTTP LAN IP — falls back
// to a window.prompt pre-filled with the text so the user can still copy it
// manually (Ctrl+C, Enter).
//
// Returns true ONLY when the programmatic copy succeeded, so callers can show a
// "Kopyalandı" confirmation on true and stay silent on false (the prompt has
// already given the user the text).
export async function copyToClipboard(
  text: string,
  promptLabel = 'Kopyalayın (Ctrl+C, Enter):',
): Promise<boolean> {
  if (!text) return false
  const ok = await copyText(text)
  if (!ok) window.prompt(promptLabel, text)
  return ok
}
