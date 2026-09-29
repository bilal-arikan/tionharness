import type { Dispatch, SetStateAction } from 'react'
import { RotateCcw } from 'lucide-react'
import { EmojiField } from '@/shared/components/EmojiField'
import { normalizeAvatar } from '@/shared/lib/avatar'
import { CopyPathButton } from '@/shared/components/CopyPathButton'
import { ViewButton } from '@/features/view/ViewButton'
import { statusLabel, statusColor } from './runStatus'
import { flowTemplateDescription, flowTemplateName, type FlowTemplate } from './flowTemplates'
import type { FlowsTab } from './flowsPanelShared'
import type { Flow, FlowRun } from '@/types'
import { Button, PaneHeader } from '@/shared/components'
import { formatDateTime } from '@/shared/lib/intl'
import { useTranslation } from 'react-i18next'

interface Props {
  flowsListOpen: boolean
  toggleFlowsList: () => void
  tab: FlowsTab
  flows: Flow[]
  selectedId: string | null
  selectedTemplate: FlowTemplate | null
  selectedRun: FlowRun | null
  emoji: string
  changeEmoji: (next: string) => void
  name: string
  setName: Dispatch<SetStateAction<string>>
  flowPath: string
  saveFlow: () => void
  instantiateTemplate: (t: FlowTemplate) => void
  rerunRun: (r: FlowRun) => void
  rerunning: boolean
}

// FlowsHeader is the flows screen's top PaneHeader: the flow editor's
// emoji/name/id + file actions + save, the template preview's name + create
// button, or the run inspector's flow/status/date + re-run button — depending
// on the active tab and selection.
export function FlowsHeader({
  flowsListOpen,
  toggleFlowsList,
  tab,
  flows,
  selectedId,
  selectedTemplate,
  selectedRun,
  emoji,
  changeEmoji,
  name,
  setName,
  flowPath,
  saveFlow,
  instantiateTemplate,
  rerunRun,
  rerunning,
}: Props) {
  const { t } = useTranslation('flows')
  return (
    <PaneHeader
      listOpen={flowsListOpen}
      onToggleList={toggleFlowsList}
      // Detail views (flow editor / template preview / run inspector) put their
      // own toolbar directly in the top bar via titleSlot + right — no redundant
      // "Akışlar" title / subtitle. Empty/list states keep the "Akışlar" title.
      title={
        (tab === 'flows' && selectedId) ||
        (tab === 'templates' && selectedTemplate) ||
        (tab === 'runs' && selectedRun)
          ? undefined
          : t('header.flows')
      }
      titleSlot={
        tab === 'flows' && selectedId ? (
          <>
            <EmojiField value={emoji} onChange={changeEmoji} clearLabel="🔀" />
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('header.flowNamePlaceholder')}
              className="min-w-0 flex-1 rounded bg-[var(--color-surface-2)] px-3 py-1.5 text-sm font-medium outline-none"
            />
            <span
              className="flex-shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--color-text-dim)]"
              title={t('header.flowIdTitle')}
            >
              {selectedId}
            </span>
          </>
        ) : tab === 'templates' && selectedTemplate ? (
          <span className="flex min-w-0 flex-col leading-tight">
            <span className="truncate text-sm font-medium">
              {flowTemplateName(selectedTemplate)}
            </span>
            {selectedTemplate.description && (
              <span className="truncate text-xs text-[var(--color-text-dim)]">
                {flowTemplateDescription(selectedTemplate)}
              </span>
            )}
          </span>
        ) : tab === 'runs' && selectedRun ? (
          <span className="flex min-w-0 items-center gap-2">
            <span className="truncate text-sm font-medium">
              {(() => {
                const rf = flows.find((f) => f.id === selectedRun.flowId)
                const e = normalizeAvatar(rf?.emoji)
                return `${e ? e + ' ' : ''}${rf?.name ?? t('flow.deletedName')}`
              })()}
            </span>
            <span className={`flex-shrink-0 text-xs ${statusColor(selectedRun.status)}`}>
              {statusLabel(selectedRun.status)}
            </span>
            <span className="flex-shrink-0 text-xs text-[var(--color-text-dim)]">
              {formatDateTime(new Date(selectedRun.createdAt * 1000), {
                dateStyle: 'short',
                timeStyle: 'medium',
              })}
            </span>
          </span>
        ) : undefined
      }
      right={
        tab === 'flows' && selectedId ? (
          <>
            <CopyPathButton path={flowPath} title={t('header.flowPathCopy')} />
            <Button onClick={saveFlow} size="lg" className="flex-shrink-0">
              {t('actions.save')}
            </Button>
          </>
        ) : tab === 'templates' && selectedTemplate ? (
          <>
            <span className="hidden text-xs text-[var(--color-text-dim)] sm:inline">
              {t('header.readOnlyPreview')}
            </span>
            <Button
              onClick={() => instantiateTemplate(selectedTemplate)}
              size="lg"
              className="flex-shrink-0"
            >
              {t('actions.createFromTemplate')}
            </Button>
          </>
        ) : tab === 'runs' && selectedRun ? (
          <>
            {/* The projection of this run — the same bytes an agent would get. */}
            <ViewButton target={{ kind: 'flowrun', id: selectedRun.id }} />
            <button
              type="button"
              onClick={() => rerunRun(selectedRun)}
              disabled={
                rerunning ||
                selectedRun.status === 'running' ||
                !flows.some((f) => f.id === selectedRun.flowId)
              }
              title={
                !flows.some((f) => f.id === selectedRun.flowId)
                  ? t('header.rerunDeleted')
                  : t('header.rerunTitle')
              }
              className="flex shrink-0 items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:cursor-not-allowed disabled:opacity-50"
            >
              <RotateCcw size={13} className={rerunning ? 'animate-spin' : ''} />
              {rerunning ? t('actions.running') : t('actions.rerun')}
            </button>
          </>
        ) : undefined
      }
    />
  )
}
