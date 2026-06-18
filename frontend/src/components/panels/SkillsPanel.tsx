import { type MouseEvent as ReactMouseEvent, useCallback, useEffect, useState } from 'react'
import { FolderOpen, Globe, Lock, Pencil, Plus, RefreshCw, Sparkles, Trash2 } from 'lucide-react'
import type { Skill, SkillDetail, SkillSource } from '../../types'
import { api } from '../../api'
import { Markdown } from '../markdown/Markdown'
import { CopyPathButton } from '../CopyPathButton'
import { SkillEditor } from './SkillEditor'

interface Props {
  onError: (msg: string) => void
}

// Tier badge styling — workspace (higher priority) is accented, global is muted.
// Mirrors the override order on the backend (workspace > global).
const SOURCE_LABEL: Record<SkillSource, string> = {
  global: 'Global',
  workspace: 'Workspace',
}

function SourceBadge({ source }: { source: SkillSource }) {
  const tone =
    source === 'workspace'
      ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
      : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
  return (
    <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide ${tone}`}>
      {SOURCE_LABEL[source]}
    </span>
  )
}

// AccessBadge shows whether a skill is on-demand (every agent sees + can use it)
// or restricted (only agents it is assigned to).
function AccessBadge({ shared }: { shared?: boolean }) {
  return shared ? (
    <span
      className="rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-[color-mix(in_srgb,var(--color-success)_18%,transparent)] text-[var(--color-success)]"
      title="Tüm ajanlar gerektiğinde kullanabilir (atama gerekmez)"
    >
      Gerektiğinde
    </span>
  ) : (
    <span
      className="rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-[var(--color-surface-2)] text-[var(--color-text-dim)]"
      title="Yalnız atanan ajanlar kullanabilir"
    >
      Atanınca
    </span>
  )
}

// SkillsPanel is the two-panel Skills screen: a list of resolved skills on the
// left, the selected skill's full instructions (loaded on demand) on the right.
export function SkillsPanel({ onError }: Props) {
  const [list, setList] = useState<Skill[]>([])
  const [activeSlug, setActiveSlug] = useState<string | null>(null)
  const [active, setActive] = useState<SkillDetail | null>(null)
  const [loadingBody, setLoadingBody] = useState(false)
  const [accessBusy, setAccessBusy] = useState(false)
  const [deleteBusy, setDeleteBusy] = useState(false)
  // Editor overlay: null = closed, otherwise create or edit (with the loaded skill).
  const [editor, setEditor] = useState<{ mode: 'create' | 'edit'; initial?: SkillDetail } | null>(null)
  // Resizable left list width (persisted, clamped). 288px == the old w-72.
  const [listWidth, setListWidth] = useState(() => {
    const v = Number(localStorage.getItem('swarmgo.skillsListWidth'))
    return v >= 200 && v <= 640 ? v : 288
  })
  useEffect(() => {
    localStorage.setItem('swarmgo.skillsListWidth', String(listWidth))
  }, [listWidth])

  // Drag the divider to resize the list panel; tracks the pointer on document so
  // the drag continues even when the cursor leaves the thin handle.
  const startResize = useCallback(
    (e: ReactMouseEvent) => {
      e.preventDefault()
      const startX = e.clientX
      const startW = listWidth
      const onMove = (ev: MouseEvent) =>
        setListWidth(Math.min(640, Math.max(200, startW + ev.clientX - startX)))
      const onUp = () => {
        document.removeEventListener('mousemove', onMove)
        document.removeEventListener('mouseup', onUp)
        document.body.style.cursor = ''
        document.body.style.userSelect = ''
      }
      document.addEventListener('mousemove', onMove)
      document.addEventListener('mouseup', onUp)
      document.body.style.cursor = 'col-resize'
      document.body.style.userSelect = 'none'
    },
    [listWidth],
  )

  const reload = useCallback(() => {
    api
      .listSkills()
      .then((rows) => {
        setList(rows)
        setActiveSlug((cur) => cur ?? rows[0]?.slug ?? null)
      })
      .catch((e) => onError((e as Error).message))
  }, [onError])

  useEffect(() => reload(), [reload])

  // Load the selected skill's full body lazily when the selection changes.
  useEffect(() => {
    if (!activeSlug) {
      setActive(null)
      return
    }
    setLoadingBody(true)
    api
      .getSkill(activeSlug)
      .then(setActive)
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoadingBody(false))
  }, [activeSlug, onError])

  // Open the selected skill's folder in the OS file manager (local desktop app).
  const reveal = useCallback(() => {
    if (!activeSlug) return
    api.revealSkill(activeSlug).catch((e) => onError((e as Error).message))
  }, [activeSlug, onError])

  // Flip the selected skill between shared (on-demand) and restricted; rewrites
  // the SKILL.md frontmatter on disk and refreshes the catalog.
  const toggleAccess = useCallback(() => {
    if (!active) return
    setAccessBusy(true)
    api
      .setSkillAccess(active.slug, !active.shared)
      .then((sk) => {
        setActive((a) => (a ? { ...a, shared: sk.shared } : a))
        reload()
      })
      .catch((e) => onError((e as Error).message))
      .finally(() => setAccessBusy(false))
  }, [active, reload, onError])

  // After the editor saves, refresh the list and focus the saved skill.
  const onEditorSaved = useCallback(
    (saved: SkillDetail) => {
      setEditor(null)
      setActive(saved)
      setActiveSlug(saved.slug)
      reload()
    },
    [reload],
  )

  // Delete the selected skill (confirm first), then refresh + clear selection.
  const removeActive = useCallback(() => {
    if (!active) return
    if (!window.confirm(`"${active.name}" becerisini silmek istediğine emin misin? Bu, klasörünü diskten kaldırır.`)) {
      return
    }
    setDeleteBusy(true)
    api
      .deleteSkill(active.slug)
      .then(() => {
        setActive(null)
        setActiveSlug(null)
        reload()
      })
      .catch((e) => onError((e as Error).message))
      .finally(() => setDeleteBusy(false))
  }, [active, reload, onError])

  // Re-scan tiers on disk, then refresh the catalog + current selection.
  const rescan = useCallback(() => {
    api
      .reloadSkills()
      .then(() => {
        reload()
        if (activeSlug) api.getSkill(activeSlug).then(setActive).catch(() => {})
      })
      .catch((e) => onError((e as Error).message))
  }, [reload, activeSlug, onError])

  return (
    <div className="flex min-h-0 flex-1">
      {/* List (resizable) */}
      <div
        className="flex flex-shrink-0 flex-col"
        style={{ width: listWidth }}
      >
        <div className="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
          <span className="text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
            Beceriler · {list.length}
          </span>
          <div className="flex items-center gap-1.5">
            <button
              onClick={() => setEditor({ mode: 'create' })}
              title="Yeni beceri oluştur"
              className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
            >
              <Plus size={13} /> Yeni
            </button>
            <button
              onClick={rescan}
              title="Diskten yeniden tara"
              className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
            >
              <RefreshCw size={13} /> Tara
            </button>
          </div>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-2">
          {list.length === 0 && (
            <div className="flex flex-col items-center gap-2 px-4 py-10 text-center text-sm text-[var(--color-text-dim)]">
              <Sparkles size={28} className="opacity-40" />
              <p>
                Henüz beceri yok. <code>SKILL.md</code> içeren bir klasörü{' '}
                <code>~/.swarmgo/skills/</code> (global) ya da workspace{' '}
                <code>skills/</code> altına koyup <strong>Tara</strong>'ya bas.
              </p>
            </div>
          )}
          {list.map((sk) => {
            const isActive = sk.slug === activeSlug
            return (
              <button
                key={sk.slug}
                onClick={() => setActiveSlug(sk.slug)}
                className={`group mb-1 flex w-full items-start gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition ${
                  isActive
                    ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                    : 'text-[var(--color-text)] hover:bg-[var(--color-surface-2)]'
                }`}
              >
                <span className="mt-0.5 shrink-0 text-base leading-none">{sk.icon || '✨'}</span>
                <span className="min-w-0 flex-1">
                  <span className="flex items-center gap-1.5">
                    <span className="min-w-0 flex-1 truncate font-medium">{sk.name}</span>
                    {sk.shared && <AccessBadge shared />}
                    <SourceBadge source={sk.source} />
                  </span>
                  <span className="mt-0.5 block truncate text-[11px] text-[var(--color-text-dim)]">
                    {sk.description || sk.slug}
                  </span>
                </span>
              </button>
            )
          })}
        </div>
      </div>

      {/* Resize handle */}
      <div
        onMouseDown={startResize}
        title="Sürükleyerek genişlet"
        className="group relative w-1 shrink-0 cursor-col-resize bg-[var(--color-border)] hover:bg-[var(--color-accent)]"
      >
        <span className="absolute inset-y-0 -left-1 -right-1" />
      </div>

      {/* Detail */}
      <div className="flex min-w-0 flex-1 flex-col">
        {!active ? (
          <div className="flex flex-1 items-center justify-center text-sm text-[var(--color-text-dim)]">
            {loadingBody ? 'Yükleniyor…' : 'Görüntülemek için bir beceri seç.'}
          </div>
        ) : (
          <>
            <div className="flex items-start justify-between gap-3 border-b border-[var(--color-border)] px-5 py-3">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="text-lg leading-none">{active.icon || '✨'}</span>
                  <h2 className="truncate text-base font-semibold">{active.name}</h2>
                  <AccessBadge shared={active.shared} />
                  <SourceBadge source={active.source} />
                </div>
                <p className="mt-1 text-xs text-[var(--color-text-dim)]">
                  <code>{active.slug}</code>
                  {active.description ? ` · ${active.description}` : ''}
                </p>
                {active.whenToUse && (
                  <p className="mt-1 text-xs text-[var(--color-text-dim)]">
                    <span className="font-medium">Ne zaman:</span> {active.whenToUse}
                  </p>
                )}
                {active.alwaysAllow && active.alwaysAllow.length > 0 && (
                  <p className="mt-1 flex flex-wrap items-center gap-1 text-[11px] text-[var(--color-text-dim)]">
                    <span className="font-medium">İzinli araçlar:</span>
                    {active.alwaysAllow.map((tool) => (
                      <code
                        key={tool}
                        className="rounded bg-[var(--color-surface-2)] px-1 py-0.5"
                      >
                        {tool}
                      </code>
                    ))}
                  </p>
                )}
                {active.subSkills && active.subSkills.length > 0 && (
                  <p className="mt-1 flex flex-wrap items-center gap-1 text-[11px] text-[var(--color-text-dim)]">
                    <span className="font-medium" title="use_skill ile gerektiğinde yüklenen daha ayrıntılı beceriler">
                      Alt beceriler:
                    </span>
                    {active.subSkills.map((sub) => {
                      const known = list.some((s) => s.slug === sub)
                      return (
                        <button
                          key={sub}
                          onClick={() => known && setActiveSlug(sub)}
                          disabled={!known}
                          title={known ? `${sub} becerisine git` : `${sub} bulunamadı`}
                          className={`rounded px-1 py-0.5 ${
                            known
                              ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)] hover:underline'
                              : 'bg-[var(--color-surface-2)] line-through opacity-60'
                          }`}
                        >
                          {sub}
                        </button>
                      )
                    })}
                  </p>
                )}
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <button
                  onClick={() => setEditor({ mode: 'edit', initial: active })}
                  title="Bu beceriyi düzenle (ad, simge, açıklama, içerik)"
                  className="flex items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
                >
                  <Pencil size={14} /> Düzenle
                </button>
                <button
                  onClick={toggleAccess}
                  disabled={accessBusy}
                  title={
                    active.shared
                      ? 'Kısıtlıya çevir: yalnız atanan ajanlar kullanabilsin'
                      : 'Paylaşımlı yap: tüm ajanlar gerektiğinde kullanabilsin'
                  }
                  className="flex items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] disabled:opacity-50"
                >
                  {active.shared ? <Lock size={14} /> : <Globe size={14} />}
                  {active.shared ? 'Kısıtla' : 'Paylaş'}
                </button>
                <CopyPathButton path={active.dir} />
                <button
                  onClick={reveal}
                  title="Skill klasörünü dosya yöneticisinde aç"
                  className="flex items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
                >
                  <FolderOpen size={14} /> Klasörü aç
                </button>
                <button
                  onClick={removeActive}
                  disabled={deleteBusy}
                  title="Bu beceriyi sil (klasörünü diskten kaldırır)"
                  className="flex items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)] disabled:opacity-50"
                >
                  <Trash2 size={14} /> Sil
                </button>
              </div>
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto p-5">
              {active.body ? (
                <Markdown>{active.body}</Markdown>
              ) : (
                <p className="text-sm text-[var(--color-text-dim)]">Bu becerinin gövde içeriği yok.</p>
              )}
            </div>
          </>
        )}
      </div>

      {editor && (
        <SkillEditor
          mode={editor.mode}
          initial={editor.initial}
          onClose={() => setEditor(null)}
          onSaved={onEditorSaved}
        />
      )}
    </div>
  )
}
