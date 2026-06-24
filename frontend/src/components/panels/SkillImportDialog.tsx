import { useState } from 'react'
import { Download, Globe, FolderInput, AlertTriangle, CheckCircle2 } from 'lucide-react'
import { api } from '../../api'
import type { SkillImportResult } from '../../api/skills'
import { Button } from '../common'

interface Props {
  onClose: () => void
  /** Called with the imported skill's slug so the list + selection can refresh. */
  onImported: (slug: string) => void
}

type Source = 'local' | 'github'

// SkillImportDialog imports a Claude Code skill into the workspace (SK-IMP): pick a
// source (a local folder path or a github.com folder URL), optionally override the
// slug and share it, then import. On success it shows the mapped result + any
// warnings about unsupported CC features that were dropped.
export function SkillImportDialog({ onClose, onImported }: Props) {
  const [source, setSource] = useState<Source>('github')
  const [location, setLocation] = useState('')
  const [slug, setSlug] = useState('')
  const [shared, setShared] = useState(false)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [result, setResult] = useState<SkillImportResult | null>(null)

  const doImport = async () => {
    if (!location.trim()) {
      setErr(source === 'github' ? 'GitHub klasör URL’si gerekli.' : 'Yerel klasör yolu gerekli.')
      return
    }
    setBusy(true)
    setErr(null)
    try {
      const resp = await api.importSkill({
        source,
        path: source === 'local' ? location.trim() : undefined,
        url: source === 'github' ? location.trim() : undefined,
        slug: slug.trim() || undefined,
        shared,
      })
      setResult(resp.result)
      onImported(resp.result.slug) // refresh the list now; dialog stays to show warnings
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
        className="flex max-h-[90vh] w-full max-w-lg flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-3 border-b border-[var(--color-border)] px-5 py-4">
          <span className="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--color-surface-2)]">
            <Download size={18} className="text-[var(--color-accent)]" />
          </span>
          <div className="min-w-0 flex-1">
            <h2 className="text-sm font-semibold text-[var(--color-text)]">Claude Code skill içe aktar</h2>
            <p className="text-xs text-[var(--color-text-dim)]">
              Yerel klasör veya github.com klasör URL’sinden — frontmatter eşlenir, uyumsuz özellikler uyarıyla
              ayıklanır.
            </p>
          </div>
        </div>

        <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-5">
          {/* Source toggle */}
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
              {source === 'github' ? 'GitHub klasör URL’si' : 'Yerel klasör yolu'}
            </span>
            <input
              data-testid="import-location"
              value={location}
              onChange={(e) => setLocation(e.target.value)}
              placeholder={
                source === 'github'
                  ? 'https://github.com/owner/repo/tree/main/skills/my-skill'
                  : 'C:\\path\\to\\skill-folder'
              }
              className={`${inputCls} font-mono`}
            />
          </label>

          <div className="flex gap-3">
            <label className="block flex-1">
              <span className="mb-1 block text-xs font-medium text-[var(--color-text-dim)]">Slug (opsiyonel)</span>
              <input
                data-testid="import-slug"
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
                placeholder="isimden türetilir"
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

          {err && (
            <p className="flex items-start gap-1.5 rounded-md bg-[color-mix(in_srgb,var(--color-danger)_12%,transparent)] px-3 py-2 text-xs text-[var(--color-danger)]">
              <AlertTriangle size={14} className="mt-0.5 shrink-0" /> {err}
            </p>
          )}

          {result && (
            <div data-testid="import-result" className="space-y-2 rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] p-3">
              <p className="flex items-center gap-1.5 text-sm font-medium text-[var(--color-success,#16a34a)]">
                <CheckCircle2 size={15} /> İçe aktarıldı: <code>{result.slug}</code>
              </p>
              {result.files.length > 0 && (
                <p className="text-xs text-[var(--color-text-dim)]">
                  <span className="font-medium">Kopyalanan dosyalar:</span> {result.files.join(', ')}
                </p>
              )}
              {result.warnings.length > 0 ? (
                <div className="text-xs text-[var(--color-warning,#d97706)]">
                  <p className="mb-1 flex items-center gap-1.5 font-medium">
                    <AlertTriangle size={13} /> Uyarılar ({result.warnings.length})
                  </p>
                  <ul className="list-disc space-y-0.5 pl-5">
                    {result.warnings.map((w, i) => (
                      <li key={i}>{w}</li>
                    ))}
                  </ul>
                </div>
              ) : (
                <p className="text-xs text-[var(--color-text-dim)]">Uyumsuz özellik yok — temiz içe aktarma.</p>
              )}
            </div>
          )}
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-[var(--color-border)] px-5 py-4">
          <button onClick={onClose} className="rounded px-3 py-1.5 text-sm text-[var(--color-text-dim)] hover:text-[var(--color-text)]">
            {result ? 'Kapat' : 'İptal'}
          </button>
          {!result && (
            <Button data-testid="import-submit" onClick={doImport} disabled={busy}>
              {busy ? 'İçe aktarılıyor…' : 'İçe aktar'}
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}
