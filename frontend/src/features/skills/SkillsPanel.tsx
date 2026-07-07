import { useCallback, useEffect, useMemo, useState } from 'react'
import { useSessionState } from '@/shared/hooks/useSessionState'
import { ChevronDown, ChevronRight, ChevronsDownUp, ChevronsUpDown, FolderInput, Globe, Lock, Pencil, RefreshCw, Sparkles, Trash2 } from 'lucide-react'
import type { Skill, SkillDetail, SkillSource, ToolVisibility } from '@/types'
import { api } from '@/api'
import { VISIBILITY_TIERS, visibilityMeta } from '@/features/tools/toolMeta'
import { Markdown } from '@/shared/components/markdown/Markdown'
import { CopyPathButton } from '@/shared/components/CopyPathButton'
import { RevealButton } from '@/shared/components/RevealButton'
import { SkillEditor } from './SkillEditor'
import { useMultiSelect } from '@/shared/hooks/useMultiSelect'
import { useGroupedList } from '@/shared/hooks/useGroupedList'
import { SelectionBar, SelectionBarButton, ListPane, PaneHeader } from '@/shared/components'
import { NewItemButton, SELECTED_ITEM_CLS, SELECTED_ITEM_RING } from '@/shared/components/SidebarChrome'
import { useCollapsibleList } from '@/shared/hooks/useCollapsibleList'
import { relativeTime, fullDateTime } from '@/shared/lib/time'

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

// Group key for one skill: its `group` field, or the ungrouped bucket.
function skillGroupKey(sk: Skill): string {
  return sk.group?.trim() || UNGROUPED
}

// Order groups: named groups alphabetically (tr) first, ungrouped bucket last.
function sortSkillGroups(a: string, b: string): number {
  if (a === UNGROUPED) return 1
  if (b === UNGROUPED) return -1
  return a.localeCompare(b, 'tr')
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

// skillVisibility resolves a skill's 4-way tier, preferring the backend-computed
// `visibility` and falling back to the raw frontmatter flags for older payloads.
function skillVisibility(sk: Skill): ToolVisibility {
  if (sk.visibility) return sk.visibility
  if (sk.autoSummary === false) return 'hidden'
  if (sk.nameOnly) return 'name-only'
  if (sk.summaryOnly) return 'summary'
  return 'full'
}

// VisibilityChip shows which of the four catalog-visibility tiers a skill is on
// (Tam / Özet / İsim / Gizli) as a single colored chip, sharing VISIBILITY_TIERS
// metadata with the tier selector. Replaces the older per-flag badges so every
// tier — including full/summary — reads at a glance from the list.
function VisibilityChip({ v }: { v: ToolVisibility }) {
  const meta = visibilityMeta(v)
  return (
    <span
      className="rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide"
      style={{
        backgroundColor: `color-mix(in srgb, ${meta.color} 18%, transparent)`,
        color: meta.color,
      }}
      title={meta.hint}
    >
      {meta.label}
    </span>
  )
}

// SkillVisibilitySelector is the 4-way segmented control for a skill's catalog
// visibility tier (Tam / Özet / İsim / Gizli) — the skill counterpart of the
// tools screen's tier selector, sharing the same VISIBILITY_TIERS metadata. One
// click rewrites the skill's frontmatter flags via setSkillVisibility.
function SkillVisibilitySelector({
  value,
  busy,
  onSet,
}: {
  value: ToolVisibility
  busy: boolean
  onSet: (tier: ToolVisibility) => void
}) {
  return (
    <div
      className="inline-flex overflow-hidden rounded-md border border-[var(--color-border)]"
      role="group"
      aria-label="Skill görünürlüğü"
      data-testid="skill-detail-visibility"
    >
      {VISIBILITY_TIERS.map((tier) => {
        const on = value === tier.value
        return (
          <button
            key={tier.value}
            type="button"
            disabled={busy}
            onClick={() => onSet(tier.value)}
            title={tier.hint}
            data-testid={`skill-vis-${tier.value}`}
            className={
              'px-2.5 py-1.5 text-xs transition-colors disabled:opacity-50 ' +
              (on
                ? 'font-medium text-[var(--color-bg)]'
                : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]')
            }
            style={on ? { backgroundColor: tier.color } : undefined}
          >
            {tier.label}
          </button>
        )
      })}
    </div>
  )
}

// SkillsPanel is the two-panel Skills screen: a list of resolved skills on the
// left, the selected skill's full instructions (loaded on demand) on the right.
export function SkillsPanel({ onError }: Props) {
  const [list, setList] = useState<Skill[]>([])
  // Selection persists across screen switches within the session (resets on reload).
  const [activeSlug, setActiveSlug] = useSessionState<string | null>('skills.activeSlug', null)
  const [active, setActive] = useState<SkillDetail | null>(null)
  const [loadingBody, setLoadingBody] = useState(false)
  const [accessBusy, setAccessBusy] = useState(false)
  const [visBusy, setVisBusy] = useState(false)
  const [deleteBusy, setDeleteBusy] = useState(false)
  // True while a bulk visibility-tier change is applying to the selected skills.
  const [bulkVisBusy, setBulkVisBusy] = useState(false)
  // True while a bulk access change (restrict/share) is applying to the selection.
  const [bulkAccessBusy, setBulkAccessBusy] = useState(false)
  // Draft group name + busy flag for the bulk "set group" action on the selection.
  const [bulkGroup, setBulkGroup] = useState('')
  const [bulkGroupBusy, setBulkGroupBusy] = useState(false)
  // Editor overlay: null = closed, otherwise create or edit (with the loaded skill).
  const [editor, setEditor] = useState<{ mode: 'create' | 'edit'; initial?: SkillDetail } | null>(null)
  const { open: listOpen, toggle: toggleList } = useCollapsibleList('tionswarm.skillsListOpen')

  // Skills sorted newest-edited first. Since useGroupedList preserves incoming
  // order within each bucket, feeding it this pre-sorted list makes every group
  // list its skills from most- to least-recently edited (by SKILL.md mtime).
  const sortedList = useMemo(
    () => [...list].sort((a, b) => (b.modifiedAt ?? 0) - (a.modifiedAt ?? 0)),
    [list],
  )

  // Skills bucketed by group (named groups first, ungrouped last), with
  // persisted per-group collapse state. Recomputed only when the catalog changes.
  const {
    groups: grouped,
    collapsed,
    toggle: toggleGroup,
    allCollapsed,
    toggleAll,
  } = useGroupedList(sortedList, {
    keyOf: skillGroupKey,
    sortGroups: sortSkillGroups,
    persistKey: 'tionswarm.skillsCollapsedGroups',
  })
  // Distinct existing group names, offered as editor autocomplete suggestions.
  const groupNames = useMemo(
    () => grouped.map(([name]) => name).filter((n) => n !== UNGROUPED),
    [grouped],
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

  // Set the selected skill's 4-way visibility tier (full | summary | name-only |
  // hidden) — the skill analogue of a tool's context tier. One backend call
  // rewrites the autoSummary/nameOnly/summaryOnly frontmatter flags together.
  const setVisibility = useCallback(
    (tier: ToolVisibility) => {
      if (!active || active.visibility === tier) return
      setVisBusy(true)
      api
        .setSkillVisibility(active.slug, tier)
        .then((sk) => {
          setActive((a) =>
            a ? { ...a, visibility: sk.visibility, autoSummary: sk.autoSummary, nameOnly: sk.nameOnly, summaryOnly: sk.summaryOnly } : a,
          )
          reload()
        })
        .catch((e) => onError((e as Error).message))
        .finally(() => setVisBusy(false))
    },
    [active, reload, onError],
  )

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

  // Multi-select (Ctrl/Cmd+Click, Shift-range) for bulk skill deletion. The
  // ordered id list is the flattened visible (non-collapsed) skill order so a
  // Shift+Click range can cross group boundaries but skips folded groups.
  const sel = useMultiSelect()
  const orderedSlugs = useMemo(
    () => grouped.flatMap(([name, items]) => (collapsed.has(name) ? [] : items.map((s) => s.slug))),
    [grouped, collapsed],
  )
  const bulkDelete = useCallback(() => {
    const slugs = [...sel.selected]
    if (slugs.length === 0) return
    if (!window.confirm(`${slugs.length} beceri silinsin mi? Bu, klasörlerini diskten kaldırır.`)) return
    Promise.all(slugs.map((slug) => api.deleteSkill(slug)))
      .then(() => {
        if (activeSlug && sel.selected.has(activeSlug)) {
          setActive(null)
          setActiveSlug(null)
        }
        sel.clear()
        reload()
      })
      .catch((e) => onError((e as Error).message))
  }, [sel, activeSlug, reload, onError])

  // Bulk-set the catalog-visibility tier (full | summary | name-only | hidden)
  // for every selected skill at once, then refresh the catalog and keep the
  // selection so the user can chain another action. The tier chips update in
  // place after the reload.
  const bulkSetVisibility = useCallback(
    (tier: ToolVisibility) => {
      const slugs = [...sel.selected]
      if (slugs.length === 0) return
      setBulkVisBusy(true)
      Promise.all(slugs.map((slug) => api.setSkillVisibility(slug, tier)))
        .then(() => {
          reload()
          if (activeSlug && sel.selected.has(activeSlug)) {
            api.getSkill(activeSlug).then(setActive).catch(() => {})
          }
        })
        .catch((e) => onError((e as Error).message))
        .finally(() => setBulkVisBusy(false))
    },
    [sel.selected, reload, activeSlug, onError],
  )

  // Bulk-set the access mode (shared/on-demand vs restricted) for every selected
  // skill at once, then refresh the catalog + selected detail. `shared=true` makes
  // them reachable by every agent; `false` restricts them to explicitly assigned
  // agents. Keeps the selection so the user can chain another action.
  const bulkSetAccess = useCallback(
    (shared: boolean) => {
      const slugs = [...sel.selected]
      if (slugs.length === 0) return
      setBulkAccessBusy(true)
      Promise.all(slugs.map((slug) => api.setSkillAccess(slug, shared)))
        .then(() => {
          reload()
          if (activeSlug && sel.selected.has(activeSlug)) {
            api.getSkill(activeSlug).then(setActive).catch(() => {})
          }
        })
        .catch((e) => onError((e as Error).message))
        .finally(() => setBulkAccessBusy(false))
    },
    [sel.selected, reload, activeSlug, onError],
  )

  // Bulk-set the `group` (organisation bucket) of every selected skill at once, so
  // a batch — e.g. a freshly imported pack — lands under one collapsible header
  // without opening each skill. An empty group ungroups them. Keeps the selection
  // so the user can chain another action; the list re-buckets after the reload.
  const bulkSetGroup = useCallback(
    (group: string) => {
      const slugs = [...sel.selected]
      if (slugs.length === 0) return
      setBulkGroupBusy(true)
      Promise.all(slugs.map((slug) => api.setSkillGroup(slug, group)))
        .then(() => {
          setBulkGroup('')
          reload()
          if (activeSlug && sel.selected.has(activeSlug)) {
            api.getSkill(activeSlug).then(setActive).catch(() => {})
          }
        })
        .catch((e) => onError((e as Error).message))
        .finally(() => setBulkGroupBusy(false))
    },
    [sel.selected, reload, activeSlug, onError],
  )

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
    <div className="flex h-full min-h-0 flex-1">
      {/* List — standard ListPane column (full-height sibling, like chat). */}
      <ListPane
        open={listOpen}
        onToggle={toggleList}
        widthKey="tionswarm.skillsListWidth"
        defaultWidth={288}
        label="Skills"
        testId="skills-list-toggle"
        hideRail
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
              data-testid="skills-rescan"
              onClick={rescan}
              title="Diskten yeniden tara"
              className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
            >
              <RefreshCw size={13} /> Tara
            </button>
          </div>
        </div>
        <NewItemButton
          onClick={() => setEditor({ mode: 'create' })}
          label="Yeni Beceri"
          title="Yeni beceri oluştur"
          testId="skills-create"
        />
        <div className="min-h-0 flex-1 overflow-y-auto p-2">
          {list.length === 0 && (
            <div className="flex flex-col items-center gap-2 px-4 py-10 text-center text-sm text-[var(--color-text-dim)]">
              <Sparkles size={28} className="opacity-40" />
              <p>
                Henüz beceri yok. <code>SKILL.md</code> içeren bir klasörü{' '}
                <code>~/.tionswarm/skills/</code> (global) ya da workspace{' '}
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
                          onClick={(e) => {
                            if (sel.handleClick(e, sk.slug, orderedSlugs, activeSlug)) return
                            setActiveSlug(sk.slug)
                          }}
                          className={`group flex w-full items-start gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition ${
                            sel.isSelected(sk.slug)
                              ? `${SELECTED_ITEM_CLS} ${SELECTED_ITEM_RING}`
                              : isActive
                                ? SELECTED_ITEM_CLS
                                : 'text-[var(--color-text)] hover:bg-[var(--color-surface-2)]'
                          }`}
                        >
                          <span className="mt-0.5 shrink-0 text-base leading-none">{sk.icon || '✨'}</span>
                          <span className="min-w-0 flex-1">
                            <span className="flex items-center gap-1.5">
                              <span className="min-w-0 flex-1 truncate font-medium">{sk.name}</span>
                              {!sk.shared && <RestrictedBadge />}
                              <VisibilityChip v={skillVisibility(sk)} />
                              <SourceBadge source={sk.source} />
                            </span>
                            <span className="mt-0.5 block truncate text-[11px] text-[var(--color-text-dim)]">
                              {sk.description || sk.slug}
                            </span>
                            {sk.modifiedAt ? (
                              <span
                                className="mt-0.5 block text-[10px] text-[var(--color-text-dim)] opacity-70"
                                title={`Son düzenleme: ${fullDateTime(sk.modifiedAt)}`}
                              >
                                Düzenlendi: {relativeTime(sk.modifiedAt)}
                              </span>
                            ) : null}
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

        <SelectionBar
          count={sel.count}
          onClear={sel.clear}
          onSelectAll={orderedSlugs.length ? () => sel.selectAll(orderedSlugs) : undefined}
        >
          {/* Bulk tier: set the visibility of every selected skill at once. */}
          <div
            className="inline-flex overflow-hidden rounded-md border border-[var(--color-border)]"
            role="group"
            aria-label="Seçili becerilerin görünürlüğü"
            data-testid="skills-bulk-visibility"
          >
            {VISIBILITY_TIERS.map((tier) => (
              <button
                key={tier.value}
                type="button"
                disabled={bulkVisBusy}
                onClick={() => bulkSetVisibility(tier.value)}
                title={`Seçili becerileri "${tier.label}" yap — ${tier.hint}`}
                data-testid={`skills-bulk-vis-${tier.value}`}
                className="px-2 py-1 text-xs text-[var(--color-text-dim)] transition-colors hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] disabled:opacity-50"
              >
                {tier.label}
              </button>
            ))}
          </div>
          {/* Bulk access: restrict (assigned-only) or share (on-demand for all). */}
          <div
            className="inline-flex overflow-hidden rounded-md border border-[var(--color-border)]"
            role="group"
            aria-label="Seçili becerilerin erişimi"
            data-testid="skills-bulk-access"
          >
            <button
              type="button"
              disabled={bulkAccessBusy}
              onClick={() => bulkSetAccess(false)}
              title="Seçili becerileri kısıtla — yalnız atanan ajanlar kullanabilsin"
              data-testid="skills-bulk-access-restrict"
              className="flex items-center gap-1 px-2 py-1 text-xs text-[var(--color-text-dim)] transition-colors hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] disabled:opacity-50"
            >
              <Lock size={13} /> Kısıtla
            </button>
            <button
              type="button"
              disabled={bulkAccessBusy}
              onClick={() => bulkSetAccess(true)}
              title="Seçili becerileri paylaş — tüm ajanlar gerektiğinde kullanabilsin"
              data-testid="skills-bulk-access-share"
              className="flex items-center gap-1 border-l border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] transition-colors hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] disabled:opacity-50"
            >
              <Globe size={13} /> Paylaş
            </button>
          </div>
          {/* Bulk group: move every selected skill into one organisation bucket. */}
          <div data-testid="skills-bulk-group" className="inline-flex items-center gap-1">
            <input
              list="skills-bulk-group-names"
              value={bulkGroup}
              onChange={(e) => setBulkGroup(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                  bulkSetGroup(bulkGroup.trim())
                }
              }}
              disabled={bulkGroupBusy}
              placeholder="Grup ata…"
              data-testid="skills-bulk-group-input"
              className="w-28 rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-xs text-[var(--color-text)] outline-none focus:border-[var(--color-accent)] disabled:opacity-50"
            />
            <datalist id="skills-bulk-group-names">
              {groupNames.map((n) => (
                <option key={n} value={n} />
              ))}
            </datalist>
            <SelectionBarButton
              icon={<FolderInput size={13} />}
              onClick={() => bulkSetGroup(bulkGroup.trim())}
              disabled={bulkGroupBusy}
              title={bulkGroup.trim() ? `Seçili becerileri "${bulkGroup.trim()}" grubuna taşı` : 'Seçili becerileri grupsuz yap'}
            >
              {bulkGroup.trim() ? 'Ata' : 'Grupsuz'}
            </SelectionBarButton>
          </div>
          <SelectionBarButton icon={<Trash2 size={13} />} onClick={bulkDelete} danger>
            Sil
          </SelectionBarButton>
        </SelectionBar>
      </ListPane>

      {/* Detail */}
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <PaneHeader
          listOpen={listOpen}
          onToggleList={toggleList}
          // Detail view: the top bar hosts the skill name + badges + actions (no
          // redundant "Skills" title / subtitle). The descriptive block (slug,
          // description, when-to-use, tools, sub-skills) stays below in the body.
          // Chips + the Tam/Özet/İsim/Gizli selector always live on their own second
          // row (every width), so the first row stays compact.
          secondaryAlwaysWrap
          title={active ? undefined : 'Skills'}
          titleSlot={
            active ? (
              <span className="flex min-w-0 items-center gap-2">
                <span className="text-lg leading-none">{active.icon || '✨'}</span>
                <h2 className="truncate text-base font-semibold">{active.name}</h2>
              </span>
            ) : undefined
          }
          secondary={
            active ? (
              <>
                {active.group && (
                  <span className="rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-[var(--color-surface-2)] text-[var(--color-text-dim)]">
                    {active.group}
                  </span>
                )}
                {!active.shared && <RestrictedBadge />}
                <VisibilityChip v={skillVisibility(active)} />
                <SourceBadge source={active.source} />
                {/* Push the Tam/Özet/İsim/Gizli selector to the right of the row. */}
                <span className="ml-auto flex">
                  <SkillVisibilitySelector
                    value={active.visibility ?? (active.autoSummary === false ? 'hidden' : active.nameOnly ? 'name-only' : 'full')}
                    busy={visBusy}
                    onSet={setVisibility}
                  />
                </span>
              </>
            ) : undefined
          }
          right={
            active ? (
              <div className="flex flex-wrap items-center gap-2">
                <button
                  data-testid="skill-detail-edit"
                  onClick={() => setEditor({ mode: 'edit', initial: active })}
                  title="Bu beceriyi düzenle (ad, simge, açıklama, içerik)"
                  className="flex items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
                >
                  <Pencil size={14} /> 
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
                <CopyPathButton path={active.dir} label="Yolu kopyala" labelClassName="hidden" />
                <RevealButton
                  testId="skill-detail-reveal"
                  onReveal={reveal}
                  label="Aç"
                  labelClassName="hidden sm:inline"
                  title="Skill klasörünü dosya yöneticisinde aç"
                />
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
            ) : undefined
          }
        />
        {!active ? (
          <div className="flex flex-1 items-center justify-center text-sm text-[var(--color-text-dim)]">
            {loadingBody ? 'Yükleniyor…' : 'Görüntülemek için bir beceri seç.'}
          </div>
        ) : (
          <>
            <div className="flex flex-wrap items-start justify-between gap-3 border-b border-[var(--color-border)] px-5 py-3">
              <div className="min-w-0">
                <p className="text-xs text-[var(--color-text-dim)]">
                  <code>{active.slug}</code>
                  {active.description ? ` · ${active.description}` : ''}
                </p>
                {active.whenToUse && (
                  <p className="mt-1 text-xs text-[var(--color-text-dim)]">
                    <span className="font-medium">Ne zaman:</span> {active.whenToUse}
                  </p>
                )}
                {active.modifiedAt ? (
                  <p className="mt-1 text-xs text-[var(--color-text-dim)]">
                    <span className="font-medium">Son düzenleme:</span> {fullDateTime(active.modifiedAt)}{' '}
                    <span className="opacity-70">({relativeTime(active.modifiedAt)})</span>
                  </p>
                ) : null}
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
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto px-[1px] py-3 md:p-5">
              {active.body ? (
                // Render the skill body inside the same bubble shell used for an
                // assistant message in chat (AssistantTurn), so a skill reads like a
                // message from the agent.
                <div className="w-full min-w-0 rounded-2xl bg-[color-mix(in_srgb,var(--color-surface-2)_65%,var(--color-bg))] px-4 py-3 text-[var(--color-text)]">
                  <Markdown>{active.body}</Markdown>
                </div>
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
