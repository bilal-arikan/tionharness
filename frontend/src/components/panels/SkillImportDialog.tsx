import { useMemo, useState } from 'react'
import { Download, Globe, FolderInput, AlertTriangle, CheckCircle2, Search, ChevronLeft } from 'lucide-react'
import { api } from '../../api'
import type { Discovered, IngestInstallResult, IngestKind } from '../../api/ingest'
import { Button, ModalOverlay } from '../common'

interface Props {
  onClose: () => void
  /** Called after a successful import so the host can refresh its collections. */
  onImported: (kinds: IngestKind[]) => void
}

type Source = 'local' | 'github'
type Step = 'locate' | 'select' | 'done'

const KIND_LABEL: Record<IngestKind, string> = {
  skill: 'Skills',
  agent: 'Agents',
  flow: 'Flows',
  provider: 'Providers',
  workspace: 'Workspaces',
  mcp: 'MCP araçları',
}

// deriveGroup suggests a Skills-UI group label from the scanned location so imported
// skills default to a namespace of their own (e.g. "owner/repo" or a GitHub tree URL
// → "repo", a local path → its last folder). Keeps a fresh import from mixing into the
// user's existing skills. Empty when nothing sensible can be derived.
function deriveGroup(location: string): string {
  const raw = location.trim().replace(/\/+$/, '')
  if (!raw) return ''
  let segs = raw.split(/[\\/]/).filter(Boolean)
  // For a github.com/owner/repo/tree/branch/... URL, the repo is the 2nd path segment
  // after the host; otherwise fall back to the last path segment.
  const host = segs.findIndex((s) => s.includes('github.com'))
  if (host >= 0 && segs.length > host + 2) return segs[host + 2]
  const last = segs[segs.length - 1] || ''
  return last.replace(/\.git$/, '')
}

// SkillImportDialog is the generic IMPORT dialog (SK-IMP3): it scans a source — a
// GitHub repo/plugin URL (or owner/repo shorthand) or a local folder TREE — and
// discovers every importable artifact inside (Claude Code skills, subagents, slash
// commands, MCP configs), grouped by kind, then installs the selected ones through
// the same authority the market uses. Nested resources are preserved; unsupported
// features are stripped with a warning.
export function SkillImportDialog({ onClose, onImported }: Props) {
  const [step, setStep] = useState<Step>('locate')
  const [source, setSource] = useState<Source>('github')
  const [location, setLocation] = useState('')
  const [slugPrefix, setSlugPrefix] = useState('')
  const [group, setGroup] = useState('')
  const [groupTouched, setGroupTouched] = useState(false)
  const [shared, setShared] = useState(false)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const [items, setItems] = useState<Discovered[]>([])
  const [scanWarnings, setScanWarnings] = useState<string[]>([])
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [result, setResult] = useState<IngestInstallResult | null>(null)

  // Group discovered items by kind for display.
  const groups = useMemo(() => {
    const m = new Map<IngestKind, Discovered[]>()
    for (const it of items) {
      const arr = m.get(it.kind) ?? []
      arr.push(it)
      m.set(it.kind, arr)
    }
    return Array.from(m.entries())
  }, [items])

  const doScan = async () => {
    if (!location.trim()) {
      setErr(source === 'github' ? 'GitHub URL’si / owner/repo gerekli.' : 'Yerel klasör yolu gerekli.')
      return
    }
    setBusy(true)
    setErr(null)
    try {
      const resp = await api.ingestScan({
        source,
        path: source === 'local' ? location.trim() : undefined,
        url: source === 'github' ? location.trim() : undefined,
      })
      setItems(resp.items)
      setScanWarnings(resp.warnings || [])
      setSelected(new Set(resp.items.filter((i) => !i.exists).map((i) => i.key)))
      // Default the group to the source name so imported skills don't mix with existing
      // ones — unless the user already typed their own group.
      if (!groupTouched) setGroup(deriveGroup(location))
      setStep('select')
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const toggle = (key: string) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })

  const selectAll = () => setSelected(new Set(items.filter((i) => !i.exists).map((i) => i.key)))
  const selectNone = () => setSelected(new Set())

  const doImport = async () => {
    if (selected.size === 0) {
      setErr('En az bir öğe seç.')
      return
    }
    setBusy(true)
    setErr(null)
    try {
      const resp = await api.ingestInstall({
        source,
        path: source === 'local' ? location.trim() : undefined,
        url: source === 'github' ? location.trim() : undefined,
        keys: Array.from(selected),
        slugPrefix: slugPrefix.trim() || undefined,
        group: group.trim() || undefined,
        shared,
      })
      setResult(resp)
      setStep('done')
      const kinds = Array.from(new Set(resp.installed.map((i) => i.kind))) as IngestKind[]
      if (kinds.length > 0) onImported(kinds)
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const inputCls =
    'w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-accent)]'

  return (
    <ModalOverlay onClose={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label="İçe aktar"
        data-testid="skill-import-modal"
        className="flex max-h-[90vh] w-full max-w-xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-3 border-b border-[var(--color-border)] px-5 py-4">
          <span className="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--color-surface-2)]">
            <Download size={18} className="text-[var(--color-accent)]" />
          </span>
          <div className="min-w-0 flex-1">
            <h2 className="text-sm font-semibold text-[var(--color-text)]">GitHub / klasörden içe aktar</h2>
            <p className="text-xs text-[var(--color-text-dim)]">
              Repo, plugin veya yerel klasör — skill, agent, komut ve MCP araçları keşfedilir. Nested kaynaklar
              korunur, uyumsuz özellikler ayıklanır.
            </p>
          </div>
        </div>

        <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-5">
          {/* STEP 1: locate */}
          {step === 'locate' && (
            <>
              <div className="flex gap-2">
                {(['github', 'local'] as Source[]).map((s) => (
                  <button
                    key={s}
                    data-testid={`import-source-${s}`}
                    onClick={() => setSource(s)}
                    className={`flex flex-1 items-center justify-center gap-1.5 rounded-md border px-3 py-2 text-sm transition ${
                      source === s
                        ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                        : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
                    }`}
                  >
                    {s === 'github' ? <Globe size={15} /> : <FolderInput size={15} />}
                    {s === 'github' ? 'GitHub' : 'Yerel klasör'}
                  </button>
                ))}
              </div>

              <label className="block">
                <span className="mb-1 block text-xs font-medium text-[var(--color-text-dim)]">
                  {source === 'github' ? 'GitHub URL / owner/repo' : 'Yerel klasör yolu (ağaç taranır)'}
                </span>
                <input
                  data-testid="import-location"
                  value={location}
                  onChange={(e) => setLocation(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && void doScan()}
                  placeholder={
                    source === 'github'
                      ? 'owner/repo  ·  github.com/owner/repo/tree/main/skills'
                      : 'C:\\path\\to\\repo'
                  }
                  className={`${inputCls} font-mono`}
                />
                {source === 'github' && (
                  <span className="mt-1 block text-[10px] text-[var(--color-text-dim)]">
                    Bir repo, plugin, <code>skills/</code> klasörü ya da tek skill klasörü olabilir.
                  </span>
                )}
              </label>
            </>
          )}

          {/* STEP 2: select */}
          {step === 'select' && (
            <>
              <div className="flex items-center justify-between">
                <p className="text-sm font-medium text-[var(--color-text)]">
                  {items.length} öğe bulundu
                  <span className="ml-2 text-xs font-normal text-[var(--color-text-dim)]">{selected.size} seçili</span>
                </p>
                <div className="flex gap-2 text-xs">
                  <button onClick={selectAll} className="text-[var(--color-accent)] hover:underline">
                    Tümü
                  </button>
                  <button onClick={selectNone} className="text-[var(--color-text-dim)] hover:underline">
                    Hiçbiri
                  </button>
                </div>
              </div>

              {scanWarnings.length > 0 && (
                <div className="rounded-md bg-[color-mix(in_srgb,var(--color-warning,#d97706)_12%,transparent)] px-3 py-2 text-xs text-[var(--color-warning,#d97706)]">
                  {scanWarnings.map((w, i) => (
                    <p key={i} className="flex items-start gap-1.5">
                      <AlertTriangle size={13} className="mt-0.5 shrink-0" /> {w}
                    </p>
                  ))}
                </div>
              )}

              <div data-testid="import-scan-list" className="max-h-72 space-y-3 overflow-y-auto rounded-md border border-[var(--color-border)] p-2">
                {groups.map(([kind, list]) => (
                  <div key={kind}>
                    <div className="mb-1 px-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                      {KIND_LABEL[kind]} ({list.length})
                    </div>
                    <div className="space-y-1">
                      {list.map((it) => (
                        <label
                          key={it.key}
                          data-testid="import-scan-item"
                          className={`flex cursor-pointer items-start gap-2.5 rounded px-2 py-1.5 text-sm hover:bg-[var(--color-surface-2)] ${
                            it.exists ? 'opacity-50' : ''
                          }`}
                        >
                          <input
                            type="checkbox"
                            className="mt-1 shrink-0"
                            checked={selected.has(it.key)}
                            disabled={it.exists}
                            onChange={() => toggle(it.key)}
                          />
                          <div className="min-w-0 flex-1">
                            <div className="flex items-center gap-2">
                              <code className="text-xs font-medium text-[var(--color-text)]">
                                {slugPrefix.trim() ? `${slugPrefix.trim()}-${it.slug}` : it.slug}
                              </code>
                              {it.exists && (
                                <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[9px] uppercase text-[var(--color-text-dim)]">
                                  zaten var
                                </span>
                              )}
                              {it.files.length > 0 && (
                                <span className="text-[10px] text-[var(--color-text-dim)]">+{it.files.length} dosya</span>
                              )}
                              {it.warnings && it.warnings.length > 0 && (
                                <span className="flex items-center gap-0.5 text-[10px] text-[var(--color-warning,#d97706)]">
                                  <AlertTriangle size={10} /> {it.warnings.length}
                                </span>
                              )}
                            </div>
                            {it.description && (
                              <p className="line-clamp-2 text-[11px] text-[var(--color-text-dim)]">{it.description}</p>
                            )}
                          </div>
                        </label>
                      ))}
                    </div>
                  </div>
                ))}
              </div>

              <div className="flex gap-3">
                <label className="block flex-1">
                  <span className="mb-1 block text-xs font-medium text-[var(--color-text-dim)]">Grup (opsiyonel)</span>
                  <input
                    data-testid="import-group"
                    value={group}
                    onChange={(e) => {
                      setGroup(e.target.value)
                      setGroupTouched(true)
                    }}
                    placeholder="Skills ekranında ayrı başlık altında toplanır"
                    className={inputCls}
                  />
                </label>
                <label className="flex items-end gap-2 pb-2">
                  <input
                    data-testid="import-shared"
                    type="checkbox"
                    checked={shared}
                    onChange={(e) => setShared(e.target.checked)}
                  />
                  <span className="text-sm text-[var(--color-text)]" title="Skill'leri tüm ajanlara on-demand sun">
                    Paylaşımlı
                  </span>
                </label>
              </div>

              <label className="block">
                <span className="mb-1 block text-xs font-medium text-[var(--color-text-dim)]">Slug öneki (opsiyonel)</span>
                <input
                  data-testid="import-slug-prefix"
                  value={slugPrefix}
                  onChange={(e) => setSlugPrefix(e.target.value)}
                  placeholder="ör. caveman → caveman-commit"
                  className={`${inputCls} font-mono`}
                />
              </label>
            </>
          )}

          {/* STEP 3: done */}
          {step === 'done' && result && (
            <div data-testid="import-result" className="space-y-3">
              <p className="flex items-center gap-1.5 text-sm font-medium text-[var(--color-success,#16a34a)]">
                <CheckCircle2 size={16} /> {result.installed.length} öğe içe aktarıldı
                {result.skipped.length > 0 && (
                  <span className="text-[var(--color-text-dim)]">· {result.skipped.length} atlandı</span>
                )}
              </p>

              {result.installed.length > 0 && (
                <div className="rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] p-3">
                  <div className="flex flex-wrap gap-1.5">
                    {result.installed.map((ir, i) => (
                      <code key={i} className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[11px]">
                        {ir.message}
                      </code>
                    ))}
                  </div>
                </div>
              )}

              {result.skipped.length > 0 && (
                <div className="rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] p-3 text-xs">
                  <p className="mb-1 font-medium text-[var(--color-text-dim)]">Atlananlar</p>
                  <ul className="space-y-0.5">
                    {result.skipped.map((s, i) => (
                      <li key={i} className="text-[var(--color-text-dim)]">
                        <code>{s.slug}</code> — {s.reason}
                      </li>
                    ))}
                  </ul>
                </div>
              )}

              {result.warnings.length > 0 && (
                <div className="text-xs text-[var(--color-warning,#d97706)]">
                  <p className="mb-1 flex items-center gap-1.5 font-medium">
                    <AlertTriangle size={13} /> Uyarılar
                  </p>
                  <ul className="list-disc space-y-0.5 pl-5">
                    {result.warnings.map((w, i) => (
                      <li key={i}>{w}</li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          )}

          {err && (
            <p className="flex items-start gap-1.5 rounded-md bg-[color-mix(in_srgb,var(--color-danger)_12%,transparent)] px-3 py-2 text-xs text-[var(--color-danger)]">
              <AlertTriangle size={14} className="mt-0.5 shrink-0" /> {err}
            </p>
          )}
        </div>

        <div className="flex items-center justify-between gap-2 border-t border-[var(--color-border)] px-5 py-4">
          <div>
            {step === 'select' && (
              <button
                onClick={() => {
                  setStep('locate')
                  setErr(null)
                }}
                className="flex items-center gap-1 rounded px-2 py-1.5 text-sm text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
              >
                <ChevronLeft size={14} /> Geri
              </button>
            )}
          </div>
          <div className="flex items-center gap-2">
            <button onClick={onClose} className="rounded px-3 py-1.5 text-sm text-[var(--color-text-dim)] hover:text-[var(--color-text)]">
              {step === 'done' ? 'Kapat' : 'İptal'}
            </button>
            {step === 'locate' && (
              <Button data-testid="import-scan" onClick={doScan} disabled={busy} className="flex items-center gap-1.5">
                <Search size={14} /> {busy ? 'Taranıyor…' : 'Tara'}
              </Button>
            )}
            {step === 'select' && (
              <Button data-testid="import-submit" onClick={doImport} disabled={busy || selected.size === 0} className="flex items-center gap-1.5">
                <Download size={14} /> {busy ? 'İçe aktarılıyor…' : `İçe aktar (${selected.size})`}
              </Button>
            )}
          </div>
        </div>
      </div>
    </ModalOverlay>
  )
}
