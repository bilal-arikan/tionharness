import { useMemo, useState } from 'react'
import { Download, Globe, FolderInput, AlertTriangle, CheckCircle2, Search, ChevronLeft } from 'lucide-react'
import { api } from '../../api'
import type { ScannedSkill, CollectionImportResult } from '../../api/skills'
import { Button } from '../common'

interface Props {
  onClose: () => void
  /** Called after a successful import so the list + selection can refresh. */
  onImported: (slug: string) => void
}

type Source = 'local' | 'github'
type Step = 'locate' | 'select' | 'done'

// SkillImportDialog imports Claude Code skills into the workspace (SK-IMP/SK-IMP2).
// It scans a source — a GitHub repo/plugin URL (or owner/repo shorthand) or a local
// folder TREE — discovers every SKILL.md inside (single skill or a whole collection
// like caveman / taste-skill / marketing-skills), then lets the user pick which to
// import. Nested resources (references/, evals/…) are preserved; unsupported CC
// features are stripped with a warning.
export function SkillImportDialog({ onClose, onImported }: Props) {
  const [step, setStep] = useState<Step>('locate')
  const [source, setSource] = useState<Source>('github')
  const [location, setLocation] = useState('')
  const [slugPrefix, setSlugPrefix] = useState('')
  const [shared, setShared] = useState(false)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const [scanned, setScanned] = useState<ScannedSkill[]>([])
  const [scanWarnings, setScanWarnings] = useState<string[]>([])
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [result, setResult] = useState<CollectionImportResult | null>(null)

  const selectableCount = useMemo(() => scanned.filter((s) => !s.exists).length, [scanned])

  const doScan = async () => {
    if (!location.trim()) {
      setErr(source === 'github' ? 'GitHub URL’si / owner/repo gerekli.' : 'Yerel klasör yolu gerekli.')
      return
    }
    setBusy(true)
    setErr(null)
    try {
      const resp = await api.scanCollection({
        source,
        path: source === 'local' ? location.trim() : undefined,
        url: source === 'github' ? location.trim() : undefined,
      })
      setScanned(resp.skills)
      setScanWarnings(resp.warnings || [])
      // Default selection: everything not already installed.
      setSelected(new Set(resp.skills.filter((s) => !s.exists).map((s) => s.relPath)))
      setStep('select')
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const toggle = (relPath: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(relPath)) next.delete(relPath)
      else next.add(relPath)
      return next
    })
  }

  const selectAll = () => setSelected(new Set(scanned.filter((s) => !s.exists).map((s) => s.relPath)))
  const selectNone = () => setSelected(new Set())

  const doImport = async () => {
    if (selected.size === 0) {
      setErr('En az bir skill seç.')
      return
    }
    setBusy(true)
    setErr(null)
    try {
      const resp = await api.importCollection({
        source,
        path: source === 'local' ? location.trim() : undefined,
        url: source === 'github' ? location.trim() : undefined,
        paths: Array.from(selected),
        slugPrefix: slugPrefix.trim() || undefined,
        shared,
      })
      setResult(resp)
      setStep('done')
      if (resp.imported.length > 0) onImported(resp.imported[0].slug)
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const inputCls =
    'w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-accent)]'

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Skill içe aktar"
        data-testid="skill-import-modal"
        className="flex max-h-[90vh] w-full max-w-xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-3 border-b border-[var(--color-border)] px-5 py-4">
          <span className="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--color-surface-2)]">
            <Download size={18} className="text-[var(--color-accent)]" />
          </span>
          <div className="min-w-0 flex-1">
            <h2 className="text-sm font-semibold text-[var(--color-text)]">Claude Code skill içe aktar</h2>
            <p className="text-xs text-[var(--color-text-dim)]">
              GitHub repo/plugin veya yerel klasör — tek skill ya da koleksiyon. Frontmatter eşlenir, nested
              kaynaklar (references/, evals/…) korunur, uyumsuz özellikler ayıklanır.
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
                      : 'C:\\path\\to\\skills-repo'
                  }
                  className={`${inputCls} font-mono`}
                />
                {source === 'github' && (
                  <span className="mt-1 block text-[10px] text-[var(--color-text-dim)]">
                    Bir repo, bir <code>skills/</code> klasörü, bir plugin ya da tek skill klasörü olabilir.
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
                  {scanned.length} skill bulundu
                  <span className="ml-2 text-xs font-normal text-[var(--color-text-dim)]">
                    {selected.size} seçili
                  </span>
                </p>
                <div className="flex gap-2 text-xs">
                  <button onClick={selectAll} className="text-[var(--color-accent)] hover:underline" disabled={selectableCount === 0}>
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

              <div data-testid="import-scan-list" className="max-h-72 space-y-1 overflow-y-auto rounded-md border border-[var(--color-border)] p-1">
                {scanned.map((sk) => (
                  <label
                    key={sk.relPath}
                    data-testid="import-scan-item"
                    className={`flex cursor-pointer items-start gap-2.5 rounded px-2 py-1.5 text-sm hover:bg-[var(--color-surface-2)] ${
                      sk.exists ? 'opacity-50' : ''
                    }`}
                  >
                    <input
                      type="checkbox"
                      className="mt-1 shrink-0"
                      checked={selected.has(sk.relPath)}
                      disabled={sk.exists}
                      onChange={() => toggle(sk.relPath)}
                    />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <code className="text-xs font-medium text-[var(--color-text)]">
                          {slugPrefix.trim() ? `${slugPrefix.trim()}-${sk.slug}` : sk.slug}
                        </code>
                        {sk.exists && (
                          <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[9px] uppercase text-[var(--color-text-dim)]">
                            zaten var
                          </span>
                        )}
                        {sk.files.length > 0 && (
                          <span className="text-[10px] text-[var(--color-text-dim)]">+{sk.files.length} dosya</span>
                        )}
                      </div>
                      {sk.description && (
                        <p className="line-clamp-2 text-[11px] text-[var(--color-text-dim)]">{sk.description}</p>
                      )}
                    </div>
                  </label>
                ))}
              </div>

              <div className="flex gap-3">
                <label className="block flex-1">
                  <span className="mb-1 block text-xs font-medium text-[var(--color-text-dim)]">Slug öneki (opsiyonel)</span>
                  <input
                    data-testid="import-slug-prefix"
                    value={slugPrefix}
                    onChange={(e) => setSlugPrefix(e.target.value)}
                    placeholder="ör. caveman → caveman-commit"
                    className={`${inputCls} font-mono`}
                  />
                </label>
                <label className="flex items-end gap-2 pb-2">
                  <input
                    data-testid="import-shared"
                    type="checkbox"
                    checked={shared}
                    onChange={(e) => setShared(e.target.checked)}
                  />
                  <span className="text-sm text-[var(--color-text)]" title="Tüm ajanlara on-demand sun; kapalıysa kısıtlı (atama gerekir)">
                    Paylaşımlı
                  </span>
                </label>
              </div>
            </>
          )}

          {/* STEP 3: done */}
          {step === 'done' && result && (
            <div data-testid="import-result" className="space-y-3">
              <p className="flex items-center gap-1.5 text-sm font-medium text-[var(--color-success,#16a34a)]">
                <CheckCircle2 size={16} /> {result.imported.length} skill içe aktarıldı
                {result.skipped.length > 0 && (
                  <span className="text-[var(--color-text-dim)]">· {result.skipped.length} atlandı</span>
                )}
              </p>

              {result.imported.length > 0 && (
                <div className="rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] p-3">
                  <div className="flex flex-wrap gap-1.5">
                    {result.imported.map((ir) => (
                      <code key={ir.slug} className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[11px]">
                        {ir.slug}
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

              {/* Aggregate any per-skill mapping warnings. */}
              {(() => {
                const warns = result.imported.flatMap((ir) => ir.warnings.map((w) => ({ slug: ir.slug, w })))
                if (warns.length === 0 && result.warnings.length === 0) {
                  return <p className="text-xs text-[var(--color-text-dim)]">Uyumsuz özellik yok — temiz içe aktarma.</p>
                }
                return (
                  <div className="text-xs text-[var(--color-warning,#d97706)]">
                    <p className="mb-1 flex items-center gap-1.5 font-medium">
                      <AlertTriangle size={13} /> Uyarılar ({warns.length + result.warnings.length})
                    </p>
                    <ul className="list-disc space-y-0.5 pl-5">
                      {result.warnings.map((w, i) => (
                        <li key={`g${i}`}>{w}</li>
                      ))}
                      {warns.map((x, i) => (
                        <li key={i}>
                          <code>{x.slug}</code>: {x.w}
                        </li>
                      ))}
                    </ul>
                  </div>
                )
              })()}
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
    </div>
  )
}
