import { AskPrompt, type PendingAsk } from './AskPrompt'
import { PermissionPrompt } from './PermissionPrompt'
import { PlanPrompt } from './PlanPrompt'
import { useTranslation } from 'react-i18next'

export function InteractionPrompts({
  asks,
  onAnswer,
}: {
  asks: PendingAsk[]
  onAnswer: (text: string, interactionId?: string) => void
}) {
  const { t } = useTranslation('chat')
  if (!asks.length) return null
  return (
    <div className="flex max-h-[45vh] flex-col gap-2 overflow-y-auto">
      {asks.map((ask, index) => {
        const answer = (text: string) => onAnswer(text, ask.interactionId)
        return (
          <div key={ask.interactionId ?? index}>
            {ask.async && (
              <div className="mb-1 text-xs text-[var(--color-text-dim)]">{t('ask.asyncHint')}</div>
            )}
            {ask.kind === 'permission' ? (
              <PermissionPrompt ask={ask} onAnswer={answer} />
            ) : ask.kind === 'plan' ? (
              <PlanPrompt ask={ask} onAnswer={answer} />
            ) : (
              <AskPrompt ask={ask} onAnswer={answer} />
            )}
          </div>
        )
      })}
    </div>
  )
}
