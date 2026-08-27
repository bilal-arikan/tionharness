import { useCallback, useEffect, useState } from 'react'
import {
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  GitBranch,
  Hourglass,
  Inbox,
  Loader2,
  Network,
  OctagonAlert,
  Play,
  Users,
} from 'lucide-react'
import { api } from '@/api'
import { subscribeWorkerChange } from '@/shared/lib/workerBus'
import type { CoordinatorTree, CoordinatorTreeNode } from '@/types'

interface Props {
  sessionId: string
  // Bumped by the parent when the conversation changes, so the tree refreshes as
  // workers come and go.
  refreshKey?: number
  onSelectSession?: (id: string) => void
}

// formatUSD renders a cost the way the Budget screen does: sub-cent amounts get
// more decimals rather than collapsing to "$0.00", which would read as free.
function formatUSD(v: number): string {
  if (!v) return '$0'
  if (v < 0.01) return `$${v.toFixed(4)}`
  return `$${v.toFixed(2)}`
}

// CoordinatorTreeView renders the WHOLE coordinator tree a session belongs to,
// indented by level, with its rolled-up spend at the top.
//
// The flat worker roster in CoordinatorSection only ever showed a coordinator's
// direct children, which was complete while trees were one level deep. Once a
// worker can be a coordinator, that roster hides everything below the second
// level: the branch a sub-coordinator is running — and most of the tree's cost —
// is simply invisible. This is the view that shows the actual shape.
export function CoordinatorTreeView({ sessionId, refreshKey, onSelectSession }: Props) {
  const [tree, setTree] = useState<CoordinatorTree | null>(null)
  const [loading, setLoading] = useState(false)
  const [open, setOpen] = useState(() => localStorage.getItem('tionharness.coordTreeOpen') === '1')

  const toggle = () =>
    setOpen((v) => {
      const next = !v
      localStorage.setItem('tionharness.coordTreeOpen', next ? '1' : '0')
      return next
    })

  const load = useCallback(() => {
    // Only fetched while the section is expanded: it walks every session in the
    // tree and prices each one, which is not worth doing for a panel nobody is
    // looking at.
    if (!open) return
    setLoading(true)
    api
      .getCoordinatorTree(sessionId)
      .then(setTree)
      .catch(() => setTree(null))
      .finally(() => setLoading(false))
  }, [open, sessionId])

  useEffect(() => {
    load()
  }, [load, refreshKey])

  // Same live signal the roster uses: a worker starting or finishing anywhere
  // under this coordinator changes the tree.
  useEffect(() => {
    if (!open) return
    return subscribeWorkerChange(sessionId, load)
  }, [open, sessionId, load])

  // A tree of one (just this session) says nothing worth a panel.
  const nodes = tree?.nodes ?? []
  const meaningful = nodes.length > 1
  // Broken branches are counted for the collapsed header: a failure deep in the
  // tree is exactly the thing nobody scrolls down to find, so the fold itself has
  // to say it is there.
  const unhealthy = nodes.filter(
    (n) => n.health === 'stuck' || n.health === 'error' || n.stallHalted,
  )

  return (
    <div className="rounded-lg border border-[var(--color-border)] px-2.5 py-2">
      <button
        onClick={toggle}
        aria-expanded={open}
        className="flex w-full items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
      >
        {open ? (
          <ChevronDown size={12} className="shrink-0" />
        ) : (
          <ChevronRight size={12} className="shrink-0" />
        )}
        <GitBranch size={12} className="shrink-0" /> Koordinatör ağacı
        {loading && <Loader2 size={11} className="animate-spin" />}
        {/* Visible even while collapsed — a failure deep in a branch is precisely
            what the user would never expand to discover. */}
        {unhealthy.length > 0 && (
          <span
            title={`${unhealthy.length} oturumda hata/takılma`}
            className="flex items-center gap-0.5 rounded px-1 text-[var(--color-danger)] normal-case tracking-normal"
          >
            <AlertTriangle size={11} className="shrink-0" /> {unhealthy.length}
          </span>
        )}
        {open && meaningful && (
          <span className="ml-auto normal-case tracking-normal text-[var(--color-text-dim)]">
            {nodes.length} oturum · {formatUSD(tree?.totalCostUSD ?? 0)}
          </span>
        )}
      </button>

      {open && (
        <div className="mt-1.5">
          {!meaningful ? (
            <p className="text-[10px] text-[var(--color-text-dim)]">
              Bu oturum henüz bir koordinatör ağacının parçası değil.
            </p>
          ) : (
            <>
              <ul className="space-y-0.5">
                {nodes.map((n) => (
                  <TreeRow
                    key={n.sessionId}
                    node={n}
                    isSelf={n.sessionId === sessionId}
                    onSelectSession={onSelectSession}
                  />
                ))}
              </ul>
              {/* The whole point of the rollup: per-session billing hides what a
                  fan-out actually cost, because the spend sits in descendants. */}
              <div className="mt-1.5 flex items-center justify-between border-t border-[var(--color-border)] pt-1.5 text-[10px] text-[var(--color-text-dim)]">
                <span>Ağaç toplamı</span>
                <span className="font-medium text-[var(--color-text)]">
                  {formatUSD(tree?.totalCostUSD ?? 0)}
                  {tree?.estimated && <span className="ml-1 opacity-60">(tahmini)</span>}
                </span>
              </div>
              {!!tree?.totalSavingsUSD && (
                <div className="flex items-center justify-between text-[10px] text-[var(--color-text-dim)]">
                  <span>Prompt-cache tasarrufu</span>
                  <span className="text-[var(--color-success)]">
                    {formatUSD(tree.totalSavingsUSD)}
                  </span>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </div>
  )
}

// TreeRow is one node, indented by its depth. Nodes arrive breadth-first from the
// root, so indentation alone reproduces the hierarchy — no client-side nesting
// pass, and no risk of the two disagreeing.
function TreeRow({
  node,
  isSelf,
  onSelectSession,
}: {
  node: CoordinatorTreeNode
  isSelf: boolean
  onSelectSession?: (id: string) => void
}) {
  const clickable = !!onSelectSession && !isSelf
  const stuck = node.health === 'stuck'
  const errored = node.health === 'error'
  const halted = !!node.stallHalted
  // Name colour carries the health: a broken node has to be findable by scanning,
  // not by reading each row's trailing icons.
  const nameColor =
    stuck || errored || halted
      ? 'text-[var(--color-danger)]'
      : isSelf
        ? 'text-[var(--color-accent)]'
        : 'text-[var(--color-text)]'
  const healthTitle = halted
    ? 'Durduruldu — hayalet spawn; otomatik turlar durdu, devam ettirilmesi gerekiyor'
    : stuck
      ? 'Takıldı — otonom turlar reddediliyor, müdahale gerekiyor'
      : errored
        ? 'Son turu hata ile bitti'
        : ''
  const body = (
    <span className="flex w-full items-center gap-1.5">
      {node.isCoordinator ? (
        <Users size={11} className="shrink-0 text-[var(--color-accent)]" />
      ) : (
        <Network size={11} className="shrink-0 text-[var(--color-text-dim)]" />
      )}
      <span className={`truncate text-[11px] ${isSelf ? 'font-semibold' : ''} ${nameColor}`}>
        {node.agentName}
      </span>
      {(stuck || halted) && (
        <OctagonAlert size={11} className="shrink-0 text-[var(--color-danger)]" />
      )}
      {errored && !stuck && !halted && (
        <AlertTriangle size={11} className="shrink-0 text-[var(--color-danger)]" />
      )}
      {/* A node that owes its coordinator a report is not broken, but it IS what
          holds the branch above it — worth its own marker, not an error colour. */}
      {node.reportPending && (
        <Hourglass size={10} className="shrink-0 text-[var(--color-text-dim)]" />
      )}
      {node.running && <Play size={10} className="shrink-0 text-[var(--color-accent)]" />}
      {/* A parked send_to_worker follow-up on this node: delivered when its turn
          ends. Mirrors the flat roster's "kuyrukta" badge so a queued message deep
          in the tree is visible from the root. */}
      {node.running && node.queued && (
        <Inbox
          size={10}
          className="shrink-0 text-[var(--color-warning)]"
          aria-label="Bekleyen mesaj"
        />
      )}
      <span className="ml-auto shrink-0 text-[9px] text-[var(--color-text-dim)]">
        {node.costUSD ? formatUSD(node.costUSD) : ''}
      </span>
    </span>
  )
  const title = [
    `${node.title || node.sessionId}`,
    healthTitle,
    node.reportPending ? 'Koordinatörüne raporunu henüz kapatmadı' : '',
    node.running && node.queued ? 'Bekleyen mesaj: bu tur bitince teslim edilecek' : '',
  ]
    .filter(Boolean)
    .join(' — ')
  return (
    <li style={{ paddingLeft: `${node.depth * 12}px` }}>
      {clickable ? (
        <button
          type="button"
          onClick={() => onSelectSession!(node.sessionId)}
          title={title}
          className={`flex w-full items-center rounded px-1 py-0.5 text-left transition hover:bg-[var(--color-surface-2)] ${
            stuck || errored || halted ? 'bg-[var(--color-danger)]/5' : ''
          }`}
        >
          {body}
        </button>
      ) : (
        <div
          title={title}
          className={`flex w-full items-center rounded px-1 py-0.5 ${
            stuck || errored || halted ? 'bg-[var(--color-danger)]/5' : ''
          }`}
        >
          {body}
        </div>
      )}
    </li>
  )
}
