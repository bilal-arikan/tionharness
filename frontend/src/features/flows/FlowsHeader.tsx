import type { Dispatch, SetStateAction } from 'react'
import { RotateCcw } from 'lucide-react'
import { EmojiField } from '@/shared/components/EmojiField'
import { normalizeAvatar } from '@/shared/lib/avatar'
import { CopyPathButton } from '@/shared/components/CopyPathButton'
import { ViewButton } from '@/features/view/ViewButton'
import { STATUS_LABEL, statusColor } from './RunView'
import type { FlowTemplate } from './flowTemplates'
import type { FlowsTab } from './flowsPanelShared'
import type { Flow, FlowRun } from '@/types'
import { Button, PaneHeader } from '@/shared/components'

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
          : 'Akışlar'
      }
      titleSlot={
        tab === 'flows' && selectedId ? (
          <>
            <EmojiField value={emoji} onChange={changeEmoji} clearLabel="🔀" />
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Akış adı"
              className="min-w-0 flex-1 rounded bg-[var(--color-surface-2)] px-3 py-1.5 text-sm font-medium outline-none"
            />
            <span
              className="flex-shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--color-text-dim)]"
              title="Akış ID (dosya adı)"
            >
              {selectedId}
            </span>
          </>
        ) : tab === 'templates' && selectedTemplate ? (
          <span className="flex min-w-0 flex-col leading-tight">
            <span className="truncate text-sm font-medium">{selectedTemplate.name}</span>
            {selectedTemplate.description && (
              <span className="truncate text-xs text-[var(--color-text-dim)]">
                {selectedTemplate.description}
              </span>
            )}
          </span>
        ) : tab === 'runs' && selectedRun ? (
          <span className="flex min-w-0 items-center gap-2">
            <span className="truncate text-sm font-medium">
              {(() => {
                const rf = flows.find((f) => f.id === selectedRun.flowId)
                const e = normalizeAvatar(rf?.emoji)
                return `${e ? e + ' ' : ''}${rf?.name ?? '（silinmiş akış）'}`
              })()}
            </span>
            <span className={`flex-shrink-0 text-xs ${statusColor(selectedRun.status)}`}>
              {STATUS_LABEL[selectedRun.status] ?? selectedRun.status}
            </span>
            <span className="flex-shrink-0 text-xs text-[var(--color-text-dim)]">
              {new Date(selectedRun.createdAt * 1000).toLocaleString()}
            </span>
          </span>
        ) : undefined
      }
      right={
        tab === 'flows' && selectedId ? (
          <>
            <CopyPathButton path={flowPath} title="Akış yolunu kopyala" />
            <Button onClick={saveFlow} size="lg" className="flex-shrink-0">
              Kaydet
            </Button>
          </>
        ) : tab === 'templates' && selectedTemplate ? (
          <>
            <span className="hidden text-xs text-[var(--color-text-dim)] sm:inline">
              salt-okunur önizleme
            </span>
            <Button
              onClick={() => instantiateTemplate(selectedTemplate)}
              size="lg"
              className="flex-shrink-0"
            >
              + Bu şablondan akış oluştur
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
                  ? 'Akış silinmiş — tekrar çalıştırılamaz'
                  : 'Bu koşuyu aynı girdiyle tekrar çalıştır'
              }
              className="flex shrink-0 items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:cursor-not-allowed disabled:opacity-50"
            >
              <RotateCcw size={13} className={rerunning ? 'animate-spin' : ''} />
              {rerunning ? 'Çalışıyor…' : 'Tekrar çalıştır'}
            </button>
          </>
        ) : undefined
      }
    />
  )
}
