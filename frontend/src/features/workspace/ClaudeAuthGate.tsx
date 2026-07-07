// ClaudeAuthGate is the post-workspace-create claude-cli readiness gate. When a
// new workspace is created its claude-home starts empty, so an agent turn driven
// by the claude-cli provider would hit an auth wall. This component runs a
// one-shot pre-flight probe against the (now active) new workspace and reacts:
//
//   - CLI present but this workspace is not logged in → a dismissible notification
//     offers to open the "claude-cli kimlik doğrulama" popup (ClaudeAuthDialog),
//     which writes a credential straight into this workspace's claude-home.
//   - CLI missing entirely → nothing to authenticate; steer the user to the
//     Providers screen (via onNavigateProviders) to set up a provider/credential.
//   - Already logged in → no-op.
//
// It is driven by a `trigger` counter that the parent bumps once per successful
// create, so the same mount handles every workspace the user spins up.
import { useEffect, useState } from 'react'
import { KeyRound, X } from 'lucide-react'
import { api } from '@/api'
import type { AppSettings } from '@/types'
import { ClaudeAuthDialog } from '@/features/settings/ClaudeAuthDialog'

interface Props {
  // Bumped by the parent after each successful workspace creation. A value <= 0
  // means "no create yet" and is ignored so a fresh mount does not probe.
  trigger: number
  // Called when the CLI is not installed, so the app can open the Providers
  // ("Sağlayıcılar") screen where a provider/credential can be configured.
  onNavigateProviders: () => void
  onError?: (msg: string) => void
}

export function ClaudeAuthGate({ trigger, onNavigateProviders, onError }: Props) {
  // Non-null while the "login needed" notification is showing; carries the probe
  // failure reason for the tooltip.
  const [notice, setNotice] = useState<{ detail?: string } | null>(null)
  // Non-null while the auth popup is open; holds the settings snapshot the dialog
  // needs (config dir + current credential kind/state). Fetched lazily on open.
  const [dialogSettings, setDialogSettings] = useState<AppSettings | null>(null)

  useEffect(() => {
    if (trigger <= 0) return
    let alive = true
    // Clear any leftover UI from a previous workspace's gate before re-probing.
    setNotice(null)
    setDialogSettings(null)
    ;(async () => {
      try {
        const r = await api.checkWorkspaceClaudeAuth()
        if (!alive) return
        if (r.loggedIn) return // ready — nothing to prompt
        if (!r.installed) {
          onNavigateProviders() // no CLI to authenticate → set up a provider
          return
        }
        setNotice({ detail: r.detail }) // CLI present but this workspace has no login
      } catch (e) {
        // A probe failure (network / backend) should not block workspace use;
        // surface it quietly and let the user authenticate from Settings later.
        onError?.((e as Error).message)
      }
    })()
    return () => {
      alive = false
    }
    // Intentionally keyed only on `trigger`: the callbacks are stable enough and
    // re-running on their identity would double-probe.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [trigger])

  // Fetch the current app settings, then open the auth popup with them.
  const openDialog = async () => {
    try {
      const s = await api.getSettings()
      setDialogSettings(s)
      setNotice(null)
    } catch (e) {
      onError?.((e as Error).message)
    }
  }

  return (
    <>
      {notice && !dialogSettings && (
        <div className="pointer-events-none fixed bottom-4 right-4 z-50 flex justify-end">
          <div
            role="alert"
            data-testid="claude-auth-gate-notice"
            className="pointer-events-auto flex max-w-md items-start gap-2 rounded-lg border border-[color-mix(in_srgb,var(--color-warning)_45%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_14%,var(--color-surface))] px-3 py-2.5 text-sm shadow-[var(--shadow-md)]"
          >
            <KeyRound size={16} className="mt-0.5 shrink-0 text-[var(--color-warning)]" />
            <div className="min-w-0 flex-1">
              <p className="font-medium text-[var(--color-text)]">
                Yeni workspace için claude-cli girişi gerekli
              </p>
              <p className="mt-0.5 break-words text-xs text-[var(--color-text-dim)]" title={notice.detail}>
                Bu workspace'in claude-home'u henüz yetkilendirilmedi. Kimlik doğrulamadan
                claude-cli turları çalışmaz.
              </p>
              <button
                onClick={openDialog}
                data-testid="claude-auth-gate-open"
                className="mt-2 inline-flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1.5 text-xs font-medium hover:border-[var(--color-accent)]"
              >
                <KeyRound size={12} /> Kimlik doğrula
              </button>
            </div>
            <button
              onClick={() => setNotice(null)}
              title="Kapat"
              className="shrink-0 rounded p-0.5 text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
            >
              <X size={14} />
            </button>
          </div>
        </div>
      )}

      {dialogSettings && (
        <ClaudeAuthDialog
          configDir={dialogSettings.claudeConfigDir}
          currentKind={dialogSettings.claudeCliAuthKind}
          isSet={dialogSettings.claudeCliAuthSet}
          onClose={() => setDialogSettings(null)}
          // The saved credential is written server-side; drop the snapshot so the
          // dialog unmounts. A follow-up create re-probes fresh.
          onSaved={() => setDialogSettings(null)}
        />
      )}
    </>
  )
}
