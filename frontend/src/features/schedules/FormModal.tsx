import type { LucideIcon } from 'lucide-react'
import { Trash2, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button, ModalOverlay } from '@/shared/components'

interface Props {
  title: string
  icon: LucideIcon
  accent: string
  submitLabel: string
  onSubmit: () => void
  onClose: () => void
  /** Present only in edit mode: deletes the rule (the caller confirms + closes). */
  onDelete?: () => void
  deleteTestId?: string
  testId?: string
  children: React.ReactNode
}

// FormModal is the shared create/edit dialog shell for the automation board:
// accented header with the rule kind, a scrollable body of fields, and a pinned
// footer with the submit/cancel pair.
export function FormModal({
  title,
  icon: Icon,
  accent,
  submitLabel,
  onSubmit,
  onClose,
  onDelete,
  deleteTestId,
  testId,
  children,
}: Props) {
  const { t } = useTranslation('schedules')
  return (
    <ModalOverlay onClose={onClose}>
      <div
        data-testid={testId}
        className="flex max-h-[85vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-2xl"
      >
        <header
          className="flex shrink-0 items-center gap-2 border-b border-[var(--color-border)] border-t-2 px-4 py-3"
          style={{ borderTopColor: accent }}
        >
          <Icon size={17} style={{ color: accent }} />
          <h2 className="flex-1 text-sm font-semibold text-[var(--color-text)]">{title}</h2>
          <button
            type="button"
            onClick={onClose}
            className="text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
            title={t('common.close')}
            aria-label={t('common.close')}
          >
            <X size={16} />
          </button>
        </header>
        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto p-4">{children}</div>
        <footer className="flex shrink-0 items-center gap-2 border-t border-[var(--color-border)] px-4 py-3">
          {/* Delete sits far left, away from the primary action. */}
          {onDelete && (
            <button
              type="button"
              data-testid={deleteTestId}
              onClick={onDelete}
              className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-danger)] hover:text-[var(--color-danger)]"
              title={t('form.deleteRule')}
            >
              <Trash2 size={14} />
              {t('common.delete')}
            </button>
          )}
          <div className="flex-1" />
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button onClick={onSubmit}>{submitLabel}</Button>
        </footer>
      </div>
    </ModalOverlay>
  )
}
