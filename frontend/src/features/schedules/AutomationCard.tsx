import {
  RotateCcw,
  Pencil,
  Workflow,
  LayoutGrid,
  Zap,
  Hash,
  Archive,
  MoveRight,
  Waypoints,
  Flag,
} from 'lucide-react'
import type { Agent, Automation, BoardColumnDef, Flow } from '@/types'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'
import { TagEditor } from '@/shared/components'
import { normalizeAvatar } from '@/shared/lib/avatar'
import { CardAction } from './pickers'
import { COLUMN_ACCENT, boardOpLabel, counterMetricLabel } from './automationMeta'
import { fmtTime, isPast } from './timeUtils'
import { AutomationFires } from './AutomationFires'
import { count } from '@/shared/lib/format'

interface Props {
  automation: Automation
  isBoardKind: boolean
  isTokenKind: boolean
  isCounterKind: boolean
  agents: Agent[]
  flows: Flow[]
  columns: BoardColumnDef[]
  onToggle: () => void
  onReset: () => void
  // Archive: hide + stop without deleting (restorable via the API/curator).
  onArchive: () => void
  onEdit: () => void
  onSpawnTags: (tags: string[]) => void
}

// AutomationCard is one event-driven rule rendered as a board card. It serves
// both kinds: the trigger chip on top is either `#tag` (tag column) or the board
// op + column filter (board column). Deleting is not offered here — it lives
// inside the edit popup.
export function AutomationCard({
  automation: a,
  isBoardKind,
  isTokenKind,
  isCounterKind,
  agents,
  flows,
  columns,
  onToggle,
  onReset,
  onArchive,
  onEdit,
  onSpawnTags,
}: Props) {
  const flow = flows.find((f) => f.id === a.flowId)
  const flowIcon = normalizeAvatar(flow?.emoji)
  const owner = agents.find((x) => x.id === a.targetAgentId)
  const colLabel = (key?: string) =>
    key ? (columns.find((c) => c.key === key)?.label ?? key) : '—'
  const maxed = a.maxIterations > 0 && a.iterationCount >= a.maxIterations
  const expired = isPast(a.expiresAt)
  const opLabel = boardOpLabel(a.boardOp)
  const isTargetlessRule = isBoardKind && (a.boardAction === 'archive' || a.boardAction === 'move')
  // Effective session mode (agent-backed only): explicit value wins, else the
  // per-kind default (token/counter → continue). Flow-backed rules ignore it.
  const kind = a.triggerKind ?? 'tag'
  const effectiveMode =
    a.sessionMode ?? (kind === 'token' || kind === 'counter' ? 'continue' : 'spawn')
  const showContinue = !a.flowId && !isTargetlessRule && effectiveMode === 'continue'

  return (
    <div
      data-testid="automation-row"
      data-automation-id={a.id}
      className="rounded-lg border border-l-4 border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-2 text-sm"
      style={{
        borderLeftColor: isBoardKind
          ? COLUMN_ACCENT.board
          : isTokenKind
            ? COLUMN_ACCENT.token
            : isCounterKind
              ? COLUMN_ACCENT.counter
              : kind === 'phase'
                ? COLUMN_ACCENT.phase
                : kind === 'trajectory_end'
                  ? COLUMN_ACCENT.trajectory_end
                  : COLUMN_ACCENT.tag,
      }}
    >
      <div className="flex items-start gap-2">
        <div className="flex shrink-0 flex-col items-center gap-1.5">
          <button
            type="button"
            role="switch"
            aria-checked={a.enabled}
            aria-label={a.enabled ? 'Etkin' : 'Pasif'}
            onClick={onToggle}
            className={`h-4 w-8 rounded-full transition ${
              a.enabled ? 'bg-[var(--color-accent)]' : 'bg-[var(--color-border)]'
            }`}
            title={a.enabled ? 'Etkin' : 'Pasif'}
          >
            <span
              className={`block h-4 w-4 rounded-full bg-[var(--color-text)] transition ${a.enabled ? 'translate-x-4' : ''}`}
            />
          </button>
          {isTargetlessRule ? (
            <span
              className="flex h-7 w-7 items-center justify-center rounded-full bg-[var(--color-surface-2)] text-[var(--color-text-dim)]"
              title="Hedefsiz pano aksiyonu — LLM çağrısı yapılmaz"
            >
              {a.boardAction === 'archive' ? <Archive size={15} /> : <MoveRight size={15} />}
            </span>
          ) : a.flowId ? (
            <span
              className="flex h-7 w-7 items-center justify-center rounded-full bg-[var(--color-accent-soft)] text-[var(--color-accent)]"
              title="Akış tabanlı otomasyon"
            >
              {flowIcon ? (
                <span className="text-base leading-none">{flowIcon}</span>
              ) : (
                <Workflow size={15} />
              )}
            </span>
          ) : owner ? (
            <AgentAvatar agent={owner} size={28} />
          ) : (
            <span className="flex h-7 w-7 items-center justify-center rounded-full bg-[var(--color-surface-2)] text-[10px] text-[var(--color-text-dim)]">
              ?
            </span>
          )}
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-1">
            {isBoardKind ? (
              <span
                className="flex items-center gap-1 rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[11px] text-[var(--color-accent)]"
                title="Pano (kart) tetikleyicili otomasyon"
              >
                <LayoutGrid size={11} />
                {opLabel}
                {(a.boardFromState || a.boardToState) && (
                  <span className="opacity-80">
                    ({a.boardFromState ? colLabel(a.boardFromState) : '∗'} →{' '}
                    {a.boardToState ? colLabel(a.boardToState) : '∗'})
                  </span>
                )}
                {a.boardAction === 'archive' && (
                  <span
                    className="flex items-center gap-0.5 opacity-80"
                    title="Kartı arşivler (LLM yok)"
                  >
                    <Archive size={10} /> arşiv
                  </span>
                )}
                {a.boardAction === 'move' && (
                  <span
                    className="flex items-center gap-0.5 opacity-80"
                    title="Kartı hedef sütuna taşır (LLM yok)"
                  >
                    <MoveRight size={10} /> {colLabel(a.boardMoveToState)}
                  </span>
                )}
              </span>
            ) : isTokenKind ? (
              <span
                className="flex items-center gap-1 rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[11px] text-[var(--color-accent)]"
                title="Token tetikleyicili otomasyon — kümülatif harcama eşiği geçince çalışır"
              >
                <Zap size={11} />
                {a.tokenScope === 'workspace' ? 'workspace' : 'oturum'} · her{' '}
                {count(a.tokenThreshold ?? 0)} token
              </span>
            ) : isCounterKind ? (
              <span
                className="flex items-center gap-1 rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[11px] text-[var(--color-accent)]"
                title="Sayaç tetikleyicili otomasyon — mesaj/tool sayısı aralığı geçince çalışır"
              >
                <Hash size={11} />
                {a.counterScope === 'workspace' ? 'workspace' : 'oturum'} · her{' '}
                {a.counterInterval ?? 0} {a.counterMetric === 'tool' ? 'tool' : 'mesaj'}
              </span>
            ) : kind === 'phase' ? (
              <span
                className="flex items-center gap-1 rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[11px] text-[var(--color-accent)]"
                title="Rota fazı tetikleyicili otomasyon — ilan edilmiş bir faz bitince/başlayınca çalışır"
              >
                <Waypoints size={11} />
                faz {a.trajPhase || '∗'} · {a.trajEvent === 'enter' ? 'başlayınca' : 'bitince'}
                {a.trajRecipe && <span className="opacity-80">· {a.trajRecipe}</span>}
              </span>
            ) : kind === 'trajectory_end' ? (
              <span
                className="flex items-center gap-1 rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[11px] text-[var(--color-accent)]"
                title="Rota sonu tetikleyicili otomasyon — rota terminal duruma gelince çalışır"
              >
                <Flag size={11} />
                rota sonu · {a.trajStatus || 'her bitiş'}
                {a.trajRecipe && <span className="opacity-80">· {a.trajRecipe}</span>}
              </span>
            ) : (
              <span className="rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--color-accent)]">
                #{a.triggerTag}
              </span>
            )}
            {isBoardKind && a.boardExclusive && (
              <span
                className="rounded bg-[color-mix(in_srgb,var(--color-warning)_16%,transparent)] px-1.5 py-0.5 text-[11px] text-[var(--color-text)]"
                title="Tek sahip: eşleşen kart değişiminde yalnız bu otomasyon çalışır, diğer eşleşmeler bastırılır."
              >
                🔒 tek sahip
              </span>
            )}
            {isBoardKind && (a.boardPriority ?? 0) !== 0 && (
              <span
                className="rounded bg-[var(--color-bg)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--color-text-dim)]"
                title="Aynı kart değişimini yakalayan otomasyonlar arasındaki ateşleme sırası (küçük olan önce)."
              >
                sıra {a.boardPriority}
              </span>
            )}
            {showContinue && (
              <span
                className="rounded bg-[var(--color-bg)] px-1.5 py-0.5 text-[11px] text-[var(--color-text-dim)]"
                title="Aynı oturumu sürdürür — ajan her tetikte önceki konuşmayı görür (geçmiş-farkında)."
              >
                🧵 sürdür
              </span>
            )}
          </div>
          {a.name && (
            <div className="mt-0.5 truncate text-[13px] font-medium text-[var(--color-text)]">
              {a.name}
            </div>
          )}
          <div className="truncate text-xs text-[var(--color-text-dim)]">
            → {a.flowId ? `${flowIcon ?? '🔀'} ${flow?.name ?? a.flowId}` : (owner?.name ?? '—')}
          </div>
          <div
            className="mt-1 line-clamp-2 text-xs text-[var(--color-text-dim)]"
            title={a.promptTemplate}
          >
            {a.promptTemplate}
          </div>
        </div>

        {/* Edit (+ counter reset when maxed); deleting lives inside the popup. */}
        <div className="flex shrink-0 flex-col items-center gap-1.5">
          <CardAction icon={Pencil} label="Düzenle" onClick={onEdit} entityId={a.id} />
          <CardAction icon={Archive} label="Arşivle" onClick={onArchive} entityId={a.id} />
          {maxed && (
            <CardAction icon={RotateCcw} label="Sayacı sıfırla" onClick={onReset} entityId={a.id} />
          )}
        </div>
      </div>

      <div className="mt-1.5 flex flex-wrap items-center gap-x-2.5 gap-y-0.5 text-[11px] text-[var(--color-text-dim)]">
        <span className={maxed ? 'text-[var(--color-danger)]' : ''}>
          İter: {a.iterationCount}
          {a.maxIterations > 0 ? ` / ${a.maxIterations}` : ' / ∞'}
          {maxed && ' (doldu)'}
        </span>
        <span>Bekleme: {a.cooldownSec}s</span>
        <span>Son: {fmtTime(a.lastFiredAt)}</span>
        {a.expiresAt ? (
          <span className={expired ? 'text-[var(--color-danger)]' : ''}>
            Son tarih: {fmtTime(a.expiresAt)}
            {expired && ' (doldu)'}
          </span>
        ) : null}
      </div>
      {a.lastError && (
        <div className="mt-0.5 text-[11px] text-[var(--color-danger)]">Hata: {a.lastError}</div>
      )}
      {/* Fire ledger (R5): every attempt with its outcome / skip reason. */}
      <AutomationFires automationId={a.id} refreshKey={a.lastFiredAt} />

      {isBoardKind ? (
        <div className="mt-1 text-[11px] text-[var(--color-text-dim)] opacity-80">
          Pano tetikleyicili — kendini döngülemez (spawn etiketleri yok sayılır).
        </div>
      ) : isTokenKind ? (
        <div className="mt-1 text-[11px] text-[var(--color-text-dim)] opacity-80">
          Token tetikleyicili — kendini döngülemez (spawn etiketleri yok sayılır).
        </div>
      ) : isCounterKind ? (
        <div className="mt-1 text-[11px] text-[var(--color-text-dim)] opacity-80">
          {counterMetricLabel(a.counterMetric)} sayacı — kendini döngülemez (spawn etiketleri yok
          sayılır).
        </div>
      ) : a.flowId ? (
        <div className="mt-1 text-[11px] text-[var(--color-text-dim)] opacity-80">
          Akış tabanlı — kendini döngülemez (spawn etiketleri yok sayılır).
        </div>
      ) : (
        <div className="mt-1">
          <span className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
            Spawn etiketleri
          </span>
          <TagEditor
            tags={a.spawnTags ?? [a.triggerTag]}
            onChange={onSpawnTags}
            placeholder="loop kırmak için boş bırak"
            className="py-1"
          />
        </div>
      )}
    </div>
  )
}
