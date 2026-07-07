// AppHeader is the app-level top bar for views that don't render their own
// in-pane header (see HEADERLESS_VIEWS). On chat it shows the session title and
// the folder/context/debug/detail shortcuts; on workspace/settings it hosts the
// mobile category-rail toggle.
import { Bug, Menu, PanelRight, ScanEye } from 'lucide-react'
import { api } from '@/api'
import type { Agent, Session } from '@/types'
import { CopyPathButton } from '@/shared/components/CopyPathButton'
import { RevealButton } from '@/shared/components/RevealButton'
import type { View } from './NavRail'
import { VIEW_TITLE } from './viewRegistry'

export interface AppHeaderProps {
  view: View
  sessions: Session[]
  agents: Agent[]
  activeSessionId: string | null
  activeAgentId: string | null
  detailOpen: boolean
  onOpenMobileList: () => void
  // workspace/settings category-rail collapse (shared with the panel).
  navOpen: boolean
  onToggleNav: () => void
  onOpenContextPreview: () => void
  onOpenDebug: () => void
  onToggleDetail: () => void
  onRevealSession: (id: string) => void
  onError: (msg: string) => void
}

export function AppHeader({
  view,
  sessions,
  agents,
  activeSessionId,
  activeAgentId,
  detailOpen,
  onOpenMobileList,
  navOpen,
  onToggleNav,
  onOpenContextPreview,
  onOpenDebug,
  onToggleDetail,
  onRevealSession,
  onError,
}: AppHeaderProps) {
  return (
    <header className="flex items-center justify-between gap-2 border-b border-[var(--color-border)] py-3 max-md:px-3 md:px-6">
      <div className="flex min-w-0 items-center gap-2">
        {view === 'chat' && (
          <button
            onClick={onOpenMobileList}
            aria-label="Oturumlar"
            title="Oturumlar"
            className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] md:hidden"
          >
            <Menu size={18} />
          </button>
        )}
        {(view === 'workspace' || view === 'settings') && (
          <button
            onClick={onToggleNav}
            aria-label="Panel listesini aç/kapat"
            aria-pressed={navOpen}
            title={navOpen ? 'Listeyi gizle' : 'Listeyi göster'}
            data-testid="pane-list-toggle"
            className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] md:hidden"
          >
            <Menu size={18} />
          </button>
        )}
        {view === 'chat' ? (
          // Show the session's own title (not "Sohbet · Ajan"); fall back to
          // the agent name, then a generic label for a fresh untitled chat.
          <span className="truncate text-sm font-semibold">
            {sessions.find((s) => s.id === activeSessionId)?.title ||
              agents.find((a) => a.id === activeAgentId)?.name ||
              'Yeni sohbet'}
          </span>
        ) : (
          <span className="shrink-0 text-sm font-semibold">{VIEW_TITLE[view]}</span>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-3">
        {view === 'chat' && activeSessionId && (
          <div className="flex items-center gap-1.5">
            {/* Folder shortcuts (moved here from the detail panel's Klasör card). */}
            <CopyPathButton
              getPath={async () => (await api.sessionPath(activeSessionId)).path}
              label="Yolu kopyala"
              labelClassName="hidden"
              title="Oturum klasörü yolunu kopyala"
              onError={onError}
            />
            <RevealButton
              onReveal={() => onRevealSession(activeSessionId)}
              label="Aç"
              labelClassName="hidden sm:inline"
              title="Oturum klasörünü aç"
            />
            {/* Next-turn context preview (moved here from the detail panel). */}
            <button
              onClick={onOpenContextPreview}
              title="Sonraki turun bağlamını önizle (debug)"
              className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
            >
              <ScanEye size={15} className="shrink-0" />
              <span className="hidden sm:inline">Bağlam</span>
            </button>
            {/* Debug / observability panel (moved out of the detail inspector). */}
            <button
              onClick={onOpenDebug}
              title="Debug / gözlemlenebilirlik panelini aç"
              className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
            >
              <Bug size={15} className="shrink-0" />
              <span className="hidden sm:inline">Debug</span>
            </button>
            <button
              onClick={onToggleDetail}
              title="Oturum bilgisi panelini aç/kapat"
              className={`flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-xs transition ${
                detailOpen
                  ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
                  : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-accent)]'
              }`}
            >
              <PanelRight size={15} className="shrink-0" />
              <span className="hidden sm:inline">Detay</span>
            </button>
          </div>
        )}
      </div>
    </header>
  )
}
