import { useEffect, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import { RotateCcw, Trash2, Info, Pencil, X, Workflow, LayoutGrid } from 'lucide-react'
import { api } from '@/api'
import type { Agent, Automation, Flow, BoardColumnDef, AutomationTriggerKind, BoardOp } from '@/types'
import { AgentPicker } from '@/shared/components/agents/AgentPicker'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'
import { Button, TagEditor, LoadingState } from '@/shared/components'
import { normalizeAvatar } from '@/shared/lib/avatar'
import { TargetModeToggle, FlowPicker } from './Schedules'

// Fallback columns used until workspace board columns load (mirrors TaskBoard).
const DEFAULT_COLUMNS: BoardColumnDef[] = [
  { key: 'todo', label: 'Yapılacak', color: '' },
  { key: 'in_progress', label: 'Devam Eden', color: '' },
  { key: 'review', label: 'İnceleme', color: '' },
  { key: 'done', label: 'Bitti', color: '' },
  { key: 'failed', label: 'Başarısız', color: '' },
]

// Board-trigger operation options (label = Turkish UI text).
const BOARD_OPS: { value: BoardOp; label: string }[] = [
  { value: 'move', label: 'Taşındı (sütun değişti)' },
  { value: 'create', label: 'Oluşturuldu' },
  { value: 'update', label: 'Güncellendi' },
  { value: 'delete', label: 'Silindi' },
  { value: 'any', label: 'Herhangi bir değişiklik' },
]

// datetime-local <-> unix-seconds helpers (mirrors Schedules.tsx).
function unixToLocalInput(unix?: number): string {
  if (!unix) return ''
  const d = new Date(unix * 1000)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
function localInputToUnix(s: string): number {
  if (!s) return 0
  const ms = new Date(s).getTime()
  return isNaN(ms) ? 0 : Math.floor(ms / 1000)
}

// Tag-trigger prompt placeholders (kept in sync with agent/automation.go turnVars).
const PROMPT_VARS: { name: string; desc: string }[] = [
  { name: '{{result}}', desc: 'Biten oturumun son yanıtı' },
  { name: '{{title}}', desc: 'Biten oturumun başlığı' },
  { name: '{{tag}}', desc: 'Tetikleyici etiket' },
  { name: '{{sessionId}}', desc: 'Biten oturumun ID’si' },
  { name: '{{iteration}}', desc: 'Bu ateşlemenin sıra no’su (1-tabanlı)' },
  { name: '{{maxIterations}}', desc: 'Üst sınır (0 → ∞)' },
  { name: '{{agent}}', desc: 'Sonucu üreten ajanın adı ({{agentName}} eşdeğer)' },
  { name: '{{prevPrompt}}', desc: 'Bir önceki turu tetikleyen kullanıcı promptu' },
  { name: '{{automation}}', desc: 'Otomasyonun adı' },
  { name: '{{date}}', desc: 'Geçerli tarih (2026-07-03)' },
  { name: '{{time}}', desc: 'Geçerli saat (03:00)' },
  { name: '{{datetime}}', desc: 'Tarih + saat' },
]

// Board-trigger prompt placeholders (kept in sync with agent/automation.go boardVars).
const BOARD_PROMPT_VARS: { name: string; desc: string }[] = [
  { name: '{{taskId}}', desc: 'Değişen kartın ID’si' },
  { name: '{{title}}', desc: 'Kartın başlığı' },
  { name: '{{op}}', desc: 'İşlem (move/create/update/delete)' },
  { name: '{{from}}', desc: 'Önceki sütun anahtarı' },
  { name: '{{to}}', desc: 'Yeni sütun anahtarı' },
  { name: '{{fromLabel}}', desc: 'Önceki sütun adı' },
  { name: '{{toLabel}}', desc: 'Yeni sütun adı' },
  { name: '{{board}}', desc: 'Güncel sütun (= {{to}})' },
  { name: '{{tags}}', desc: 'Kartın etiketleri (virgülle ayrık)' },
  { name: '{{owner}}', desc: 'Atanan ajanın adı (boş = atanmamış)' },
  { name: '{{priority}}', desc: 'Öncelik (critical/high/medium/low, boş olabilir)' },
  { name: '{{iteration}}', desc: 'Bu ateşlemenin sıra no’su (1-tabanlı)' },
  { name: '{{maxIterations}}', desc: 'Üst sınır (0 → ∞)' },
  { name: '{{automation}}', desc: 'Otomasyonun adı' },
  { name: '{{date}}', desc: 'Geçerli tarih' },
  { name: '{{time}}', desc: 'Geçerli saat' },
  { name: '{{datetime}}', desc: 'Tarih + saat' },
]

// Default prompt template for a fresh automation of each kind.
const DEFAULT_PROMPT: Record<AutomationTriggerKind, string> = {
  tag: 'Devam et. Önceki sonuç:\n{{result}}',
  board: 'Bir kart taşındı: {{title}} ({{op}} → {{toLabel}}). Gereğini yap.',
}

// BoardTriggerFields renders the op + source/target column filters for a
// board-triggered automation. Source is shown for move/any/delete, target for
// everything except delete.
function BoardTriggerFields({
  op,
  from,
  to,
  columns,
  onChange,
}: {
  op: BoardOp
  from: string
  to: string
  columns: BoardColumnDef[]
  onChange: (patch: { op?: BoardOp; from?: string; to?: string }) => void
}) {
  const selCls =
    'rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]'
  const showFrom = op === 'move' || op === 'any' || op === 'delete'
  const showTo = op !== 'delete'
  return (
    <>
      <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
        Olay
        <select value={op} onChange={(e) => onChange({ op: e.target.value as BoardOp })} className={selCls}>
          {BOARD_OPS.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </label>
      {showFrom && (
        <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
          Kaynak
          <select value={from} onChange={(e) => onChange({ from: e.target.value })} className={selCls}>
            <option value="">(herhangi)</option>
            {columns.map((c) => (
              <option key={c.key} value={c.key}>
                {c.label}
              </option>
            ))}
          </select>
        </label>
      )}
      {showTo && (
        <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
          Hedef
          <select value={to} onChange={(e) => onChange({ to: e.target.value })} className={selCls}>
            <option value="">(herhangi)</option>
            {columns.map((c) => (
              <option key={c.key} value={c.key}>
                {c.label}
              </option>
            ))}
          </select>
        </label>
      )}
    </>
  )
}

// PromptVarsField renders the prompt-template textarea plus the ℹ️ variable
// picker popover, choosing the variable list by trigger kind.
function PromptVarsField({
  kind,
  value,
  onChange,
}: {
  kind: AutomationTriggerKind
  value: string
  onChange: (next: string) => void
}) {
  const [show, setShow] = useState(false)
  const vars = kind === 'board' ? BOARD_PROMPT_VARS : PROMPT_VARS
  return (
    <div className="relative flex-1">
      <div className="mb-1 flex items-center gap-1 text-[11px] text-[var(--color-text-dim)]">
        <span>Prompt şablonu</span>
        <button
          type="button"
          onClick={() => setShow((v) => !v)}
          className={`rounded p-0.5 transition hover:text-[var(--color-accent)] ${show ? 'text-[var(--color-accent)]' : ''}`}
          title="Kullanılabilir değişkenler"
          aria-label="Kullanılabilir değişkenler"
        >
          <Info size={13} />
        </button>
      </div>
      {show && (
        <>
          <div className="fixed inset-0 z-10" onClick={() => setShow(false)} />
          <div className="absolute bottom-full left-0 z-20 mb-1 w-[360px] max-w-[90vw] rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-2 shadow-lg">
            <div className="mb-1 px-1 text-[11px] font-semibold text-[var(--color-text-dim)]">
              Şablonda kullanılabilir değişkenler (tıkla → ekle)
            </div>
            <div className="max-h-64 overflow-y-auto">
              {vars.map((v) => (
                <button
                  key={v.name}
                  type="button"
                  onClick={() => {
                    onChange(value + v.name)
                    setShow(false)
                  }}
                  className="flex w-full items-baseline gap-2 rounded px-1.5 py-1 text-left transition hover:bg-[var(--color-surface-2)]"
                  title="Şablona ekle"
                >
                  <code className="shrink-0 rounded bg-[var(--color-accent-soft)] px-1 py-0.5 font-mono text-[11px] text-[var(--color-accent)]">
                    {v.name}
                  </code>
                  <span className="text-[11px] text-[var(--color-text-dim)]">{v.desc}</span>
                </button>
              ))}
            </div>
          </div>
        </>
      )}
      <textarea
        value={value}
        onChange={(e) => onChange(e.target.value)}
        rows={2}
        placeholder="Prompt şablonu — ℹ️ ile değişkenleri gör."
        className="w-full resize-y rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
      />
    </div>
  )
}

function fmtTime(unix?: number): string {
  if (!unix) return '—'
  return new Date(unix * 1000).toLocaleString('tr-TR')
}

interface Props {
  agents: Agent[]
  flows: Flow[]
  onError: (msg: string) => void
  // Which section to render: 'tag' or 'board'. null renders nothing (the parent
  // is showing a different tab) — the component stays mounted so its item fetch
  // and counts stay live for the parent's tab badges.
  activeKind: AutomationTriggerKind | null
  // Reports per-kind counts up so the parent tab bar can badge them.
  onCounts?: (counts: { tag: number; board: number }) => void
}

// Automations — event-driven rules of two kinds: 🏷 tag-triggered (a tagged
// session finishing a turn) and 🗂 board-triggered (a kanban card change). It is
// controlled by the parent (Schedules) tab bar via activeKind: it renders the
// matching AutomationSection (own create form / list / inline-edit state) or
// nothing. It owns the shared item + column fetch and reports counts upward.
export function Automations({ agents, flows, onError, activeKind, onCounts }: Props) {
  const [items, setItems] = useState<Automation[]>([])
  // Workspace board columns, for the board-trigger source/target filters.
  const [columns, setColumns] = useState<BoardColumnDef[]>(DEFAULT_COLUMNS)
  // True until the first automation list lands — sections render a loading state
  // rather than claiming there are no automations yet.
  const [loading, setLoading] = useState(true)

  const reload = () =>
    api
      .listAutomations()
      .then(setItems)
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoading(false))

  useEffect(() => {
    reload()
    api
      .getWorkspaceSettings()
      .then((s) => {
        if (s.boardColumns && s.boardColumns.length > 0) setColumns(s.boardColumns)
      })
      .catch(() => {
        /* keep default columns on failure */
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Report per-kind counts up whenever the list changes (for the tab badges).
  useEffect(() => {
    onCounts?.({
      tag: items.filter((a) => (a.triggerKind ?? 'tag') === 'tag').length,
      board: items.filter((a) => (a.triggerKind ?? 'tag') === 'board').length,
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [items])

  if (!activeKind) return null

  const shared = { allItems: items, setItems, reload, agents, flows, columns, loading, onError }
  return <AutomationSection kind={activeKind} {...shared} />
}

interface SectionProps {
  kind: AutomationTriggerKind
  allItems: Automation[]
  setItems: Dispatch<SetStateAction<Automation[]>>
  reload: () => void
  agents: Agent[]
  flows: Flow[]
  columns: BoardColumnDef[]
  loading: boolean
  onError: (msg: string) => void
}

// AutomationSection is one kind-scoped block (tag or board): header, create form,
// and list. It filters the shared item list to its own kind and drives all
// mutations through the parent's setItems/reload so the two sections stay in sync.
function AutomationSection({ kind, allItems, setItems, reload, agents, flows, columns, loading, onError }: SectionProps) {
  const isBoardKind = kind === 'board'
  const items = allItems.filter((a) => (a.triggerKind ?? 'tag') === kind)

  // Create form.
  const [name, setName] = useState('')
  const [triggerTag, setTriggerTag] = useState('')
  const [boardOp, setBoardOp] = useState<BoardOp>('move')
  const [boardFromState, setBoardFromState] = useState('')
  const [boardToState, setBoardToState] = useState('')
  const [targetAgentId, setTargetAgentId] = useState('')
  const [targetMode, setTargetMode] = useState<'agent' | 'flow'>('agent')
  const [flowId, setFlowId] = useState('')
  const [promptTemplate, setPromptTemplate] = useState(DEFAULT_PROMPT[kind])
  const [maxIterations, setMaxIterations] = useState('50')
  const [cooldownSec, setCooldownSec] = useState('0')
  const [expiresAt, setExpiresAt] = useState('')
  // spawnTagsOverride: set by a template (e.g. stuck repair must NOT re-tag the
  // fixer, or it would loop); null = backend default ([triggerTag]).
  const [spawnTagsOverride, setSpawnTagsOverride] = useState<string[] | null>(null)

  // Prefill the create form as a "stuck session repairer" (self-healing,
  // _Docs/56): fires on the failing turn of a session tagged `stuck`, spawns a
  // fixer that diagnoses via the debug journal; on the fixer's success the
  // framework clears the parent's stuck tag + counter, re-opening autonomy.
  const applyStuckTemplate = () => {
    setName('Stuck oturum onarıcısı')
    setTriggerTag('stuck')
    setPromptTemplate(
      'Session {{sessionId}} ("{{title}}") is STUCK: it failed several consecutive turns and its ' +
        'autonomous turns are now suspended. Last error:\n{{result}}\n\n' +
        'Diagnose and fix it:\n' +
        '1. Read its debug journal (read_session_debug with session_id {{sessionId}}) and recent ' +
        'messages (conversation_search) to find the failing tool calls and the root cause.\n' +
        '2. Fix the underlying problem if it is fixable (wrong path/config, missing file, bad state). ' +
        'Check read_lessons for known failure shapes first.\n' +
        '3. Report what you found and what you changed. Do NOT retry the same failing calls blindly.\n' +
        'When you finish successfully, the stuck tag and counter are cleared automatically.',
    )
    setMaxIterations('10')
    setCooldownSec('300')
    setSpawnTagsOverride([]) // the fixer itself must not carry `stuck`
  }

  // Inline edit state (one automation edited at a time, within this section).
  const [editId, setEditId] = useState<string | null>(null)
  const [edit, setEdit] = useState({
    name: '', triggerTag: '', boardOp: 'move' as BoardOp, boardFromState: '', boardToState: '',
    targetAgentId: '', targetMode: 'agent' as 'agent' | 'flow',
    flowId: '', promptTemplate: '', maxIterations: '50', cooldownSec: '0', expiresAt: '',
  })

  const agentName = (id: string) => agents.find((a) => a.id === id)?.name ?? '—'
  const flowName = (id?: string) => flows.find((f) => f.id === id)?.name ?? id ?? '—'
  const flowEmoji = (id?: string) => normalizeAvatar(flows.find((f) => f.id === id)?.emoji)
  const colLabel = (key?: string) => (key ? columns.find((c) => c.key === key)?.label ?? key : '—')

  const create = async () => {
    if (!promptTemplate.trim()) {
      onError('Prompt şablonu zorunlu')
      return
    }
    if (!isBoardKind && !triggerTag.trim()) {
      onError('Tetikleyici etiket zorunlu')
      return
    }
    if (targetMode === 'flow' ? !flowId : !targetAgentId) {
      onError(targetMode === 'flow' ? 'Hedef akış zorunlu' : 'Hedef ajan zorunlu')
      return
    }
    const expUnix = localInputToUnix(expiresAt)
    if (expUnix && expUnix <= Math.floor(Date.now() / 1000)) {
      onError('Son tarih gelecekte olmalı')
      return
    }
    try {
      const a = await api.createAutomation({
        name: name.trim(),
        triggerKind: kind,
        ...(isBoardKind ? { boardOp, boardFromState, boardToState } : { triggerTag: triggerTag.trim() }),
        ...(targetMode === 'flow' ? { flowId } : { targetAgentId }),
        ...(spawnTagsOverride !== null ? { spawnTags: spawnTagsOverride } : {}),
        promptTemplate: promptTemplate.trim(),
        maxIterations: Number(maxIterations) || 0,
        cooldownSec: Number(cooldownSec) || 0,
        expiresAt: expUnix,
        enabled: true,
      })
      setItems((prev) => [a, ...prev])
      setName('')
      setTriggerTag('')
      setBoardFromState('')
      setBoardToState('')
      setExpiresAt('')
      setFlowId('')
      setSpawnTagsOverride(null)
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const startEdit = (a: Automation) => {
    setEditId(a.id)
    setEdit({
      name: a.name ?? '',
      triggerTag: a.triggerTag,
      boardOp: a.boardOp ?? 'move',
      boardFromState: a.boardFromState ?? '',
      boardToState: a.boardToState ?? '',
      targetAgentId: a.targetAgentId,
      targetMode: a.flowId ? 'flow' : 'agent',
      flowId: a.flowId ?? '',
      promptTemplate: a.promptTemplate,
      maxIterations: String(a.maxIterations),
      cooldownSec: String(a.cooldownSec),
      expiresAt: unixToLocalInput(a.expiresAt),
    })
  }

  const cancelEdit = () => setEditId(null)

  const saveEdit = async (a: Automation) => {
    if (!edit.promptTemplate.trim()) {
      onError('Prompt şablonu zorunlu')
      return
    }
    if (!isBoardKind && !edit.triggerTag.trim()) {
      onError('Tetikleyici etiket zorunlu')
      return
    }
    if (edit.targetMode === 'flow' ? !edit.flowId : !edit.targetAgentId) {
      onError(edit.targetMode === 'flow' ? 'Hedef akış zorunlu' : 'Hedef ajan zorunlu')
      return
    }
    const expUnix = localInputToUnix(edit.expiresAt)
    if (expUnix && expUnix <= Math.floor(Date.now() / 1000)) {
      onError('Son tarih gelecekte olmalı')
      return
    }
    try {
      await api.updateAutomation(a.id, {
        name: edit.name.trim(),
        // A full edit always sends the (fixed) kind so the board filters below are
        // re-applied together with it; a partial patch (e.g. spawnTags) omits it.
        triggerKind: kind,
        ...(isBoardKind
          ? { boardOp: edit.boardOp, boardFromState: edit.boardFromState, boardToState: edit.boardToState }
          : { triggerTag: edit.triggerTag.trim() }),
        ...(edit.targetMode === 'flow'
          ? { flowId: edit.flowId, targetAgentId: '' }
          : { targetAgentId: edit.targetAgentId, flowId: '' }),
        promptTemplate: edit.promptTemplate.trim(),
        maxIterations: Number(edit.maxIterations) || 0,
        cooldownSec: Number(edit.cooldownSec) || 0,
        expiresAt: expUnix,
      })
      setEditId(null)
      reload()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const toggle = async (a: Automation) => {
    setItems((prev) => prev.map((x) => (x.id === a.id ? { ...x, enabled: !x.enabled } : x)))
    try {
      await api.toggleAutomation(a.id, !a.enabled)
      reload() // toggling on resets the counter server-side; resync
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }

  const reset = async (a: Automation) => {
    try {
      await api.resetAutomation(a.id)
      reload()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const remove = async (a: Automation) => {
    if (!confirm('Otomasyon silinsin mi?')) return
    setItems((prev) => prev.filter((x) => x.id !== a.id))
    try {
      await api.deleteAutomation(a.id)
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }

  const setSpawnTags = async (a: Automation, spawnTags: string[]) => {
    setItems((prev) => prev.map((x) => (x.id === a.id ? { ...x, spawnTags } : x)))
    try {
      await api.updateAutomation(a.id, { spawnTags })
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }

  const accent = isBoardKind ? 'border-l-sky-500' : 'border-l-violet-500'

  return (
    <div>
      <p className="mb-3 text-xs text-[var(--color-text-dim)]">
        {isBoardKind ? (
          <>
            Bir kanban kartı panoda <span className="font-medium">taşındığında</span> (veya oluşturulunca/
            güncellenince/silinince) hedef ajan/akış, kart bağlamıyla çalışır. Olay + kaynak/hedef sütun ile
            filtrelenir; kendini döngülemez (maks. iterasyon / bekleme / aç-kapa ile sınırlı).
          </>
        ) : (
          <>
            Bir <span className="font-mono">tetikleyici etiket</span> taşıyan oturum bir turu bitirdiğinde son
            yanıtı prompt şablonuna işlenir ve hedef ajan/akış çalışır. Yeni oturum aynı etiketi taşıdığından
            döngü kendiliğinden sürer (maks. iterasyon / bekleme / aç-kapa ile sınırlı).
          </>
        )}
      </p>

      {/* Create form */}
      <div className={`mb-4 space-y-2 rounded-lg border border-l-4 border-[var(--color-border)] ${accent} bg-[var(--color-surface)] p-3`}>
        <div className="flex flex-wrap items-center gap-2">
          <TargetModeToggle mode={targetMode} onChange={setTargetMode} />
          {targetMode === 'flow' ? (
            <FlowPicker flows={flows} value={flowId} onChange={setFlowId} />
          ) : (
            <AgentPicker agents={agents} value={targetAgentId} onChange={setTargetAgentId} />
          )}
          {isBoardKind ? (
            <BoardTriggerFields
              op={boardOp}
              from={boardFromState}
              to={boardToState}
              columns={columns}
              onChange={(p) => {
                if (p.op !== undefined) setBoardOp(p.op)
                if (p.from !== undefined) setBoardFromState(p.from)
                if (p.to !== undefined) setBoardToState(p.to)
              }}
            />
          ) : (
            <input
              value={triggerTag}
              onChange={(e) => setTriggerTag(e.target.value)}
              placeholder="tetikleyici etiket (ör. loop)"
              className="w-52 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 font-mono text-sm outline-none focus:border-[var(--color-accent)]"
            />
          )}
          <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
            Maks. iter.
            <input
              type="number"
              min={0}
              value={maxIterations}
              onChange={(e) => setMaxIterations(e.target.value)}
              className="w-16 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none"
              title="0 = sınırsız (dikkat: sonsuz döngü)"
            />
          </label>
          <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
            Bekleme (sn)
            <input
              type="number"
              min={0}
              value={cooldownSec}
              onChange={(e) => setCooldownSec(e.target.value)}
              className="w-16 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none"
              title="İki tetik arası minimum saniye"
            />
          </label>
          <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
            Son tarih (ops.)
            <input
              type="datetime-local"
              value={expiresAt}
              onChange={(e) => setExpiresAt(e.target.value)}
              className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
              title="Bu tarihten sonra otomasyon tetiklenmez (opsiyonel)"
            />
            {expiresAt && (
              <button
                type="button"
                onClick={() => setExpiresAt('')}
                className="text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
                title="Son tarihi temizle"
              >
                <X size={13} />
              </button>
            )}
          </label>
        </div>
        <div className="flex items-end gap-2">
          <PromptVarsField kind={kind} value={promptTemplate} onChange={setPromptTemplate} />
          <Button onClick={create}>+ Otomasyon</Button>
        </div>
        {!isBoardKind && (
          <div className="flex items-center gap-2 text-xs text-[var(--color-text-dim)]">
            Şablon:
            <button
              type="button"
              onClick={applyStuckTemplate}
              className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-0.5 font-medium text-[var(--color-text)] hover:border-[var(--color-accent)]"
              title="Formu 'stuck oturum onarıcısı' olarak doldurur: stuck etiketli (üst üste turları hata veren) oturumun BAŞARISIZ turunda tetiklenir, teşhis+onarım yapan bir fixer başlatır; fixer başarıyla bitince stuck etiketi + sayaç otomatik temizlenir ve otonomi yeniden açılır. Fixer'a stuck etiketi verilmez (döngü olmaz). Hedef ajanı seçmeyi unutma."
            >
              🩹 Stuck oturum onarıcısı
            </button>
            {spawnTagsOverride !== null && (
              <span className="text-[var(--color-warning)]">şablon aktif: fixer oturumu etiketsiz başlar (döngüsüz)</span>
            )}
          </div>
        )}
      </div>

      {/* List */}
      <div className="space-y-2">
        {loading && <LoadingState label="Otomasyonlar yükleniyor…" />}
        {!loading && items.length === 0 && (
          <p className="text-sm text-[var(--color-text-dim)]">
            {isBoardKind ? 'Henüz pano otomasyonu yok.' : 'Henüz etiket otomasyonu yok.'}
          </p>
        )}
        {items.map((a) => {
          const maxed = a.maxIterations > 0 && a.iterationCount >= a.maxIterations
          const expired = a.expiresAt && a.expiresAt <= Math.floor(Date.now() / 1000)
          const boardOpLabel = BOARD_OPS.find((o) => o.value === (a.boardOp || 'move'))?.label ?? a.boardOp

          if (editId === a.id) {
            return (
              <div
                key={a.id}
                className={`space-y-2 rounded-lg border border-l-4 border-[var(--color-accent)] ${accent} bg-[var(--color-surface)] p-3 text-sm`}
              >
                <div className="flex flex-wrap items-center gap-2">
                  <TargetModeToggle mode={edit.targetMode} onChange={(m) => setEdit((s) => ({ ...s, targetMode: m }))} />
                  {edit.targetMode === 'flow' ? (
                    <FlowPicker flows={flows} value={edit.flowId} onChange={(v) => setEdit((s) => ({ ...s, flowId: v }))} />
                  ) : (
                    <AgentPicker agents={agents} value={edit.targetAgentId} onChange={(v) => setEdit((s) => ({ ...s, targetAgentId: v }))} />
                  )}
                  {isBoardKind ? (
                    <BoardTriggerFields
                      op={edit.boardOp}
                      from={edit.boardFromState}
                      to={edit.boardToState}
                      columns={columns}
                      onChange={(p) =>
                        setEdit((s) => ({
                          ...s,
                          ...(p.op !== undefined ? { boardOp: p.op } : {}),
                          ...(p.from !== undefined ? { boardFromState: p.from } : {}),
                          ...(p.to !== undefined ? { boardToState: p.to } : {}),
                        }))
                      }
                    />
                  ) : (
                    <input
                      value={edit.triggerTag}
                      onChange={(e) => setEdit((s) => ({ ...s, triggerTag: e.target.value }))}
                      placeholder="tetikleyici etiket"
                      className="w-52 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 font-mono text-sm outline-none focus:border-[var(--color-accent)]"
                    />
                  )}
                  <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
                    Maks. iter.
                    <input type="number" min={0} value={edit.maxIterations}
                      onChange={(e) => setEdit((s) => ({ ...s, maxIterations: e.target.value }))}
                      className="w-16 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none" />
                  </label>
                  <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
                    Bekleme (sn)
                    <input type="number" min={0} value={edit.cooldownSec}
                      onChange={(e) => setEdit((s) => ({ ...s, cooldownSec: e.target.value }))}
                      className="w-16 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none" />
                  </label>
                  <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
                    Son tarih (ops.)
                    <input type="datetime-local" value={edit.expiresAt}
                      onChange={(e) => setEdit((s) => ({ ...s, expiresAt: e.target.value }))}
                      className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]" />
                    {edit.expiresAt && (
                      <button type="button" onClick={() => setEdit((s) => ({ ...s, expiresAt: '' }))}
                        className="text-[var(--color-text-dim)] hover:text-[var(--color-danger)]" title="Son tarihi temizle">
                        <X size={13} />
                      </button>
                    )}
                  </label>
                </div>
                <PromptVarsField
                  kind={kind}
                  value={edit.promptTemplate}
                  onChange={(next) => setEdit((s) => ({ ...s, promptTemplate: next }))}
                />
                <div className="flex items-center gap-2">
                  <Button onClick={() => saveEdit(a)}>Kaydet</Button>
                  <Button variant="secondary" onClick={cancelEdit}>İptal</Button>
                </div>
              </div>
            )
          }

          return (
            <div
              key={a.id}
              className={`flex items-start gap-3 rounded-lg border border-l-4 border-[var(--color-border)] ${accent} bg-[var(--color-surface)] px-3 py-2 text-sm`}
            >
              {/* Enable toggle on top, agent/flow icon below (stacked vertically). */}
              <div className="flex shrink-0 flex-col items-center gap-2">
                <button
                  onClick={() => toggle(a)}
                  className={`h-4 w-8 flex-shrink-0 rounded-full transition ${
                    a.enabled ? 'bg-[var(--color-accent)]' : 'bg-[var(--color-border)]'
                  }`}
                  title={a.enabled ? 'Etkin' : 'Pasif'}
                >
                  <span className={`block h-4 w-4 rounded-full bg-white transition ${a.enabled ? 'translate-x-4' : ''}`} />
                </button>
                {(() => {
                  if (a.flowId) {
                    const fe = flowEmoji(a.flowId)
                    return (
                      <span
                        className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[var(--color-accent-soft)] text-[var(--color-accent)]"
                        title="Akış tabanlı otomasyon"
                      >
                        {fe ? <span className="text-base leading-none">{fe}</span> : <Workflow size={15} />}
                      </span>
                    )
                  }
                  const owner = agents.find((x) => x.id === a.targetAgentId)
                  return owner ? (
                    <AgentAvatar agent={owner} size={28} />
                  ) : (
                    <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[var(--color-surface-2)] text-[10px] text-[var(--color-text-dim)]">?</span>
                  )
                })()}
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  {isBoardKind ? (
                    <span
                      className="flex items-center gap-1 rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[11px] text-[var(--color-accent)]"
                      title="Pano (kart) tetikleyicili otomasyon"
                    >
                      <LayoutGrid size={11} />
                      {boardOpLabel}
                      {(a.boardFromState || a.boardToState) && (
                        <span className="opacity-80">
                          {' '}
                          ({a.boardFromState ? colLabel(a.boardFromState) : '∗'} →{' '}
                          {a.boardToState ? colLabel(a.boardToState) : '∗'})
                        </span>
                      )}
                    </span>
                  ) : (
                    <span className="rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--color-accent)]">
                      #{a.triggerTag}
                    </span>
                  )}
                  <span className="text-xs text-[var(--color-text-dim)]">
                    → {a.flowId ? `${flowEmoji(a.flowId) ?? '🔀'} ${flowName(a.flowId)}` : agentName(a.targetAgentId)}
                  </span>
                  <span className="ml-auto font-mono text-[10px] text-[var(--color-text-dim)] opacity-60">{a.id}</span>
                </div>
                <div className="mt-1 truncate text-xs text-[var(--color-text-dim)]" title={a.promptTemplate}>
                  {a.promptTemplate}
                </div>
                <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--color-text-dim)]">
                  <span className={maxed ? 'text-[var(--color-danger)]' : ''}>
                    İterasyon: {a.iterationCount}
                    {a.maxIterations > 0 ? ` / ${a.maxIterations}` : ' / ∞'}
                    {maxed && ' (limit doldu)'}
                  </span>
                  <span>Bekleme: {a.cooldownSec}s</span>
                  <span>Son çalışma: {fmtTime(a.lastFiredAt)}</span>
                  {a.expiresAt ? (
                    <span className={expired ? 'text-[var(--color-danger)]' : ''}>
                      Son tarih: {fmtTime(a.expiresAt)}{expired && ' (süresi doldu)'}
                    </span>
                  ) : null}
                  {a.lastError && <span className="text-[var(--color-danger)]">Hata: {a.lastError}</span>}
                </div>
                {isBoardKind ? (
                  <div className="mt-1.5 text-[11px] text-[var(--color-text-dim)] opacity-80">
                    Pano tetikleyicili — bir kart {boardOpLabel?.toLowerCase()} olduğunda çalışır
                    (kendini döngülemez; spawn etiketleri yok sayılır).
                  </div>
                ) : a.flowId ? (
                  <div className="mt-1.5 text-[11px] text-[var(--color-text-dim)] opacity-80">
                    Akış tabanlı — her tetikte akış çalışır (kendini döngülemez; spawn etiketleri yok sayılır).
                  </div>
                ) : (
                  <div className="mt-1.5 flex items-center gap-2">
                    <span className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
                      Spawn etiketleri
                    </span>
                    <TagEditor
                      tags={a.spawnTags ?? [a.triggerTag]}
                      onChange={(tags) => setSpawnTags(a, tags)}
                      placeholder="loop kırmak için boş bırak"
                      className="flex-1 py-1"
                    />
                  </div>
                )}
              </div>
              <div className="flex flex-col items-center gap-2">
                <button
                  onClick={() => startEdit(a)}
                  className="text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
                  title="Düzenle"
                >
                  <Pencil size={15} />
                </button>
                {maxed && (
                  <button
                    onClick={() => reset(a)}
                    className="text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
                    title="Sayacı sıfırla"
                  >
                    <RotateCcw size={15} />
                  </button>
                )}
                <button
                  onClick={() => remove(a)}
                  className="text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
                  title="Sil"
                >
                  <Trash2 size={15} />
                </button>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
