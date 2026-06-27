import { type MouseEvent as ReactMouseEvent, useCallback, useEffect, useMemo, useState } from 'react'
import { ChevronDown, ChevronRight, ChevronsDownUp, ChevronsUpDown, Eye, EyeOff, FolderOpen, Globe, Lock, Pencil, Plus, RefreshCw, Sparkles, Tag, Trash2 } from 'lucide-react'
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

// Label for the bucket holding skills with no `group` set; always rendered last.
const UNGROUPED = 'Grupsuz'

// groupSkills buckets a skill list by its `group` field, preserving the incoming
// (name-sorted) order within each bucket. Returns ordered [groupName, skills]
// pairs: named groups alphabetically first, the ungrouped bucket last.
function groupSkills(list: Skill[]): Array<[string, Skill[]]> {
  const buckets = new Map<string, Skill[]>()
  for (const sk of list) {
    const key = sk.group?.trim() || UNGROUPED
    const arr = buckets.get(key)
    if (arr) arr.push(sk)
    else buckets.set(key, [sk])
  }
  return [...buckets.entries()].sort(([a], [b]) => {
    if (a === UNGROUPED) return 1
    if (b === UNGROUPED) return -1
    return a.localeCompare(b, 'tr')
  })
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

// RestrictedBadge marks a skill that is NOT on-demand: only agents it is
// explicitly assigned to can see/use it. On-demand (shared) skills are the
// default case and carry no badge.
function RestrictedBadge() {
  return (
    <span
      className="rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-[color-mix(in_srgb,var(--color-danger)_18%,transparent)] text-[var(--color-danger)]"
      title="Yalnız atanan ajanlar kullanabilir (atama gerekir)"
    >
      Kısıtlı
    </span>
  )
}

// SummaryOffBadge marks a skill whose summary is NOT auto-injected into every
// agent's prompt (auto-summary disabled). The skill still works when assigned.
function SummaryOffBadge() {
  return (
    <span
      className="rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-[color-mix(in_srgb,var(--color-warning,#d97706)_18%,transparent)] text-[var(--color-warning,#d97706)]"
      title="Özeti her oturuma otomatik eklenmez (yalnızca atanan ajana görünür)"
    >
      Gizli
    </span>
  )
}

// NameOnlyBadge marks a skill advertised as SLUG ONLY in the Available Skills
// block (description + when-to-use suppressed) — the skill analogue of a tool's
// NameOnly tier. The skill stays listed and is discoverable via skill_search.
function NameOnlyBadge() {
  return (
    <span
      className="rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-[color-mix(in_srgb,var(--color-accent)_18%,transparent)] text-[var(--color-accent)]"
      title="Available Skills bloğunda yalnız slug görünür (açıklama+when bastırılır); model skill_search ile keşfeder"
    >
      NameOnly
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
  const [summaryBusy, setSummaryBusy] = useState(false)
  const [nameOnlyBusy, setNameOnlyBusy] = useState(false)
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

  // Collapsed (folded-in) group names, persisted so the layout survives reloads.
  const [collapsed, setCollapsed] = useState<Set<string>>(() => {
    try {
      const raw = localStorage.getItem('swarmgo.skillsCollapsedGroups')
      return new Set(raw ? (JSON.parse(raw) as string[]) : [])
    } catch {
      return new Set()
    }
  })
  useEffect(() => {
    localStorage.setItem('swarmgo.skillsCollapsedGroups', JSON.stringify([...collapsed]))
  }, [collapsed])
  const toggleGroup = useCallback((name: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }, [])

  // Skills bucketed by group (named groups first, ungrouped last). Recomputed
  // only when the catalog changes.
  const grouped = useMemo(() => groupSkills(list), [list])
  // Distinct existing group names, offered as editor autocomplete suggestions.
  const groupNames = useMemo(
    () => grouped.map(([name]) => name).filter((n) => n !== UNGROUPED),
    [grouped],
  )
  // All groups currently folded? Drives the collapse/expand-all toggle.
  const allCollapsed = grouped.length > 0 && grouped.every(([name]) => collapsed.has(name))
  const toggleAll = useCallback(() => {
    setCollapsed(() => (allCollapsed ? new Set() : new Set(grouped.map(([name]) => name))))
  }, [allCollapsed, grouped])

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

  // Toggle whether the selected skill's summary is auto-injected into every
  // agent's prompt; rewrites the SKILL.md frontmatter on disk + refreshes.
  const toggleAutoSummary = useCallback(() => {
    if (!active) return
    setSummaryBusy(true)
    api
      .setSkillAutoSummary(active.slug, active.autoSummary === false)
      .then((sk) => {
        setActive((a) => (a ? { ...a, autoSummary: sk.autoSummary } : a))
        reload()
      })
      .catch((e) => onError((e as Error).message))
      .finally(() => setSummaryBusy(false))
  }, [active, reload, onError])

  // Toggle whether the selected skill is advertised as slug-only (NameOnly) in
  // the Available Skills block; rewrites the SKILL.md frontmatter + refreshes.
  const toggleNameOnly = useCallback(() => {
    if (!active) return
    setNameOnlyBusy(true)
    api
      .setSkillNameOnly(active.slug, !active.nameOnly)
      .then((sk) => {
        setActive((a) => (a ? { ...a, nameOnly: sk.nameOnly } : a))
        reload()
      })
      .catch((e) => onError((e as Error).message))
      .finally(() => setNameOnlyBusy(false))
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
            Skills · {list.length}
          </span>
          <div className="flex items-center gap-1.5">
            {grouped.length > 1 && (
              <button
                data-testid="skills-toggle-all"
                onClick={toggleAll}
                title={allCollapsed ? 'Tüm grupları aç' : 'Tüm grupları katla'}
                className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
              >
                {allCollapsed ? <ChevronsUpDown size={13} /> : <ChevronsDownUp size={13} />}
              </button>
            )}
            <button
              data-testid="skills-create"
              onClick={() => setEditor({ mode: 'create' })}
              title="Yeni beceri oluştur"
              className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
            >
              <Plus size={13} /> Yeni
            </button>
            <button
              data-testid="skills-rescan"
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
          {grouped.map(([groupName, items]) => {
            const isCollapsed = collapsed.has(groupName)
            return (
              <div key={groupName} className="mb-1">
                <button
                  data-testid="skills-group-header"
                  data-group-name={groupName}
                  data-collapsed={isCollapsed}
                  onClick={() => toggleGroup(groupName)}
                  title={isCollapsed ? 'Grubu aç' : 'Grubu katla'}
                  className="flex w-full items-center gap-1.5 rounded-md px-1.5 py-1 text-left text-[11px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
                >
                  {isCollapsed ? <ChevronRight size={13} /> : <ChevronDown size={13} />}
                  <span className="min-w-0 flex-1 truncate">{groupName}</span>
                  <span className="shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] tabular-nums text-[var(--color-text-dim)]">
                    {items.length}
                  </span>
                </button>
                {!isCollapsed && (
                  <div className="mt-1 space-y-1 pl-1.5">
                    {items.map((sk) => {
                      const isActive = sk.slug === activeSlug
                      return (
                        <button
                          key={sk.slug}
                          data-testid="skills-list-item"
                          data-skill-slug={sk.slug}
                          onClick={() => setActiveSlug(sk.slug)}
                          className={`group flex w-full items-start gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition ${
                            isActive
                              ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                              : 'text-[var(--color-text)] hover:bg-[var(--color-surface-2)]'
                          }`}
                        >
                          <span className="mt-0.5 shrink-0 text-base leading-none">{sk.icon || '✨'}</span>
                          <span className="min-w-0 flex-1">
                            <span className="flex items-center gap-1.5">
                              <span className="min-w-0 flex-1 truncate font-medium">{sk.name}</span>
                              {!sk.shared && <RestrictedBadge />}
                              {sk.autoSummary === false && <SummaryOffBadge />}
                              {sk.nameOnly && <NameOnlyBadge />}
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
                )}
              </div>
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
                  {active.group && (
                    <span className="rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-[var(--color-surface-2)] text-[var(--color-text-dim)]">
                      {active.group}
                    </span>
                  )}
                  {!active.shared && <RestrictedBadge />}
                  {active.autoSummary === false && <SummaryOffBadge />}
                  {active.nameOnly && <NameOnlyBadge />}
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
                  data-testid="skill-detail-edit"
                  onClick={() => setEditor({ mode: 'edit', initial: active })}
                  title="Bu beceriyi düzenle (ad, simge, açıklama, içerik)"
                  className="flex items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
                >
                  <Pencil size={14} /> Düzenle
                </button>
                <button
                  data-testid="skill-detail-toggle-access"
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
                <button
                  data-testid="skill-detail-toggle-summary"
                  onClick={toggleAutoSummary}
                  disabled={summaryBusy}
                  title={
                    active.autoSummary === false
                      ? 'Özeti her oturuma ekle: ajanların sistem promptunda otomatik görünsün'
                      : 'Özeti her oturumdan çıkar: otomatik prompta eklenmesin (atanan ajana yine görünür)'
                  }
                  className="flex items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] disabled:opacity-50"
                >
                  {active.autoSummary === false ? <EyeOff size={14} /> : <Eye size={14} />}
                  {active.autoSummary === false ? 'Özeti aç' : 'Özeti kapat'}
                </button>
                <button
                  data-testid="skill-detail-toggle-nameonly"
                  onClick={toggleNameOnly}
                  disabled={nameOnlyBusy}
                  title={
                    active.nameOnly
                      ? 'NameOnly kapat: özet (açıklama+when) Available Skills bloğunda tekrar görünsün'
                      : 'NameOnly aç: blokta yalnız slug görünsün (açıklama+when bastırılır); model skill_search ile keşfeder'
                  }
                  className="flex items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] disabled:opacity-50"
                >
                  <Tag size={14} />
                  {active.nameOnly ? 'NameOnly kapat' : 'NameOnly'}
                </button>
                <CopyPathButton path={active.dir} />
                <button
                  data-testid="skill-detail-reveal"
                  onClick={reveal}
                  title="Skill klasörünü dosya yöneticisinde aç"
                  className="flex items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
                >
                  <FolderOpen size={14} /> Klasörü aç
                </button>
                <button
                  data-testid="skill-detail-delete"
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
          groups={groupNames}
          onClose={() => setEditor(null)}
          onSaved={onEditorSaved}
        />
      )}
    </div>
  )
}
