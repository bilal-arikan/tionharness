import { Loader2, Check, Pencil, Target, CheckCircle2, Circle } from 'lucide-react'
import type { SessionInfo } from '@/types'
import { PromptEditor } from '@/shared/components'

interface Props {
  info: SessionInfo
  editingGoal: boolean
  goalDraft: string
  savingGoal: boolean
  setGoalDraft: (v: string) => void
  setEditingGoal: (v: boolean) => void
  startEditGoal: () => void
  commitGoal: () => void
  toggleGoalDone: () => void
}

// Goal ("north star") — persistent objective injected into context
export function SessionGoalSection({
  info,
  editingGoal,
  goalDraft,
  savingGoal,
  setGoalDraft,
  setEditingGoal,
  startEditGoal,
  commitGoal,
  toggleGoalDone,
}: Props) {
  return (
    <section>
      <div className="mb-2 flex items-center justify-between">
        <div className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
          <Target size={12} className="shrink-0" />
          <span>Hedef</span>
        </div>
        {!editingGoal && (
          <button
            onClick={startEditGoal}
            title={info.goal ? 'Hedefi düzenle' : 'Hedef belirle'}
            className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
          >
            <Pencil size={13} />
          </button>
        )}
      </div>
      {editingGoal ? (
        <div className="flex flex-col gap-1.5">
          <PromptEditor
            autoFocus
            value={goalDraft}
            onChange={setGoalDraft}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) commitGoal()
              else if (e.key === 'Escape') setEditingGoal(false)
            }}
            disabled={savingGoal}
            rows={4}
            maxLength={2000}
            placeholder="Bu sohbet için kalıcı bir hedef yaz — ajan her turda buna göre ilerler. Örn: 'X özelliğini test ederek bitir ve PR aç.'"
            className="border-[var(--color-accent)]"
            textareaClassName="text-xs"
          />
          <div className="flex items-center gap-2">
            <button
              onClick={commitGoal}
              disabled={savingGoal}
              className="flex items-center gap-1.5 rounded-lg bg-[var(--color-accent)] px-2.5 py-1.5 text-[11px] font-medium text-white transition hover:opacity-90 disabled:opacity-40"
            >
              {savingGoal ? <Loader2 size={13} className="animate-spin" /> : <Check size={13} />}
              Kaydet
            </button>
            <button
              onClick={() => setEditingGoal(false)}
              disabled={savingGoal}
              className="rounded-lg border border-[var(--color-border)] px-2.5 py-1.5 text-[11px] text-[var(--color-text-dim)] transition hover:text-[var(--color-text)] disabled:opacity-40"
            >
              İptal
            </button>
            <span className="ml-auto text-[10px] text-[var(--color-text-dim)]">⌘/Ctrl+Enter</span>
          </div>
        </div>
      ) : info.goal ? (
        <div className="flex flex-col gap-1.5">
          <p
            className={`whitespace-pre-wrap rounded-lg border px-2.5 py-2 text-xs leading-relaxed ${
              info.goalDone
                ? 'border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-success)_8%,transparent)] text-[var(--color-text-dim)] line-through decoration-[var(--color-text-dim)]/60'
                : 'border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-accent)_8%,transparent)] text-[var(--color-text)]'
            }`}
          >
            {info.goal}
          </p>
          <div className="flex items-center justify-between">
            <button
              onClick={toggleGoalDone}
              disabled={savingGoal}
              title={info.goalDone ? 'Hedefi yeniden aç (context\'e tekrar enjekte edilir)' : 'Tamamlandı olarak işaretle (context enjeksiyonu durur)'}
              className={`flex items-center gap-1.5 rounded-lg border px-2 py-1 text-[11px] transition disabled:opacity-40 ${
                info.goalDone
                  ? 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
                  : 'border-[var(--color-success)]/40 text-[var(--color-success)] hover:bg-[var(--color-success)]/10'
              }`}
            >
              {savingGoal ? (
                <Loader2 size={13} className="animate-spin" />
              ) : info.goalDone ? (
                <Circle size={13} />
              ) : (
                <CheckCircle2 size={13} />
              )}
              {info.goalDone ? 'Yeniden aç' : 'Tamamlandı'}
            </button>
            {info.goalDone && (
              <span className="flex items-center gap-1 text-[10px] font-medium text-[var(--color-success)]">
                <CheckCircle2 size={12} /> Tamamlandı · enjekte edilmiyor
              </span>
            )}
          </div>
        </div>
      ) : (
        <button
          onClick={startEditGoal}
          className="flex w-full items-center gap-2 rounded-lg border border-dashed border-[var(--color-border)] px-2.5 py-2 text-left text-[11px] text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
        >
          <Target size={13} className="shrink-0" />
          Bu sohbet için bir hedef belirle
        </button>
      )}
    </section>
  )
}
