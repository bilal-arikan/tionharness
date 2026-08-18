import { useCallback, useEffect, useRef, useState } from 'react'
import {
  RefreshCw,
  Play,
  Bug,
  ShieldCheck,
  GraduationCap,
  ScanSearch,
  Boxes,
  History,
  Settings,
  type LucideIcon,
} from 'lucide-react'
import { api } from '@/api'
import type { InsightLens, InsightFinding, InsightSettings } from '@/types'
import { FindingsTab } from './FindingsTab'
import { LensList } from './LensList'
import { FleetTab } from './FleetTab'
import { RunsTab } from './RunsTab'
import { SettingsTab } from './SettingsTab'
import { LessonsTab } from './LessonsTab'
import { LessonsList } from '@/features/settings/LessonsList'

interface Props {
  onError: (msg: string) => void
  // Jump to a session transcript (evidence link).
  onOpenSession?: (sid: string) => void
  // Deep-link the active sub-tab: #/w/{ws}/insights/{tab}. When onTabChange is
  // wired the panel is URL-controlled; otherwise it falls back to local state.
  tab?: string | null
  onTabChange?: (t: string) => void
}

type Tab = 'findings' | 'lessons' | 'saved-lessons' | 'lenses' | 'fleet' | 'runs' | 'settings'

// Left-rail sub-pages (Settings-style vertical nav), each with an icon.
const TABS: { key: Tab; label: string; icon: LucideIcon }[] = [
  { key: 'findings', label: 'Bulgular', icon: Bug },
  { key: 'lessons', label: 'Öz-iyileşme', icon: ShieldCheck },
  { key: 'saved-lessons', label: 'Dersler', icon: GraduationCap },
  { key: 'lenses', label: 'Lensler', icon: ScanSearch },
  { key: 'fleet', label: 'Fleet', icon: Boxes },
  { key: 'runs', label: 'Geçmiş', icon: History },
  { key: 'settings', label: 'Ayarlar', icon: Settings },
]

// InsightPanel is the retrospective-scanner triage cockpit: run scans, review the
// deduplicated + priority-ranked findings, manage the editable lenses, and inspect
// the fleet backlog + scan-run history. A left sub-page rail (Settings-style) holds
// the scan actions on top + the sub-pages below.
export function InsightPanel({ onError, onOpenSession, tab: tabProp, onTabChange }: Props) {
  const [localTab, setLocalTab] = useState<Tab>('findings')
  // URL-controlled when a valid tab arrives via the route; else local state.
  const tab: Tab = TABS.some((t) => t.key === tabProp) ? (tabProp as Tab) : localTab
  const setTab = (t: Tab) => {
    setLocalTab(t)
    onTabChange?.(t)
  }
  const [lenses, setLenses] = useState<InsightLens[]>([])
  const [findings, setFindings] = useState<InsightFinding[]>([])
  const [settings, setSettings] = useState<InsightSettings>({})
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
    api
      .getInsightSettings()
      .then(setSettings)
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
            setScanNote('Tarama tamamlandı — bulgular güncellendi.')
            loadFindings()
          }
        })
        .catch(() => {})
    }
    pollRef.current = window.setInterval(tick, 4000)
    return () => {
      if (pollRef.current) window.clearInterval(pollRef.current)
    }
  }, [scanning, loadFindings])

  const runScan = async (lensIds?: string[]) => {
    setScanNote(null)
    try {
      await api.runInsightScan(lensIds ? { lensIds } : {})
      setScanning(true)
      setScanNote('Tarama arka planda başladı — bittiğinde bulgular otomatik güncellenir.')
    } catch (e) {
      const msg = (e as Error).message
      if (msg.includes('zaten çalışıyor') || msg.includes('409')) {
        setScanning(true)
        setScanNote('Bir tarama zaten çalışıyor — tamamlanması bekleniyor.')
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

  return (
    <div className="flex min-h-0 flex-1">
      {/* Left: scan actions on top + sub-page rail below (Settings-style). */}
      <aside className="flex h-full w-52 flex-shrink-0 flex-col overflow-y-auto border-r border-[var(--color-border)] bg-[var(--color-surface)]">
        {/* Header: label + refresh — same look/arrangement as the chat session list. */}
        <div className="flex items-center justify-between px-4 pt-4 pb-1">
          <span className="text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
            İçgörü
          </span>
          <button
            onClick={load}
            className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
            title="Yenile"
          >
            <RefreshCw size={14} className={scanning ? 'animate-spin' : ''} />
          </button>
        </div>

        {/* Prominent scan button — styled like the chat "+ Yeni Sohbet" button. */}
        <div className="px-3 pb-1 pt-1">
          <button
            onClick={() => runScan()}
            disabled={scanning}
            className="flex w-full items-center justify-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:cursor-not-allowed disabled:opacity-40"
            title="Retrospektif tarama başlat"
          >
            <Play size={15} /> {scanning ? 'Taranıyor…' : 'Tara'}
          </button>
        </div>

        {/* Sub-page rail. */}
        <div className="flex flex-col gap-1 p-2">
          {TABS.map((t) => {
            const Icon = t.icon
            const active = tab === t.key
            return (
              <button
                key={t.key}
                onClick={() => setTab(t.key)}
                className={`flex items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition ${
                  active
                    ? 'bg-[var(--color-accent-soft)] font-medium text-[var(--color-accent)]'
                    : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'
                }`}
              >
                <Icon size={15} /> {t.label}
              </button>
            )
          })}
        </div>
      </aside>

      {/* Right: active sub-page content. */}
      <div className="min-w-0 flex-1 overflow-auto p-4">
        {(scanNote || scanning) && (
          <div className="mb-3 flex items-center gap-2 rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] p-3 text-sm">
            {scanning && <RefreshCw className="h-4 w-4 animate-spin" />}
            <span>{scanning ? (scanNote ?? 'Tarama çalışıyor…') : scanNote}</span>
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
        {tab === 'lessons' && <LessonsTab onError={onError} />}
        {tab === 'saved-lessons' && <LessonsList fill />}
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
  )
}
