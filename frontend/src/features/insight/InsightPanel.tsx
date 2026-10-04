import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  RefreshCw,
  Play,
  Bug,
  ShieldCheck,
  ScanSearch,
  Boxes,
  History,
  Settings,
  type LucideIcon,
} from 'lucide-react'
import { api } from '@/api'
import type { Agent, InsightLens, InsightFinding, InsightSettings } from '@/types'
import { AgentPicker } from '@/shared/components/agents/AgentPicker'
import { ListPane, PaneHeader } from '@/shared/components'
import { SidebarHeader } from '@/shared/components/SidebarChrome'
import { useCollapsibleList } from '@/shared/hooks/useCollapsibleList'
import { FindingsTab } from './FindingsTab'
import { LensList } from './LensList'
import { FleetTab } from './FleetTab'
import { RunsTab } from './RunsTab'
import { SettingsTab } from './SettingsTab'
import { SelfHealingTab } from './SelfHealingTab'
import { persistAnalysisAgentSelection, withDefaultAnalysisAgent } from './insightAgentSelection'

interface Props {
  onError: (msg: string) => void
  // Jump to a session transcript (evidence link).
  onOpenSession?: (sid: string) => void
  // Deep-link the active sub-tab: #/w/{ws}/insights/{tab}. When onTabChange is
  // wired the panel is URL-controlled; otherwise it falls back to local state.
  tab?: string | null
  onTabChange?: (t: string) => void
}

type Tab = 'findings' | 'self-healing' | 'lenses' | 'fleet' | 'runs' | 'settings'

// Left-rail sub-pages (Settings-style vertical nav), each with an icon. The
// distilled lessons themselves live on the Notes screen (kind: lesson).
const TABS: { key: Tab; icon: LucideIcon }[] = [
  { key: 'findings', icon: Bug },
  { key: 'self-healing', icon: ShieldCheck },
  { key: 'lenses', icon: ScanSearch },
  { key: 'fleet', icon: Boxes },
  { key: 'runs', icon: History },
  { key: 'settings', icon: Settings },
]

// InsightPanel is the retrospective-scanner triage cockpit: run scans, review the
// deduplicated + priority-ranked findings, manage the editable lenses, and inspect
// the fleet backlog + scan-run history. A left sub-page rail (Settings-style) holds
// the scan actions on top + the sub-pages below.
export function InsightPanel({ onError, onOpenSession, tab: tabProp, onTabChange }: Props) {
  const { t } = useTranslation('insight')
  const [localTab, setLocalTab] = useState<Tab>('findings')
  // Left sub-page column: the standard collapsible list pane (persisted, default open).
  const { open: listOpen, toggle: toggleList } = useCollapsibleList('tionharness.insightListOpen')
  // URL-controlled when a valid tab arrives via the route; else local state.
  const tab: Tab = TABS.some((t) => t.key === tabProp) ? (tabProp as Tab) : localTab
  const setTab = (t: Tab) => {
    setLocalTab(t)
    onTabChange?.(t)
  }
  const [lenses, setLenses] = useState<InsightLens[]>([])
  const [findings, setFindings] = useState<InsightFinding[]>([])
  const [settings, setSettings] = useState<InsightSettings>({})
  const [agents, setAgents] = useState<Agent[]>([])
  const [scanning, setScanning] = useState(false)
  const [scanNote, setScanNote] = useState<string | null>(null)
  const pollRef = useRef<number | null>(null)

  const loadFindings = useCallback(() => {
    api
      .listInsightFindings()
      .then(setFindings)
      .catch((e) => onError((e as Error).message))
  }, [onError])

  const loadLenses = useCallback(() => {
    api
      .listInsightLenses()
      .then(setLenses)
      .catch((e) => onError((e as Error).message))
  }, [onError])

  const load = useCallback(() => {
    loadLenses()
    Promise.all([api.getInsightSettings(), api.listAgents()])
      .then(([loadedSettings, loadedAgents]) => {
        setAgents(loadedAgents)
        setSettings(withDefaultAnalysisAgent(loadedSettings, loadedAgents))
      })
      .catch((e) => onError((e as Error).message))
    loadFindings()
    api
      .getInsightScanStatus()
      .then((s) => setScanning(s.scanning))
      .catch(() => {})
  }, [onError, loadFindings, loadLenses])

  useEffect(load, [load])

  // Poll a running background scan; refresh findings when it finishes.
  useEffect(() => {
    if (!scanning) return
    const tick = () => {
      api
        .getInsightScanStatus()
        .then((s) => {
          if (!s.scanning) {
            setScanning(false)
            setScanNote(t('scan.completed'))
            loadFindings()
          }
        })
        .catch(() => {})
    }
    pollRef.current = window.setInterval(tick, 4000)
    return () => {
      if (pollRef.current) window.clearInterval(pollRef.current)
    }
  }, [scanning, loadFindings, t])

  const runScan = async (lensIds?: string[]) => {
    setScanNote(null)
    try {
      await api.runInsightScan(lensIds ? { lensIds } : {})
      setScanning(true)
      setScanNote(t('scan.started'))
    } catch (e) {
      const msg = (e as Error).message
      if (msg.includes('zaten çalışıyor') || msg.includes('409')) {
        setScanning(true)
        setScanNote(t('scan.alreadyRunning'))
      } else {
        onError(msg)
      }
    }
  }

  const toggleLens = async (id: string, enabled: boolean) => {
    try {
      await api.toggleLens(id, enabled)
      loadLenses()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const selectAnalysisAgent = (agentId: string) =>
    persistAnalysisAgentSelection(
      settings,
      agentId,
      setSettings,
      api.updateInsightSettings,
      onError,
    )

  return (
    <div className="flex h-full min-h-0 flex-1 overflow-hidden">
      {/* Left: scan actions on top + sub-page rail below — the standard ListPane
          column (collapsible to a reopen rail on md+, drawer on narrow, resizable). */}
      <ListPane
        open={listOpen}
        onToggle={toggleList}
        widthKey="tionharness.insightListWidth"
        defaultWidth={208}
        minWidth={176}
        label={t('title')}
        testId="insight-list-toggle"
      >
        {/* Header: label + refresh + collapse — same chrome as every list column. */}
        <SidebarHeader title={t('title')} onCollapse={toggleList}>
          <button
            onClick={load}
            className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
            title={t('actions.refresh')}
          >
            <RefreshCw size={14} className={scanning ? 'animate-spin' : ''} />
          </button>
        </SidebarHeader>

        {/* Prominent scan button — styled like the chat "+ Yeni Sohbet" button. */}
        <div className="px-3 pb-1 pt-1">
          <button
            onClick={() => runScan()}
            disabled={scanning}
            className="flex w-full items-center justify-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:cursor-not-allowed disabled:opacity-40"
            title={t('scan.startTitle')}
          >
            <Play size={15} /> {scanning ? t('scan.scanning') : t('scan.scan')}
          </button>
        </div>

        <div className="min-w-0 px-3 pb-2 pt-1">
          <label className="block min-w-0">
            <span className="mb-1 block truncate text-xs text-[var(--color-text-dim)]">
              {t('scan.analysisAgent')}
            </span>
            <div className="min-w-0 [&>div]:w-full [&_[data-testid=agent-picker-trigger]]:w-full [&_[data-testid=agent-picker-trigger]]:min-w-0">
              <AgentPicker
                agents={agents}
                value={settings.autoScanAgentId ?? ''}
                onChange={selectAnalysisAgent}
                placeholder={t('scan.selectAgent')}
              />
            </div>
          </label>
        </div>

        {/* Sub-page rail. */}
        <div className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto p-2">
          {TABS.map((tabItem) => {
            const Icon = tabItem.icon
            const active = tab === tabItem.key
            return (
              <button
                key={tabItem.key}
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

      {/* Right: standard pane header (list toggle + title) above the active
          sub-page content, like Tools / Skills. */}
      <div className="flex min-w-0 flex-1 flex-col">
        <PaneHeader
          title={t('title')}
          subtitle={'· ' + t(`tabs.${tab}`)}
          listOpen={listOpen}
          onToggleList={toggleList}
        />
        <div
          className={`min-w-0 flex-1 p-3 sm:p-4 ${
            tab === 'findings' ? 'flex min-h-0 flex-col overflow-hidden' : 'overflow-auto'
          }`}
        >
          {(scanNote || scanning) && (
            <div className="mb-3 flex items-center gap-2 rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] p-3 text-sm">
              {scanning && <RefreshCw className="h-4 w-4 animate-spin" />}
              <span>{scanning ? (scanNote ?? t('scan.running')) : scanNote}</span>
            </div>
          )}

          {tab === 'findings' && (
            <FindingsTab
              findings={findings}
              lenses={lenses}
              reload={loadFindings}
              onOpenSession={onOpenSession ?? (() => {})}
              onError={onError}
              onNote={setScanNote}
            />
          )}
          {tab === 'lenses' && (
            <LensList
              lenses={lenses}
              scanning={scanning}
              onToggle={toggleLens}
              onScanLens={(id) => runScan([id])}
              onSaved={loadLenses}
              onError={onError}
            />
          )}
          {tab === 'self-healing' && <SelfHealingTab onError={onError} />}
          {tab === 'fleet' && <FleetTab onError={onError} />}
          {tab === 'runs' && <RunsTab onError={onError} />}
          {tab === 'settings' && (
            <SettingsTab
              settings={settings}
              setSettings={setSettings}
              onError={onError}
              onReset={load}
            />
          )}
        </div>
      </div>
    </div>
  )
}
