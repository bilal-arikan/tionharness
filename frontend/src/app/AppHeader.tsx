import { useTranslation } from 'react-i18next'
// AppHeader is the app-level top bar for views that don't render their own
// in-pane header (see HEADERLESS_VIEWS). On chat it shows the session title and
// the folder/context/debug/detail shortcuts; on workspace/settings it hosts the
// category-rail toggle. Both list toggles work at every width.
import { PanelLeft, PanelRight, ScanEye } from 'lucide-react'
import { api } from '@/api'
import type { Agent, Session } from '@/types'
import { CopyPathButton } from '@/shared/components/CopyPathButton'
import type { View } from './NavRail'
import { VIEW_TITLE } from './viewRegistry'

export interface AppHeaderProps {
  view: View
  sessions: Session[]
  agents: Agent[]
  activeSessionId: string | null
  activeAgentId: string | null
  detailOpen: boolean
  // chat: sessions list collapse (docked column on md+, drawer on narrow).
  listOpen: boolean
  onToggleList: () => void
  // workspace/settings category-rail collapse (shared with the panel).
  navOpen: boolean
  onToggleNav: () => void
  onOpenContextPreview: () => void
  onToggleDetail: () => void
  onError: (msg: string) => void
}

export function AppHeader({
  view,
  sessions,
  agents,
  activeSessionId,
  activeAgentId,
  detailOpen,
  listOpen,
  onToggleList,
  navOpen,
  onToggleNav,
  onOpenContextPreview,
  onToggleDetail,
  onError,
}: AppHeaderProps) {
  const { t } = useTranslation()
  const activeSession = view === 'chat' ? sessions.find((s) => s.id === activeSessionId) : undefined
  return (
    <header className="flex items-center justify-between gap-2 border-b border-[var(--color-border)] py-3 max-md:px-3 md:px-6">
      <div className="flex min-w-0 items-center gap-2">
        {view === 'chat' && (
          <button
            onClick={onToggleList}
            aria-label={t('shell.toggleSessions')}
            aria-pressed={listOpen}
            title={listOpen ? t('shell.hideSessions') : t('shell.showSessions')}
            data-testid="pane-list-toggle"
            className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-lg transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] ${
              listOpen ? 'text-[var(--color-text-dim)]' : 'text-[var(--color-accent)]'
            }`}
          >
            <PanelLeft size={18} />
          </button>
        )}
        {(view === 'workspace' || view === 'settings') && (
          <button
            onClick={onToggleNav}
            aria-label={t('shell.togglePanels')}
            aria-pressed={navOpen}
            title={navOpen ? t('shell.hideList') : t('shell.showList')}
            data-testid="pane-list-toggle"
            className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-lg transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] ${
              navOpen ? 'text-[var(--color-text-dim)]' : 'text-[var(--color-accent)]'
            }`}
          >
            <PanelLeft size={18} />
          </button>
        )}
        {view === 'chat' ? (
          // Show the session's own title (not "Sohbet · Ajan"); fall back to
          // the agent name, then a generic label for a fresh untitled chat.
          <span className="truncate text-sm font-semibold">
            {activeSession?.title ||
              agents.find((a) => a.id === activeAgentId)?.name ||
              t('shell.newChat')}
          </span>
        ) : (
          <span className="shrink-0 text-sm font-semibold">{VIEW_TITLE[view]}</span>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-3">
        {view === 'chat' && activeSessionId && (
          <div className="flex items-center gap-1.5">
            {/* Folder path shortcut (moved here from the detail panel's Klasör card). */}
            <CopyPathButton
              getPath={async () => (await api.sessionPath(activeSessionId)).path}
              title={t('shell.copySessionPath')}
              onError={onError}
            />
            {/* Next-turn context preview (moved here from the detail panel). */}
            <button
              onClick={onOpenContextPreview}
              title={t('shell.previewContext')}
              className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
            >
              <ScanEye size={15} className="shrink-0" />
              <span className="hidden sm:inline">{t('shell.context')}</span>
            </button>
            {/* Debug / observability lives in the "Oturum bilgisi" detail panel's
                action list (SessionDetailPanel), not in this header. */}
            <button
              onClick={onToggleDetail}
              title={t('shell.toggleDetails')}
              className={`flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-xs transition ${
                detailOpen
                  ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
                  : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-accent)]'
              }`}
            >
              <PanelRight size={15} className="shrink-0" />
              <span className="hidden sm:inline">{t('shell.details')}</span>
            </button>
          </div>
        )}
      </div>
    </header>
  )
}
