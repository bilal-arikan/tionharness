import { useCallback, useEffect, useRef, useState } from 'react'
import { RefreshCw, Play } from 'lucide-react'
import { api } from '@/api'
import { PaneHeader } from '@/shared/components'
import type { InsightLens, InsightFinding, InsightSettings } from '@/types'
import { FindingsTab } from './FindingsTab'
import { LensList } from './LensList'
import { FleetTab } from './FleetTab'
import { RunsTab } from './RunsTab'
import { SettingsTab } from './SettingsTab'
import { LessonsTab } from './LessonsTab'

interface Props {
  onError: (msg: string) => void
  // Jump to a session transcript (evidence link).
  onOpenSession?: (sid: string) => void
}

type Tab = 'findings' | 'lessons' | 'lenses' | 'fleet' | 'runs' | 'settings'

const TABS: { key: Tab; label: string }[] = [
  { key: 'findings', label: 'Bulgular' },
  { key: 'lessons', label: 'Dersler' },
  { key: 'lenses', label: 'Lensler' },
  { key: 'fleet', label: 'Fleet' },
  { key: 'runs', label: 'Geçmiş' },
  { key: 'settings', label: 'Ayarlar' },
]

// InsightPanel is the retrospective-scanner triage cockpit: run scans, review the
// deduplicated + priority-ranked findings (filter / search / cluster / bulk-triage
// / turn into board cards), manage the editable lenses, and inspect the fleet
// backlog + scan-run history.
export function InsightPanel({ onError, onOpenSession }: Props) {
  const [tab, setTab] = useState<Tab>('findings')
  const [lenses, setLenses] = useState<InsightLens[]>([])
  const [findings, setFindings] = useState<InsightFinding[]>([])
  const [settings, setSettings] = useState<InsightSettings>({})
  const [scanning, setScanning] = useState(false)
  const [scanNote, setScanNote] = useState<string | null>(null)
  const pollRef = useRef<number | null>(null)

  const loadFindings = useCallback(() => {
    api.listInsightFindings().then(setFindings).catch((e) => onError((e as Error).message))
  }, [onError])

  const loadLenses = useCallback(() => {
    api.listInsightLenses().then(setLenses).catch((e) => onError((e as Error).message))
  }, [onError])

  const load = useCallback(() => {
    loadLenses()
    api.getInsightSettings().then(setSettings).catch((e) => onError((e as Error).message))
    loadFindings()
    api.getInsightScanStatus().then((s) => setScanning(s.scanning)).catch(() => {})
  }, [onError, loadFindings, loadLenses])

  useEffect(load, [load])

  // Poll a running background scan; refresh findings when it finishes.
  useEffect(() => {
    if (!scanning) return
    const tick = () => {
      api.getInsightScanStatus().then((s) => {
        if (!s.scanning) {
          setScanning(false)
          setScanNote('Tarama tamamlandı — bulgular güncellendi.')
          loadFindings()
        }
      }).catch(() => {})
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
    <div className="flex h-full flex-col">
      <PaneHeader
        title="İçgörü"
        right={
          <div className="flex items-center gap-2">
            <button onClick={load} className="flex items-center gap-1 rounded-md px-2 py-1 text-sm hover:bg-[var(--color-surface-2)]" title="Yenile">
              <RefreshCw className="h-4 w-4" />
            </button>
            <button
              onClick={() => runScan()}
              disabled={scanning}
              className="flex items-center gap-1 rounded-md bg-[var(--color-accent)] px-3 py-1 text-sm text-white disabled:opacity-50"
            >
              <Play className="h-4 w-4" />
              {scanning ? 'Taranıyor…' : 'Tara'}
            </button>
          </div>
        }
      />

      {/* Tab bar */}
      <div className="flex gap-1 border-b border-[var(--color-border)] px-4">
        {TABS.map((t) => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            className={`border-b-2 px-3 py-2 text-sm ${
              tab === t.key
                ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
                : 'border-transparent text-[var(--color-text-muted)] hover:text-[var(--color-text)]'
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>

      <div className="flex-1 overflow-auto p-4">
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
        {tab === 'fleet' && <FleetTab onError={onError} />}
        {tab === 'runs' && <RunsTab onError={onError} />}
        {tab === 'settings' && (
          <SettingsTab settings={settings} setSettings={setSettings} onError={onError} onReset={load} />
        )}
      </div>
    </div>
  )
}
