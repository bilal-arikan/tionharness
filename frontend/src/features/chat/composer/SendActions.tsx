import { ArrowUp, Compass, ListPlus, Scissors, Square } from 'lucide-react'
import { ActionButton } from './ActionButton'
import { BTN_DANGER, BTN_PRIMARY, BTN_QUEUE, BTN_WARNING } from './buttonStyles'

interface Props {
  // Turn lifecycle: `streaming` = a turn is in flight; `waiting` = the turn ended
  // into a pending self-wake that will auto-resume. Both give the busy look.
  streaming: boolean
  waiting: boolean
  // workersActive: a coordinator session has background workers running while its
  // own turn is NOT streaming (idle between turns). A message sent now is held in
  // the tray and dispatched once the workers drain (backend runInboxWorker), so the
  // send button is presented as a queue action to make that wait explicit.
  workersActive?: boolean
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
//
// Every button is an ActionButton: icon + label on wide viewports, icon-only below
// the `sm` breakpoint. That matters most for the streaming triplet, which would
// otherwise be three labelled buttons competing for a narrow toolbar row.
export function SendActions({
  streaming,
  waiting,
  workersActive = false,
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
    <ActionButton
      onClick={onSend}
      disabled={disabled || !hasContent || anyUploading}
      testId="composer-send"
      icon={ArrowUp}
      label="Gönder"
      className={BTN_PRIMARY}
    />
  )

  if (waiting && !streaming) {
    return hasText ? (
      sendBtn
    ) : (
      <ActionButton
        onClick={onCancelWait}
        title="Otomatik uyandırmayı durdur"
        testId="composer-stop"
        icon={Square}
        label="Durdur"
        className={BTN_DANGER}
      />
    )
  }

  if (!streaming) {
    // Coordinator workers running while the coordinator's own turn is idle: sending
    // now HOLDS the message in the tray until the workers drain, so present it as a
    // queue action ("Sıraya") rather than a plain send. Same handler as Gönder — the
    // backend does the holding — so attachments and slash commands still work.
    if (workersActive && hasContent) {
      return (
        <ActionButton
          onClick={onSend}
          disabled={disabled || anyUploading}
          title="Workerlar çalışıyor — bitince sıradan gönderilecek"
          testId="composer-send"
          icon={ListPlus}
          label="Sıraya"
          className={BTN_QUEUE}
        />
      )
    }
    return sendBtn
  }

  if (hasText) {
    // Input filled while streaming → queue / interrupt / steer.
    return (
      <div className="flex items-end gap-1.5">
        <ActionButton
          onClick={onQueue}
          title="Bu tur bitince gönder"
          testId="composer-queue"
          icon={ListPlus}
          label="Sıraya"
          className={BTN_QUEUE}
        />
        <ActionButton
          onClick={onInterrupt}
          title="Turu kes ve hemen gönder"
          testId="composer-interrupt"
          icon={Scissors}
          label="Kes"
          className={BTN_WARNING}
        />
        <ActionButton
          onClick={onSteer}
          title="Çalışan turu canlı yönlendir (araç döngüsünde etkili)"
          testId="composer-steer"
          icon={Compass}
          label="Yönlendir"
          className={BTN_PRIMARY}
        />
      </div>
    )
  }

  // Streaming, empty input → stop.
  return (
    <ActionButton
      onClick={onStop}
      title="Üretimi durdur"
      testId="composer-stop"
      icon={Square}
      label="Durdur"
      className={BTN_DANGER}
    />
  )
}
