import { ShieldAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import type { PendingAsk } from './AskPrompt'
import { ScrollableCard } from '@/shared/components'
import { ComposerCard } from './ComposerCard'

interface Props {
  ask: PendingAsk
  onAnswer: (text: string) => void
}

// riskLabel maps a risk tier to a localized label for the card.
function riskLabel(t: TFunction<'chatStatus'>, risk?: string): string {
  switch (risk) {
    case 'exec':
      return t('permission.risk.exec')
    // The decider's tool-risk check flagged a command that would otherwise have
    // run without asking (auto mode or an "always allow" family grant).
    case 'exec:decider':
      return t('permission.risk.execDecider')
    case 'write':
      return t('permission.risk.write')
    default:
      return risk || t('permission.risk.default')
  }
}

// PermissionPrompt renders the approval card shown while a turn is paused on the
// permission gate (ask mode): the gated tool + its risk, and Allow once / Always
// allow / Deny buttons. Each delivers the answer over the same channel as
// ask_user, unblocking the agent (or the CLI permission-prompt tool).
export function PermissionPrompt({ ask, onAnswer }: Props) {
  const { t } = useTranslation('chatStatus')
  const values = ask.options?.length ? ask.options : ['İzin ver', 'Her zaman izin ver', 'Reddet']
  const options = values.map((value) => ({
    value,
    label:
      value === 'İzin ver' || value === 'Allow once'
        ? t('permission.allowOnce')
        : value === 'Her zaman izin ver' || value === 'Always allow'
          ? t('permission.alwaysAllow')
          : value === 'Reddet' || value === 'Deny'
            ? t('permission.deny')
            : value,
  }))
  return (
    <ComposerCard tone="permission" className="px-3 py-2.5">
      <div className="mb-2 flex items-start gap-2 text-sm text-[var(--color-text)]">
        <ShieldAlert size={16} className="mt-0.5 shrink-0 text-[var(--color-warning)]" />
        <span className="min-w-0 flex-1">
          {t('permission.intro')}{' '}
          <code className="rounded bg-[var(--color-surface-2)] px-1 py-0.5 text-xs">
            {ask.tool || t('permission.defaultTool')}
          </code>{' '}
          <span className="font-medium">
            {' '}
            {t('permission.question', { risk: riskLabel(t, ask.risk) })}
          </span>
        </span>
      </div>
      {ask.cmd && (
        <ScrollableCard maxH="max-h-[40vh]" className="mb-2 rounded bg-[var(--color-surface-2)]">
          <pre className="whitespace-pre-wrap break-words px-2 py-1.5 text-xs text-[var(--color-text-dim)]">
            {ask.cmd}
          </pre>
        </ScrollableCard>
      )}
      <div className="flex flex-wrap gap-1.5">
        {options.map((opt, i) => {
          const deny = /reddet|deny/i.test(opt.value)
          const always = /her zaman|always/i.test(opt.value)
          return (
            <button
              key={i}
              onClick={() => onAnswer(opt.value)}
              className={
                deny
                  ? 'rounded-full border border-[var(--color-danger)]/60 px-3 py-1 text-xs text-[var(--color-danger)] hover:bg-[var(--color-danger)]/10'
                  : always
                    ? 'rounded-full border border-[var(--color-border)] px-3 py-1 text-xs text-[var(--color-text)] hover:border-[var(--color-accent)] hover:bg-[var(--color-surface-2)]'
                    : 'rounded-full bg-[var(--color-warning)]/90 px-3 py-1 text-xs font-medium text-[var(--color-on-warning)] hover:opacity-90'
              }
            >
              {opt.label}
            </button>
          )
        })}
      </div>
    </ComposerCard>
  )
}
