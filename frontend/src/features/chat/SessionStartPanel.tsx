// SessionStartPanel is the pre-first-message setup card of a fresh chat session.
//
// Coordinator mode and its recipe are per-SESSION settings that only matter
// BEFORE the first turn: the tool set is frozen into the prompt epoch when the
// session starts working, and switching mode mid-thread rewrites the tools a
// running conversation was built on. Until now both switches lived only in the
// session info panel, which a user has to know about and open. This card puts
// them where the decision is actually made — above the composer of an empty
// session — and disappears the moment a message is sent (ChatView gates it on
// the transcript being empty and no turn pending).
import { useCallback, useEffect, useState } from 'react'
import { Loader2, Users, X } from 'lucide-react'
import { api } from '@/api'
import { InfoPopover } from '@/shared/components/InfoPopover'
import {
  CoordinatorWorkflowPicker,
  WORKFLOW_HELP,
} from '@/shared/components/CoordinatorWorkflowPicker'

interface Props {
  sessionId: string
  onError: (msg: string) => void
  // Called after coordinator mode / workflow changed, so the shell re-fetches the
  // session (sidebar role chip, coordination panel, worker banner gating).
  onChanged?: () => void
  // Hides the card for this session without changing anything.
  onDismiss: () => void
  // Opens the Skills screen on a recipe's slug (the picker's 📖 link).
  onOpenSkill?: (slug: string) => void
}

export function SessionStartPanel({
  sessionId,
  onError,
  onChanged,
  onDismiss,
  onOpenSkill,
}: Props) {
  // Seeded from the server rather than assumed off: an agent configured as a
  // coordinator (Ajanlar ▸ Koordinatör) opens every new session already in
  // coordinator mode, recipe included (db.createSessionLocked).
  const [coordinator, setCoordinator] = useState(false)
  const [workflow, setWorkflow] = useState('')
  const [loading, setLoading] = useState(true)
  const [toggling, setToggling] = useState(false)
  const [savingWf, setSavingWf] = useState(false)

  useEffect(() => {
    let alive = true
    setLoading(true)
    api
      .sessionInfo(sessionId)
      .then((info) => {
        if (!alive) return
        setCoordinator(Boolean(info.coordinatorMode))
        setWorkflow(info.coordinatorWorkflow ?? '')
      })
      .catch((e) => {
        if (alive) onError((e as Error).message)
      })
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => {
      alive = false
    }
  }, [sessionId, onError])

  const toggleCoordinator = useCallback(async () => {
    const next = !coordinator
    setToggling(true)
    try {
      await api.setSessionRole(sessionId, next ? 'coordinator' : '')
      setCoordinator(next)
      // Turning the mode off drops the recipe's meaning with it — the backend
      // rejects a workflow on a non-coordinator session, so mirror that here
      // instead of showing a selection that no longer applies.
      if (!next) setWorkflow('')
      onChanged?.()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setToggling(false)
    }
  }, [coordinator, sessionId, onChanged, onError])

  const selectWorkflow = useCallback(
    async (slug: string) => {
      setSavingWf(true)
      try {
        await api.setSessionWorkflow(sessionId, slug)
        setWorkflow(slug)
        onChanged?.()
      } catch (e) {
        onError((e as Error).message)
      } finally {
        setSavingWf(false)
      }
    },
    [sessionId, onChanged, onError],
  )

  return (
    // Geometry is deliberately copied from the Composer: the same outer gutter
    // (px-1 / md:px-6) and the same card (rounded-2xl, px-3, pt-2.5/pb-2), so the
    // panel reads as one stacked block with the input rather than a floating
    // toast of a different width.
    <div data-testid="session-start-panel" className="px-1 pb-1.5 md:px-6 md:pb-2">
      <div className="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] px-3 pb-2 pt-2.5 shadow-lg">
        <div className="flex items-center gap-1.5">
          <span className="flex min-w-0 flex-1 items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
            <Users size={12} className="shrink-0" /> Başlangıç ayarları
          </span>
          {loading && <Loader2 size={11} className="animate-spin text-[var(--color-text-dim)]" />}
          <button
            type="button"
            onClick={onDismiss}
            title="Bu oturum için gizle"
            aria-label="Başlangıç panelini kapat"
            data-testid="session-start-dismiss"
            className="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-md text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
          >
            <X size={13} />
          </button>
        </div>

        <button
          type="button"
          onClick={toggleCoordinator}
          disabled={loading || toggling}
          aria-pressed={coordinator}
          data-testid="session-start-coordinator"
          className={`mt-1.5 flex w-full items-center gap-2 rounded-lg border px-2.5 py-2 text-left text-[11px] transition disabled:opacity-50 ${
            coordinator
              ? 'border-[var(--color-accent)] bg-[var(--color-accent)]/5 font-medium text-[var(--color-accent)]'
              : 'border-dashed border-[var(--color-border)] text-[var(--color-text-dim)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]'
          }`}
        >
          {toggling ? (
            <Loader2 size={13} className="shrink-0 animate-spin" />
          ) : (
            <Users size={13} className="shrink-0" />
          )}
          {coordinator
            ? "Koordinatör modu açık — bu oturum paralel worker'ları yönetir"
            : "Koordinatör modunu aç (paralel worker'ları yönet)"}
          <span className="ml-auto shrink-0 text-[10px] text-[var(--color-text-dim)]">
            {coordinator ? 'Kapat' : 'Aç'}
          </span>
        </button>

        {/* The recipe only exists for a coordinator; the backend refuses it otherwise. */}
        {coordinator && (
          <div className="mt-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-2">
            <div className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
              <span>Workflow</span>
              <InfoPopover text={WORKFLOW_HELP} label="Workflow nedir?" fixed />
              {savingWf && <Loader2 size={11} className="animate-spin" />}
            </div>
            <div className="mt-1">
              <CoordinatorWorkflowPicker
                value={workflow}
                onChange={selectWorkflow}
                disabled={savingWf}
                groupName={`start-wf-${sessionId}`}
                onOpenSkill={onOpenSkill}
              />
            </div>
          </div>
        )}

        <p className="mt-1.5 px-0.5 text-[10px] leading-relaxed text-[var(--color-text-dim)]">
          İlk mesajı gönderdiğinde bu panel kapanır. Ayarlar yalnız bu oturuma aittir ve sonradan
          oturum panelindeki <strong>Koordinasyon</strong> bölümünden değiştirilebilir.
        </p>
      </div>
    </div>
  )
}
