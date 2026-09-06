// GoalIntake — the ONLY way a goal enters the store: the user writes the goal
// in their own words, the goal-writer agent maps it onto the metric catalog and
// saves a DRAFT. With `goal` set, the same dialog rewrites an existing goal
// (its id, status and history are kept; an active goal drops back to draft).
import { useState } from 'react'
import { Loader2, Sparkles, X } from 'lucide-react'
import { api } from '@/api'
import { Button, ModalOverlay } from '@/shared/components'
import type { Goal } from '@/types/goal'

interface Props {
  goal?: Goal | null
  onClose: () => void
  onWritten: (goal: Goal, created: boolean) => void
  onError: (msg: string) => void
}

const EXAMPLES = [
  'Kod inceleme reçetesi daha ucuza çıksın ama başarı oranı %90 altına düşmesin.',
  'Kanban kartları daha hızlı kapansın; günde en az 3 kart bitsin.',
  'Ajanlar bana daha az soru sorsun, kalite düşmesin.',
  'Türkçe doküman çıktıları daha tutarlı ve okunur olsun.',
]

export function GoalIntake({ goal, onClose, onWritten, onError }: Props) {
  const [text, setText] = useState(goal?.rawText ?? '')
  const [busy, setBusy] = useState(false)
  const rewrite = !!goal

  const submit = async () => {
    if (text.trim() === '' || busy) return
    setBusy(true)
    try {
      const res = await api.intakeGoal(text, goal?.id)
      onWritten(res.goal, res.created)
    } catch (e) {
      onError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <ModalOverlay onClose={busy ? () => {} : onClose} closeOnEscape={!busy}>
      <div
        className="flex max-h-[85vh] w-[min(640px,92vw)] flex-col rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl"
        data-testid="goal-intake"
      >
        <div className="flex items-center gap-2 border-b border-[var(--color-border)] px-4 py-3">
          <Sparkles size={16} className="opacity-70" />
          <span className="text-sm font-semibold">
            {rewrite ? `Hedefi yeniden yaz · ${goal.id}` : 'Yeni hedef'}
          </span>
          <button
            type="button"
            onClick={onClose}
            disabled={busy}
            className="ml-auto rounded p-1 text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
            aria-label="Kapat"
          >
            <X size={16} />
          </button>
        </div>
        <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-4 py-3">
          <p className="text-sm text-[var(--color-text-dim)]">
            Hedefi <strong>kendi sözlerinle</strong> yaz. Hedef yazıcı ajan onu ölçülebilir bir
            metriğe, guardrail'lere ve kapsama oturtup <strong>taslak</strong> olarak kaydeder; sen
            gözden geçirip düzenler, sonra etkinleştirirsin. Sözlerin olduğu gibi saklanır.
          </p>
          <textarea
            autoFocus
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') void submit()
            }}
            rows={6}
            maxLength={4000}
            disabled={busy}
            placeholder="Örn. kod inceleme reçetesi daha az tur harcasın ama hata kaçırmasın…"
            data-testid="goal-intake-text"
            className="w-full resize-y rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none focus:border-[var(--color-accent)]"
          />
          {!rewrite && (
            <div className="flex flex-wrap gap-1.5">
              {EXAMPLES.map((ex) => (
                <button
                  key={ex}
                  type="button"
                  disabled={busy}
                  onClick={() => setText(ex)}
                  className="rounded-full border border-[var(--color-border)] px-2.5 py-1 text-left text-xs text-[var(--color-text-dim)] hover:border-[var(--color-accent)] hover:text-[var(--color-text)]"
                >
                  {ex}
                </button>
              ))}
            </div>
          )}
          {rewrite && (
            <p className="text-xs text-[var(--color-text-dim)]">
              Yeniden yazım kimliği ve geçmişi korur; etkin bir hedef onay için taslağa döner.
            </p>
          )}
        </div>
        <div className="flex items-center justify-end gap-2 border-t border-[var(--color-border)] px-4 py-3">
          <span className="mr-auto text-xs text-[var(--color-text-dim)]">
            {busy ? 'Hedef yazıcı çalışıyor…' : 'Ctrl+Enter ile gönder'}
          </span>
          <Button variant="secondary" size="md" onClick={onClose} disabled={busy}>
            Vazgeç
          </Button>
          <Button
            size="md"
            onClick={() => void submit()}
            disabled={busy || text.trim() === ''}
            data-testid="goal-intake-submit"
          >
            {busy ? <Loader2 size={14} className="animate-spin" /> : <Sparkles size={14} />}
            {rewrite ? 'Yeniden yaz' : 'Taslağı yazdır'}
          </Button>
        </div>
      </div>
    </ModalOverlay>
  )
}
