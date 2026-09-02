// PhaseActions — canvas edits of a trajectory's declared plan (Rota F5): move
// the selected phase (active / done / skipped), add a phase, finish the run.
// Every call echoes the revision the screen rendered; a stale revision comes
// back as 409 and the stream re-reads the graph. A gate that does not hold
// comes back as 422 and can be forced after confirmation; a human gate opens a
// card on the root chat (202).
import { useState } from 'react'
import { Flag, Loader2, Plus } from 'lucide-react'
import { api } from '@/api'
import { toast } from '@/shared/components'
import type { Trajectory } from '@/types/trajectory'
import { phaseId } from './trajectoryLayout'

interface Props {
  trajectory: Trajectory
  // The phase the user picked on the canvas (bare id), or null.
  phase: string | null
}

type ApiErr = Error & { status?: number }

export function PhaseActions({ trajectory: t, phase }: Props) {
  const [busy, setBusy] = useState(false)
  const [adding, setAdding] = useState(false)
  const [newId, setNewId] = useState('')
  const [newProfile, setNewProfile] = useState('')
  const terminal = t.status === 'done' || t.status === 'failed' || t.status === 'abandoned'
  const node = phase
    ? t.nodes.find((n) => n.kind === 'phase' && phaseId(n.id) === phase)
    : undefined

  const move = async (state: 'active' | 'done' | 'skipped', force = false) => {
    if (!phase) return
    setBusy(true)
    try {
      const res = await api.setTrajectoryPhase(t.id, {
        id: phase,
        state,
        expectedRev: t.revision,
        force,
      })
      if ('pending' in res && res.pending) {
        toast.info(`Faz ${phase}: insan kapısı açıldı — kök sohbetteki kartı yanıtla`)
      } else {
        toast.success(`Faz ${phase} → ${state}`)
      }
    } catch (e) {
      const err = e as ApiErr
      if (err.status === 422 && !force) {
        if (confirm(`Kapı geçilmedi:\n${err.message}\n\nZorla geç?`)) {
          await move(state, true)
          return
        }
      } else if (err.status === 409) {
        toast.info('Rota bu arada değişti; ekran yenilendi, tekrar dene')
      } else {
        toast.error(err.message)
      }
    } finally {
      setBusy(false)
    }
  }

  const addPhase = async () => {
    const id = newId.trim().toLowerCase()
    if (!id) return
    setBusy(true)
    try {
      const phases = t.nodes
        .filter((n) => n.kind === 'phase')
        .map((n) => ({
          id: phaseId(n.id),
          label: n.label,
          profile: n.profile,
          optional: n.optional,
          gate: n.gate ?? undefined,
        }))
      phases.push({
        id,
        label: undefined,
        profile: newProfile.trim() || undefined,
        optional: false,
        gate: undefined,
      })
      await api.planTrajectory(t.id, { phases, expectedRev: t.revision })
      toast.success(`Faz eklendi: ${id}`)
      setNewId('')
      setNewProfile('')
      setAdding(false)
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const finish = async (status: 'done' | 'failed') => {
    if (!confirm(`Rota ${status === 'done' ? 'tamamlandı' : 'başarısız'} olarak kapatılsın mı?`))
      return
    setBusy(true)
    try {
      await api.finishTrajectory(t.id, { status, expectedRev: t.revision })
      toast.success('Rota kapatıldı')
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const btn =
    'flex items-center gap-1 rounded border border-[var(--color-border)] px-2 py-0.5 text-[var(--color-text-dim)] hover:text-[var(--color-accent)] disabled:opacity-40'
  return (
    <div
      className="flex flex-wrap items-center gap-1.5 border-b border-[var(--color-border)] bg-[var(--color-surface-2)]/40 px-3 py-1 text-[11px]"
      data-testid="phase-actions"
    >
      {busy && <Loader2 size={12} className="animate-spin" />}
      {phase && node ? (
        <>
          <span className="font-medium">faz {phase}</span>
          <span className="text-[var(--color-text-dim)]">({node.state})</span>
          {node.state !== 'active' && !terminal && (
            <button type="button" className={btn} disabled={busy} onClick={() => move('active')}>
              ● aktif yap
            </button>
          )}
          {node.state !== 'done' && !terminal && (
            <button
              type="button"
              className={btn}
              disabled={busy}
              onClick={() => move('done')}
              title={node.gate ? `Kapı: ${node.gate.kind} ${node.gate.value ?? ''}` : undefined}
            >
              ✓ tamamlandı{node.gate ? ' (kapı)' : ''}
            </button>
          )}
          {node.state !== 'skipped' && node.state !== 'done' && !terminal && (
            <button type="button" className={btn} disabled={busy} onClick={() => move('skipped')}>
              ↷ atla
            </button>
          )}
        </>
      ) : (
        <span className="text-[var(--color-text-dim)]">
          Bir faz sütununa tıkla: aktif yap / tamamlandı / atla
        </span>
      )}
      {!terminal && (
        <span className="ml-auto flex items-center gap-1.5">
          {adding ? (
            <>
              <input
                value={newId}
                onChange={(e) => setNewId(e.target.value)}
                placeholder="faz id (ör. docs)"
                className="w-28 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-1.5 py-0.5 font-mono text-[var(--color-text)]"
              />
              <input
                value={newProfile}
                onChange={(e) => setNewProfile(e.target.value)}
                placeholder="profil (opsiyonel)"
                className="w-28 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-1.5 py-0.5 text-[var(--color-text)]"
              />
              <button
                type="button"
                className={btn}
                disabled={busy || !newId.trim()}
                onClick={addPhase}
              >
                ekle
              </button>
              <button type="button" className={btn} onClick={() => setAdding(false)}>
                vazgeç
              </button>
            </>
          ) : (
            <button
              type="button"
              className={btn}
              onClick={() => setAdding(true)}
              title="Plana faz ekle"
            >
              <Plus size={11} /> faz ekle
            </button>
          )}
          <button
            type="button"
            className={btn}
            disabled={busy}
            onClick={() => finish('done')}
            title="Rotayı tamamlandı olarak kapat"
          >
            <Flag size={11} /> bitir
          </button>
        </span>
      )}
    </div>
  )
}
