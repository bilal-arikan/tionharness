import { BTN_DANGER, BTN_PRIMARY, BTN_SECONDARY, BTN_WARNING } from './buttonStyles'

interface Props {
  // Turn lifecycle: `streaming` = a turn is in flight; `waiting` = the turn ended
  // into a pending self-wake that will auto-resume. Both give the busy look.
  streaming: boolean
  waiting: boolean
  // Input state used to pick which action(s) are offered / enabled.
  hasText: boolean
  hasContent: boolean
  anyUploading: boolean
  disabled: boolean
  // Actions.
  onSend: () => void
  onStop?: () => void
  onCancelWait?: () => void
  onQueue: () => void
  onInterrupt: () => void
  onSteer: () => void
}

// SendActions is the composer's right-hand action cluster. It picks exactly one
// button (or the streaming triplet) from the turn lifecycle + input state:
//   idle            → Gönder
//   waiting, empty  → Durdur (cancel the pending self-wake)
//   waiting, typed  → Gönder (takes over: disarms the wake, starts a fresh turn)
//   streaming, empty→ Durdur (stop generation)
//   streaming, typed→ Sıraya / Kes / Yönlendir
export function SendActions({
  streaming,
  waiting,
  hasText,
  hasContent,
  anyUploading,
  disabled,
  onSend,
  onStop,
  onCancelWait,
  onQueue,
  onInterrupt,
  onSteer,
}: Props) {
  const sendBtn = (
    <button
      onClick={onSend}
      disabled={disabled || !hasContent || anyUploading}
      data-testid="composer-send"
      aria-label="Gönder"
      className={BTN_PRIMARY}
    >
      Gönder
    </button>
  )

  if (waiting && !streaming) {
    return hasText ? (
      sendBtn
    ) : (
      <button
        onClick={onCancelWait}
        title="Otomatik uyandırmayı durdur"
        data-testid="composer-stop"
        aria-label="Durdur"
        className={BTN_DANGER}
      >
        Durdur
      </button>
    )
  }

  if (!streaming) return sendBtn

  if (hasText) {
    // Input filled while streaming → queue / interrupt / steer.
    return (
      <div className="flex items-end gap-1.5">
        <button onClick={onQueue} title="Bu tur bitince gönder" data-testid="composer-queue" className={BTN_SECONDARY}>
          Sıraya
        </button>
        <button onClick={onInterrupt} title="Turu kes ve hemen gönder" data-testid="composer-interrupt" className={BTN_WARNING}>
          Kes
        </button>
        <button
          onClick={onSteer}
          title="Çalışan turu canlı yönlendir (araç döngüsünde etkili)"
          data-testid="composer-steer"
          className={BTN_PRIMARY}
        >
          Yönlendir
        </button>
      </div>
    )
  }

  // Streaming, empty input → stop.
  return (
    <button onClick={onStop} title="Üretimi durdur" data-testid="composer-stop" aria-label="Durdur" className={BTN_DANGER}>
      Durdur
    </button>
  )
}
