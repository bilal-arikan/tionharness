// GoalsPanel — the Hedefler screen (_Docs/83 §4.1): the workspace's evolution
// goals as a two-pane list + detail. A goal is only ever CREATED through the
// writer agent (GoalIntake); the user then reviews, edits, activates, pauses or
// archives it here. The detail pane is sectioned so the later phases (fitness
// trend, proposals, evolution ledger) attach under the same header.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Archive, Pencil, RefreshCw, Sparkles, Target, Trash2 } from 'lucide-react'
import { api } from '@/api'
import { Badge, Button, EmptyState, ListPane, PaneHeader } from '@/shared/components'
import { NewItemButton, SELECTED_ITEM_CLS } from '@/shared/components/SidebarChrome'
import { useCollapsibleList } from '@/shared/hooks/useCollapsibleList'
import type { Goal, GoalCatalog, GoalStatus } from '@/types/goal'
import { GoalDetail } from './GoalDetail'
import { GoalEditor } from './GoalEditor'
import { GoalIntake } from './GoalIntake'
import {
  STATUS_ACTION_LABEL,
  STATUS_LABEL,
  STATUS_TONE,
  metricLabel,
  nextStatuses,
  openQuestions,
  scopeSummary,
} from './goalMeta'

interface Props {
  onError: (msg: string) => void
  // Selected goal, deep-link aware (#/w/{ws}/goals/{GOL}).
  goalId?: string | null
  onSelectGoal?: (id: string | null) => void
}

type Filter = 'open' | 'all' | 'archived'

export function GoalsPanel({ onError, goalId, onSelectGoal }: Props) {
  const [goals, setGoals] = useState<Goal[]>([])
  const [catalog, setCatalog] = useState<GoalCatalog | null>(null)
  const [loading, setLoading] = useState(false)
  const [filter, setFilter] = useState<Filter>('open')
  const [localId, setLocalId] = useState<string | null>(null)
  const [intake, setIntake] = useState<{ goal?: Goal | null } | null>(null)
  const [editing, setEditing] = useState(false)
  const [saving, setSaving] = useState(false)
  const { open: listOpen, toggle: toggleList } = useCollapsibleList('tionharness.goalsListOpen')

  const selectedId = onSelectGoal ? (goalId ?? null) : localId
  const select = useCallback(
    (id: string | null) => {
      setLocalId(id)
      onSelectGoal?.(id)
      setEditing(false)
    },
    [onSelectGoal],
  )

  const load = useCallback(() => {
    setLoading(true)
    api
      .listGoals()
      .then(setGoals)
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoading(false))
  }, [onError])

  useEffect(load, [load])
  useEffect(() => {
    api
      .goalCatalog()
      .then(setCatalog)
      .catch((e) => onError((e as Error).message))
  }, [onError])

  const visible = useMemo(
    () =>
      goals.filter((g) =>
        filter === 'all'
          ? true
          : filter === 'archived'
            ? g.status === 'archived'
            : g.status !== 'archived',
      ),
    [goals, filter],
  )
  const active = goals.find((g) => g.id === selectedId) ?? null
  const counts = useMemo(() => {
    const c: Record<GoalStatus, number> = { draft: 0, active: 0, paused: 0, archived: 0 }
    for (const g of goals) c[g.status] = (c[g.status] ?? 0) + 1
    return c
  }, [goals])

  const upsert = (g: Goal) => setGoals((list) => [g, ...list.filter((x) => x.id !== g.id)])

  const setStatus = async (g: Goal, status: GoalStatus) => {
    try {
      upsert(await api.setGoalStatus(g.id, status))
      load()
    } catch (e) {
      onError(e instanceof Error ? e.message : String(e))
    }
  }

  const save = async (g: Goal) => {
    setSaving(true)
    try {
      upsert(await api.updateGoal(g))
      setEditing(false)
      load()
    } catch (e) {
      onError(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  const remove = async (g: Goal) => {
    if (
      !window.confirm(
        `"${g.name}" hedefi kalıcı olarak silinsin mi? Arşivlemek geri alınabilir, silmek değil.`,
      )
    )
      return
    try {
      await api.deleteGoal(g.id)
      setGoals((list) => list.filter((x) => x.id !== g.id))
      if (selectedId === g.id) select(null)
    } catch (e) {
      onError(e instanceof Error ? e.message : String(e))
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-1" data-testid="goals-panel">
      <ListPane
        open={listOpen}
        onToggle={toggleList}
        widthKey="tionharness.goalsListWidth"
        defaultWidth={300}
        label="Hedefler"
        testId="goals-list-toggle"
      >
        <div className="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
          <span className="text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
            Hedefler · {visible.length}
          </span>
          <button
            onClick={load}
            title="Yenile"
            className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
          >
            <RefreshCw size={13} className={loading ? 'animate-spin' : ''} />
          </button>
        </div>
        <NewItemButton
          onClick={() => setIntake({})}
          label="Yeni hedef"
          title="Hedefi kendi sözlerinle yaz; yazıcı ajan taslağa çevirir"
          testId="goals-new"
        />
        <div className="flex gap-1 px-3 pb-2 text-xs">
          {(
            [
              ['open', `Açık · ${counts.draft + counts.active + counts.paused}`],
              ['archived', `Arşiv · ${counts.archived}`],
              ['all', 'Tümü'],
            ] as [Filter, string][]
          ).map(([k, label]) => (
            <button
              key={k}
              onClick={() => setFilter(k)}
              aria-pressed={filter === k}
              className={`rounded-full border px-2 py-0.5 ${
                filter === k
                  ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
                  : 'border-[var(--color-border)] text-[var(--color-text-dim)]'
              }`}
            >
              {label}
            </button>
          ))}
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-2">
          {visible.length === 0 && !loading && (
            <EmptyState icon={Target} title="Henüz hedef yok.">
              "Yeni hedef" ile ne istediğini kendi sözlerinle yaz; hedef yazıcı onu ölçülebilir bir
              taslağa çevirir.
            </EmptyState>
          )}
          {visible.map((g) => {
            const q = openQuestions(g)
            return (
              <button
                key={g.id}
                onClick={() => select(g.id)}
                data-testid={`goal-row-${g.id}`}
                className={`mb-1 flex w-full flex-col gap-0.5 rounded-lg px-3 py-2 text-left transition hover:bg-[var(--color-surface-2)] ${
                  g.id === selectedId ? SELECTED_ITEM_CLS : ''
                }`}
              >
                <span className="flex items-center gap-2">
                  <span className="min-w-0 flex-1 truncate text-sm font-medium">{g.name}</span>
                  <Badge tone={STATUS_TONE[g.status]}>{STATUS_LABEL[g.status]}</Badge>
                </span>
                <span className="truncate text-xs text-[var(--color-text-dim)]">
                  {metricLabel(g.primary.metric, catalog?.metrics)}{' '}
                  {g.primary.direction === 'min' ? '↓' : '↑'} · {scopeSummary(g)}
                  {q > 0 && <span className="text-[var(--color-warning)]"> · {q} soru</span>}
                </span>
              </button>
            )
          })}
        </div>
      </ListPane>

      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <PaneHeader
          listOpen={listOpen}
          onToggleList={toggleList}
          title={active ? undefined : 'Hedefler'}
          subtitle={active ? undefined : 'workspace evrimi neye göre iyileşsin'}
          titleSlot={
            active ? (
              <span className="flex min-w-0 items-center gap-2">
                <Target size={18} className="shrink-0 opacity-70" />
                <h2 className="truncate text-base font-semibold">{active.name}</h2>
                <span className="shrink-0 font-mono text-[11px] text-[var(--color-text-dim)]">
                  {active.id}
                </span>
                <Badge tone={STATUS_TONE[active.status]}>{STATUS_LABEL[active.status]}</Badge>
              </span>
            ) : undefined
          }
          right={
            active && !editing ? (
              <div className="flex flex-wrap items-center gap-1.5">
                {nextStatuses(active.status).map((s) => (
                  <Button
                    key={s}
                    size="sm"
                    variant={s === 'active' ? 'primary' : 'secondary'}
                    onClick={() => void setStatus(active, s)}
                    disabled={s === 'active' && openQuestions(active) > 0}
                    title={
                      s === 'active' && openQuestions(active) > 0
                        ? 'Önce açık soruları boşalt'
                        : STATUS_ACTION_LABEL[s]
                    }
                    data-testid={`goal-status-${s}`}
                  >
                    {s === 'archived' && <Archive size={13} />}
                    {STATUS_ACTION_LABEL[s]}
                  </Button>
                ))}
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => setEditing(true)}
                  data-testid="goal-edit"
                >
                  <Pencil size={13} /> Düzenle
                </Button>
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => setIntake({ goal: active })}
                  title="Yeni bir cümleyle hedef yazıcıya yeniden yazdır (kimlik ve geçmiş korunur)"
                >
                  <Sparkles size={13} /> Yeniden yaz
                </Button>
                <Button
                  size="sm"
                  variant="danger"
                  onClick={() => void remove(active)}
                  title="Kalıcı sil"
                >
                  <Trash2 size={13} />
                </Button>
              </div>
            ) : undefined
          }
        />
        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4 md:px-6">
          {!active ? (
            <EmptyState icon={Target} title="Bir hedef seç ya da yeni hedef yaz.">
              Hedefler doğrudan girilmez: sözlerini hedef yazıcı ajan ölçülebilir bir taslağa
              çevirir, sen onaylarsın. Etkin hedefler ileride evrim geçişlerini (öneri, uygulama,
              geri alma) yönlendirecek.
            </EmptyState>
          ) : editing && catalog ? (
            <GoalEditor
              goal={active}
              catalog={catalog}
              saving={saving}
              onSave={(g) => void save(g)}
              onCancel={() => setEditing(false)}
            />
          ) : (
            <GoalDetail goal={active} catalog={catalog} onError={onError} />
          )}
        </div>
      </div>

      {intake && (
        <GoalIntake
          goal={intake.goal}
          onClose={() => setIntake(null)}
          onError={onError}
          onWritten={(g) => {
            upsert(g)
            setIntake(null)
            setFilter((f) => (f === 'archived' ? 'open' : f))
            select(g.id)
            load()
          }}
        />
      )}
    </div>
  )
}
