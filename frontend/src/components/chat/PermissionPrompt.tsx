import { ShieldAlert } from 'lucide-react'
import type { PendingAsk } from './AskPrompt'

interface Props {
  ask: PendingAsk
  onAnswer: (text: string) => void
}

// riskLabel maps a risk tier to a Turkish label for the card.
function riskLabel(risk?: string): string {
  switch (risk) {
    case 'exec':
      return 'komut çalıştırma'
    case 'write':
      return 'dosya/durum değişikliği'
    default:
      return risk || 'işlem'
  }
}

// PermissionPrompt renders the approval card shown while a turn is paused on the
// permission gate (ask mode): the gated tool + its risk, and Allow once / Always
// allow / Deny buttons. Each delivers the answer over the same channel as
// ask_user, unblocking the agent (or the CLI permission-prompt tool).
export function PermissionPrompt({ ask, onAnswer }: Props) {
  const options = ask.options?.length ? ask.options : ['İzin ver', 'Her zaman izin ver', 'Reddet']
  return (
    <div className="mx-3 mb-2 rounded-lg border border-amber-500/70 bg-amber-500/5 px-3 py-2.5">
      <div className="mb-2 flex items-start gap-2 text-sm text-[var(--color-text)]">
        <ShieldAlert size={16} className="mt-0.5 shrink-0 text-amber-500" />
        <span className="min-w-0 flex-1">
          Ajan{' '}
          <code className="rounded bg-[var(--color-surface-2)] px-1 py-0.5 text-xs">{ask.tool || 'bir araç'}</code>{' '}
          aracını çalıştırmak istiyor (<span className="font-medium">{riskLabel(ask.risk)}</span>). İzin veriyor musun?
        </span>
      </div>
      {ask.cmd && (
        <pre className="mb-2 overflow-x-auto whitespace-pre-wrap break-words rounded bg-[var(--color-surface-2)] px-2 py-1.5 text-xs text-[var(--color-text-dim)]">
          {ask.cmd}
        </pre>
      )}
      <div className="flex flex-wrap gap-1.5">
        {options.map((opt, i) => {
          const deny = /reddet|deny/i.test(opt)
          const always = /her zaman|always/i.test(opt)
          return (
            <button
              key={i}
              onClick={() => onAnswer(opt)}
              className={
                deny
                  ? 'rounded-full border border-red-500/60 px-3 py-1 text-xs text-red-400 hover:bg-red-500/10'
                  : always
                    ? 'rounded-full border border-[var(--color-border)] px-3 py-1 text-xs text-[var(--color-text)] hover:border-[var(--color-accent)] hover:bg-[var(--color-surface-2)]'
                    : 'rounded-full bg-amber-500/90 px-3 py-1 text-xs font-medium text-white hover:opacity-90'
              }
            >
              {opt}
            </button>
          )
        })}
      </div>
    </div>
  )
}
