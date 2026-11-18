import { ServerOff } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/shared/components'

interface Props {
  // Message of the failed request (already human-readable, from the API client).
  error: string
  // An automatic or manual attempt is in flight right now.
  retrying: boolean
  onRetry: () => void
}

// BackendUnreachableScreen replaces the shell when the workspace list cannot be
// loaded at boot. It says what failed and that the data is untouched, retries on
// its own (the caller owns the backoff) and offers a manual retry — instead of
// the onboarding screen, which would read as "your workspaces are gone".
export function BackendUnreachableScreen({ error, retrying, onRetry }: Props) {
  const { t } = useTranslation()
  return (
    <div
      data-testid="backend-unreachable"
      className="flex h-full w-full items-center justify-center bg-[var(--color-bg)] p-6 text-[var(--color-text)]"
    >
      <div className="flex max-w-md flex-col items-center gap-4 text-center">
        <ServerOff size={28} strokeWidth={1.5} className="text-[var(--color-text-dim)]" />
        <div className="flex flex-col gap-2">
          <h1 className="text-xl font-semibold">{t('shell.backendUnreachable.title')}</h1>
          <p className="text-sm text-[var(--color-text-dim)]">
            {t('shell.backendUnreachable.description')}
          </p>
        </div>
        <p role="alert" className="break-words text-xs text-[var(--color-text-dim)]">
          {error}
        </p>
        <Button onClick={onRetry} disabled={retrying} size="lg">
          {t('shell.backendUnreachable.retry')}
        </Button>
        <p aria-live="polite" className="text-xs text-[var(--color-text-dim)]">
          {retrying
            ? t('shell.backendUnreachable.retrying')
            : t('shell.backendUnreachable.autoRetry')}
        </p>
      </div>
    </div>
  )
}
