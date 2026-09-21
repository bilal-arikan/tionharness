import { ArrowUp, Compass, ListPlus, Scissors, Square } from 'lucide-react'
import { ActionButton } from './ActionButton'
import { BTN_DANGER, BTN_PRIMARY, BTN_QUEUE, BTN_WARNING } from './buttonStyles'

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
  // Whether a live steer can actually REACH the running turn (queue_update.steerable
  // — the server's verdict, since it depends on the responding agent's provider and
  // effective permission mode). Gates the Yönlendir button the same way canSteer
  // gates the tray's "şimdi yönlendir" action.
  canSteer?: boolean
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
// Kes is also bound to Ctrl/Cmd+Enter in the composer (see Composer.onKeyDown).
//
// Every button is an ActionButton: icon + label on wide viewports, icon-only below
// the `sm` breakpoint. That matters most for the streaming triplet, which would
// otherwise be three labelled buttons competing for a narrow toolbar row.
export function SendActions({
  streaming,
  waiting,
  hasText,
  hasContent,
  anyUploading,
  disabled,
  canSteer = false,
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
    // Idle (including when this coordinator has background workers running): a message
    // is delivered directly. It runs as soon as the per-session turn slot is free —
    // serialized against any worker <task-notification> turn, not held for all workers.
    return sendBtn
  }

  if (hasContent) {
    // Input filled while streaming → queue / interrupt / steer. An attachment with
    // no text still counts: it can be queued or interrupt-sent as its own turn.
    // Steer is the exception — live guidance is text-only.
    return (
      <div className="flex items-end gap-1.5">
        <ActionButton
          onClick={onQueue}
          disabled={anyUploading}
          title="Bu tur bitince gönder"
          testId="composer-queue"
          icon={ListPlus}
          label="Sıraya"
          className={BTN_QUEUE}
        />
        <ActionButton
          onClick={onInterrupt}
          disabled={anyUploading}
          title="Turu kes ve hemen gönder (Ctrl/Cmd+Enter)"
          testId="composer-interrupt"
          icon={Scissors}
          label="Kes"
          className={BTN_WARNING}
        />
        <ActionButton
          onClick={onSteer}
          disabled={!hasText || !canSteer}
          // Shown disabled WITH the reason rather than hidden: the button must not
          // appear and vanish as turns come and go. Until TSK948 this was gated on
          // text alone, so on a turn whose provider/mode has no steer boundary it
          // stayed enabled and the user only learned it could not work from the
          // server's "unsupported" reply — after the message had already been sent
          // and silently requeued as a normal turn.
          title={
            canSteer
              ? 'Çalışan turu canlı yönlendir (araç döngüsünde etkili)'
              : 'Bu tur canlı yönlendirmeyi desteklemiyor (sağlayıcı/izin modu uygun değil) — "Sıraya" ile gönderebilirsin. Canlı yönlendirme için ajanı "ask" moduna al.'
          }
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
