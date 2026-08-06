// RewindDialog is the "/rewind" checkpoint picker. It lists the session's user
// prompts (each prompt is a checkpoint); choosing one rewinds the conversation
// back to that point — the chosen prompt and every message after it are removed.
// This is a CONVERSATION-ONLY rewind: file changes from past turns are NOT
// reverted (git remains the source of truth for code). The removed prompt text is
// handed back to the caller so it can be dropped into the composer for a re-try.
import { useMemo, useState } from 'react'
import { RotateCcw } from 'lucide-react'
import type { Message } from '@/types'
import { Button, ModalOverlay } from '@/shared/components'

interface Props {
  messages: Message[]
  // Rewind to the given message (remove it + everything after). Returns the
  // removed prompt text, which the caller restores into the composer.
  onRewind: (messageId: string) => Promise<string>
  onClose: () => void
}

// clip trims a prompt preview to a single readable line.
function clip(s: string, n = 140): string {
  const oneLine = s.replace(/\s+/g, ' ').trim()
  return oneLine.length > n ? oneLine.slice(0, n) + '…' : oneLine
}

export function RewindDialog({ messages, onRewind, onClose }: Props) {
  const [selected, setSelected] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  // Checkpoints = user prompts, newest first. Each carries how many messages
  // (itself + everything after) would be removed by rewinding to it.
  const checkpoints = useMemo(() => {
    const out: { id: string; text: string; removed: number; n: number }[] = []
    let userSeen = 0
    for (let i = 0; i < messages.length; i++) {
      if (messages[i].role !== 'user') continue
      userSeen++
      out.push({
        id: messages[i].id,
        text: messages[i].text,
        removed: messages.length - i,
        n: userSeen,
      })
    }
    return out.reverse()
  }, [messages])

  const confirm = async () => {
    if (!selected || busy) return
    setBusy(true)
    try {
      await onRewind(selected)
    } finally {
      setBusy(false)
    }
  }

  return (
    <ModalOverlay onClose={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Sohbeti geri sar"
        data-testid="rewind-modal"
        className="flex max-h-[80vh] w-full max-w-xl flex-col rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 shadow-[var(--shadow-lg)]"
      >
        <h2 className="mb-1 flex items-center gap-2 text-base font-semibold">
          <RotateCcw size={16} /> Sohbeti geri sar
        </h2>
        <p className="mb-4 text-xs text-[var(--color-text-dim)]">
          Bir prompt seç — o prompt ve sonrasındaki tüm mesajlar silinir, sohbet o checkpoint'e
          döner. <b>Dosya değişiklikleri geri alınmaz</b> (kod için git kullan). Silinen prompt,
          düzenleyip yeniden göndermen için mesaj kutusuna geri konur.
        </p>

        {checkpoints.length === 0 ? (
          <div className="rounded-lg border border-[var(--color-border)] p-4 text-sm text-[var(--color-text-dim)]">
            Bu oturumda geri sarılacak bir kullanıcı mesajı yok.
          </div>
        ) : (
          <div className="min-h-0 flex-1 space-y-1.5 overflow-y-auto pr-1">
            {checkpoints.map((c) => (
              <button
                key={c.id}
                type="button"
                onClick={() => setSelected(c.id)}
                className={`w-full rounded-lg border px-3 py-2 text-left transition ${
                  selected === c.id
                    ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)]'
                    : 'border-[var(--color-border)] hover:bg-[var(--color-surface-2)]'
                }`}
              >
                <div className="mb-0.5 flex items-center justify-between gap-2">
                  <span className="text-xs font-medium text-[var(--color-text-dim)]">
                    Prompt #{c.n}
                  </span>
                  <span className="text-xs text-[var(--color-text-dim)]">
                    {c.removed} mesaj silinir
                  </span>
                </div>
                <div className="text-sm">{clip(c.text) || <i>(boş mesaj)</i>}</div>
              </button>
            ))}
          </div>
        )}

        <div className="mt-4 flex items-center justify-end gap-2">
          <Button variant="secondary" onClick={onClose} disabled={busy}>
            İptal
          </Button>
          <Button variant="danger" onClick={confirm} disabled={!selected || busy}>
            {busy ? 'Geri sarılıyor…' : 'Geri sar'}
          </Button>
        </div>
      </div>
    </ModalOverlay>
  )
}
