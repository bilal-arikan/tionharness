import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Eye, FileText, NotebookPen, type LucideIcon } from 'lucide-react'
import { api } from '@/api'
import { SIGNAL_NOTES } from '@/app/eventToRefreshSignals'
import { ListPane, PaneHeader } from '@/shared/components'
import { RefreshButton, SidebarHeader } from '@/shared/components/SidebarChrome'
import { useCollapsibleList } from '@/shared/hooks/useCollapsibleList'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { useKeyedReset } from '@/shared/lib/useKeyedReset'
import { bumpSignal } from '@/shared/lib/refreshSignals'
import type { NoteStats } from '@/types'
import { NotesTab } from './NotesTab'
import { DigestsTab } from './DigestsTab'
import { ContextTab } from './ContextTab'
import { NOTE_KINDS } from './notesHelpers'

interface Props {
  onError: (msg: string) => void
  // Deep-link target: #/w/{ws}/notes/{noteId}. The parent owns it (URL sync).
  selectedId: string | null
  onSelectNote: (id: string | null) => void
  // Jump to a session transcript (digest / provenance links).
  onOpenSession?: (sessionId: string) => void
}

type Tab = 'notes' | 'digests' | 'context'

// Left-rail sub-pages (Insight-style vertical nav), each with an icon.
const TABS: { key: Tab; icon: LucideIcon }[] = [
  { key: 'notes', icon: NotebookPen },
  { key: 'digests', icon: FileText },
  { key: 'context', icon: Eye },
]

// NotesPanel is the workspace memory + awareness screen (_Docs/94): the notes
// agents and the user write, the end-of-turn digests other sessions read, and
// the "what did this session's agent see" inspector. A left sub-page rail holds
// the store counters on top and the three sub-pages below.
export function NotesPanel({ onError, selectedId, onSelectNote, onOpenSession }: Props) {
  const { t } = useTranslation('notes')
  const tick = useRefreshTrigger(SIGNAL_NOTES)
  const [tab, setTab] = useState<Tab>('notes')
  const { open: listOpen, toggle: toggleList } = useCollapsibleList('tionharness.notesListOpen')
  const [stats, setStats] = useState<NoteStats | null>(null)
  const [agentNames, setAgentNames] = useState<Record<string, string>>({})

  // A note arriving from outside (map "open in screen", a shared link) always
  // lands on the Notes sub-page, whatever tab was open.
  useKeyedReset(selectedId, (id) => {
    if (id) setTab('notes')
  })

  useEffect(() => {
    let cancelled = false
    api
      .noteStats()
      .then((s) => {
        if (!cancelled) setStats(s)
      })
      .catch((e) => {
        if (!cancelled) onError((e as Error).message)
      })
    return () => {
      cancelled = true
    }
  }, [tick, onError])

  useEffect(() => {
    let cancelled = false
    api
      .listAgents()
      .then((agents) => {
        if (!cancelled) setAgentNames(Object.fromEntries(agents.map((a) => [a.id, a.name || a.id])))
      })
      .catch(() => {
        // Names are decoration on the digest rows; ids still render.
      })
    return () => {
      cancelled = true
    }
  }, [])

  // Manual refresh: bump the same signal the SSE feed uses so every sub-page
  // (and the stats block) re-fetches through one path.
  const refresh = useCallback(() => bumpSignal(SIGNAL_NOTES), [])

  return (
    <div className="flex h-full min-h-0 flex-1 overflow-hidden">
      <ListPane
        open={listOpen}
        onToggle={toggleList}
        widthKey="tionharness.notesListWidth"
        defaultWidth={208}
        minWidth={176}
        label={t('title')}
        testId="notes-list-toggle"
      >
        <SidebarHeader title={t('title')} onCollapse={toggleList}>
          <RefreshButton onClick={refresh} />
        </SidebarHeader>

        {/* Store counters */}
        {stats && (
          <div className="mx-3 mb-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs">
            <div className="grid grid-cols-2 gap-x-2 gap-y-0.5">
              <Stat label={t('stats.active')} value={stats.active} />
              <Stat label={t('stats.private')} value={stats.private} />
              <Stat label={t('stats.retired')} value={stats.retired} />
              <Stat label={t('stats.archived')} value={stats.archived} />
            </div>
            {stats.active > 0 && (
              <div className="mt-1.5 flex flex-wrap gap-1 border-t border-[var(--color-border)] pt-1.5">
                {NOTE_KINDS.filter((k) => (stats.byKind[k] ?? 0) > 0).map((k) => (
                  <span
                    key={k}
                    className="rounded bg-[var(--color-bg)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)]"
                  >
                    {t(`kind.${k}`)} {stats.byKind[k]}
                  </span>
                ))}
              </div>
            )}
          </div>
        )}

        {/* Sub-page rail. */}
        <div className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto p-2">
          {TABS.map((tabItem) => {
            const Icon = tabItem.icon
            const active = tab === tabItem.key
            return (
              <button
                key={tabItem.key}
                type="button"
                data-testid={`notes-tab-${tabItem.key}`}
                onClick={() => setTab(tabItem.key)}
                className={`flex items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition ${
                  active
                    ? 'bg-[var(--color-accent-soft)] font-medium text-[var(--color-accent)]'
                    : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'
                }`}
              >
                <Icon size={15} /> {t(`tabs.${tabItem.key}`)}
              </button>
            )
          })}
        </div>
      </ListPane>

      <div className="flex min-w-0 flex-1 flex-col">
        <PaneHeader
          title={t('title')}
          subtitle={'· ' + t(`tabs.${tab}`)}
          listOpen={listOpen}
          onToggleList={toggleList}
        />
        <div className="min-h-0 min-w-0 flex-1 overflow-hidden">
          {tab === 'notes' && (
            <NotesTab
              onError={onError}
              selectedId={selectedId}
              onSelectNote={onSelectNote}
              onOpenSession={onOpenSession}
            />
          )}
          {tab === 'digests' && (
            <DigestsTab onError={onError} agentNames={agentNames} onOpenSession={onOpenSession} />
          )}
          {tab === 'context' && <ContextTab onError={onError} onOpenSession={onOpenSession} />}
        </div>
      </div>
    </div>
  )
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="text-[var(--color-text-dim)]">{label}</span>
      <span className="tabular-nums font-medium">{value}</span>
    </div>
  )
}
