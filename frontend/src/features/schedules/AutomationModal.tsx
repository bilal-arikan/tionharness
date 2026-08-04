import { useState } from 'react'
import { LayoutGrid, Repeat, X, Zap } from 'lucide-react'
import { api } from '@/api'
import type {
  Agent,
  Automation,
  AutomationTriggerKind,
  BoardAction,
  BoardColumnDef,
  BoardOp,
  Flow,
  TokenScope,
} from '@/types'
import { AgentPicker } from '@/shared/components/agents/AgentPicker'
import {
  COLUMN_ACCENT,
  DEFAULT_MAX_ITERATIONS,
  DEFAULT_PROMPT,
  DEFAULT_TOKEN_THRESHOLD,
  MAX_ITERATIONS_HARD_CAP,
  MIN_TOKEN_THRESHOLD,
  STUCK_TEMPLATE,
} from './automationMeta'
import { BoardTriggerFields, PromptVarsField, TokenTriggerFields } from './AutomationFields'
import { FormModal } from './FormModal'
import { Field, FlowPicker, TargetModeToggle, inputCls } from './pickers'
import { localInputToUnix, unixToLocalInput } from './timeUtils'

interface Props {
  kind: AutomationTriggerKind
  agents: Agent[]
  flows: Flow[]
  columns: BoardColumnDef[]
  /** null = create a new automation; otherwise edit this one. */
  editing: Automation | null
  onClose: () => void
  onSaved: (a: Automation | null, isNew: boolean) => void
  /** Edit mode only: delete this automation (the board confirms and closes). */
  onDelete?: () => void
  onError: (msg: string) => void
}

// AutomationModal is the create/edit popup for both automation kinds (tag and
// board columns). The trigger block swaps by kind; everything else is shared.
export function AutomationModal({
  kind,
  agents,
  flows,
  columns,
  editing,
  onClose,
  onSaved,
  onDelete,
  onError,
}: Props) {
  const isBoardKind = kind === 'board'
  const isTokenKind = kind === 'token'

  const [name, setName] = useState(editing?.name ?? '')
  const [triggerTag, setTriggerTag] = useState(editing?.triggerTag ?? '')
  const [boardOp, setBoardOp] = useState<BoardOp>(editing?.boardOp ?? 'move')
  const [boardFromState, setBoardFromState] = useState(editing?.boardFromState ?? '')
  const [boardToState, setBoardToState] = useState(editing?.boardToState ?? '')
  const [boardPriority, setBoardPriority] = useState(editing?.boardPriority ?? 0)
  const [boardExclusive, setBoardExclusive] = useState(editing?.boardExclusive ?? false)
  const [boardAction, setBoardAction] = useState<BoardAction>(editing?.boardAction ?? 'spawn')
  const [tokenScope, setTokenScope] = useState<TokenScope>(editing?.tokenScope ?? 'session')
  const [tokenThreshold, setTokenThreshold] = useState(
    editing?.tokenThreshold ?? DEFAULT_TOKEN_THRESHOLD,
  )
  const [targetMode, setTargetMode] = useState<'agent' | 'flow'>(editing?.flowId ? 'flow' : 'agent')
  const [targetAgentId, setTargetAgentId] = useState(editing?.targetAgentId ?? '')
  const [flowId, setFlowId] = useState(editing?.flowId ?? '')
  const [promptTemplate, setPromptTemplate] = useState(
    editing?.promptTemplate ?? DEFAULT_PROMPT[kind],
  )
  const [maxIterations, setMaxIterations] = useState(
    String(editing?.maxIterations ?? DEFAULT_MAX_ITERATIONS),
  )
  const [cooldownSec, setCooldownSec] = useState(String(editing?.cooldownSec ?? 0))
  const [expiresAt, setExpiresAt] = useState(unixToLocalInput(editing?.expiresAt))
  // spawnTagsOverride: set by a template (e.g. stuck repair must NOT re-tag the
  // fixer, or it would loop); null = backend default ([triggerTag]).
  const [spawnTagsOverride, setSpawnTagsOverride] = useState<string[] | null>(null)

  const applyStuckTemplate = () => {
    setName(STUCK_TEMPLATE.name)
    setTriggerTag(STUCK_TEMPLATE.triggerTag)
    setPromptTemplate(STUCK_TEMPLATE.promptTemplate)
    setMaxIterations(STUCK_TEMPLATE.maxIterations)
    setCooldownSec(STUCK_TEMPLATE.cooldownSec)
    setSpawnTagsOverride([]) // the fixer itself must not carry `stuck`
  }

  const submit = async () => {
    if (!promptTemplate.trim()) {
      onError('Prompt şablonu zorunlu')
      return
    }
    if (kind === 'tag' && !triggerTag.trim()) {
      onError('Tetikleyici etiket zorunlu')
      return
    }
    if (isTokenKind && tokenThreshold < MIN_TOKEN_THRESHOLD) {
      onError(`Token eşiği en az ${MIN_TOKEN_THRESHOLD} olmalı`)
      return
    }
    // An 'archive' board automation performs bookkeeping with no LLM call, so it
    // needs no target agent/flow. Every other automation must have one.
    const isArchive = isBoardKind && boardAction === 'archive'
    if (!isArchive && (targetMode === 'flow' ? !flowId : !targetAgentId)) {
      onError(targetMode === 'flow' ? 'Hedef akış zorunlu' : 'Hedef ajan zorunlu')
      return
    }
    const expUnix = localInputToUnix(expiresAt)
    if (expUnix && expUnix <= Math.floor(Date.now() / 1000)) {
      onError('Son tarih gelecekte olmalı')
      return
    }
    const trigger = isBoardKind
      ? { boardOp, boardFromState, boardToState, boardPriority, boardExclusive, boardAction }
      : isTokenKind
        ? { tokenScope, tokenThreshold }
        : { triggerTag: triggerTag.trim() }
    // An archive rule carries no target; a spawn rule (and every non-board kind)
    // carries either an agent or a flow.
    const target = isArchive
      ? { targetAgentId: '', flowId: '' }
      : targetMode === 'flow'
        ? { flowId, targetAgentId: '' }
        : { targetAgentId, flowId: '' }
    const body = {
      name: name.trim(),
      // A full edit always sends the (fixed) kind so the board filters below are
      // re-applied together with it; a partial patch (e.g. spawnTags) omits it.
      triggerKind: kind,
      ...trigger,
      ...target,
      promptTemplate: promptTemplate.trim(),
      // NOT `|| 0`: an empty or non-numeric field used to submit 0, which the
      // runtime reads as UNLIMITED — the very value this form forbids. Fall back
      // to the default bound so a blank field can never create a runaway loop.
      maxIterations: Number(maxIterations) || DEFAULT_MAX_ITERATIONS,
      cooldownSec: Number(cooldownSec) || 0,
      expiresAt: expUnix,
    }
    try {
      if (editing) {
        await api.updateAutomation(editing.id, body)
        onSaved(null, false) // caller reloads the list
      } else {
        const created = await api.createAutomation({
          ...body,
          ...(spawnTagsOverride !== null ? { spawnTags: spawnTagsOverride } : {}),
          enabled: true,
        })
        onSaved(created, true)
      }
      onClose()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const kindLabel = isBoardKind
    ? 'pano otomasyonu'
    : isTokenKind
      ? 'token otomasyonu'
      : 'etiket otomasyonu'

  return (
    <FormModal
      title={`${editing ? 'Düzenle' : 'Yeni'} — ${kindLabel}`}
      icon={isBoardKind ? LayoutGrid : isTokenKind ? Zap : Repeat}
      accent={
        isBoardKind ? COLUMN_ACCENT.board : isTokenKind ? COLUMN_ACCENT.token : COLUMN_ACCENT.tag
      }
      submitLabel={editing ? 'Kaydet' : '+ Otomasyon'}
      onSubmit={submit}
      onClose={onClose}
      onDelete={editing ? onDelete : undefined}
      deleteTestId="automation-delete"
      testId={`automation-${kind}-modal`}
    >
      <Field label="Ad (opsiyonel)">
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Otomasyonun adı"
          className={inputCls}
        />
      </Field>

      <Field label="Tetikleyici">
        {isBoardKind ? (
          <BoardTriggerFields
            op={boardOp}
            from={boardFromState}
            to={boardToState}
            priority={boardPriority}
            exclusive={boardExclusive}
            action={boardAction}
            columns={columns}
            onChange={(p) => {
              if (p.op !== undefined) setBoardOp(p.op)
              if (p.from !== undefined) setBoardFromState(p.from)
              if (p.to !== undefined) setBoardToState(p.to)
              if (p.priority !== undefined) setBoardPriority(p.priority)
              if (p.exclusive !== undefined) setBoardExclusive(p.exclusive)
              if (p.action !== undefined) setBoardAction(p.action)
            }}
          />
        ) : isTokenKind ? (
          <TokenTriggerFields
            scope={tokenScope}
            threshold={tokenThreshold}
            onChange={(p) => {
              if (p.scope !== undefined) setTokenScope(p.scope)
              if (p.threshold !== undefined) setTokenThreshold(p.threshold)
            }}
          />
        ) : (
          <input
            value={triggerTag}
            onChange={(e) => setTriggerTag(e.target.value)}
            placeholder="tetikleyici etiket (ör. loop)"
            className={`${inputCls} font-mono`}
          />
        )}
      </Field>

      {isBoardKind && boardAction === 'archive' ? (
        <div className="rounded border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
          Arşiv aksiyonu kartı panodan gizler — hedef ajan/akış ya da prompt gerekmez, LLM çağrısı
          yapılmaz.
        </div>
      ) : (
        <>
          <Field label="Hedef" hint="Tetiklendiğinde çalışacak ajan ya da akış.">
            <div className="flex flex-wrap items-center gap-2">
              <TargetModeToggle mode={targetMode} onChange={setTargetMode} />
              {targetMode === 'flow' ? (
                <FlowPicker flows={flows} value={flowId} onChange={setFlowId} />
              ) : (
                <AgentPicker agents={agents} value={targetAgentId} onChange={setTargetAgentId} />
              )}
            </div>
          </Field>

          <PromptVarsField kind={kind} value={promptTemplate} onChange={setPromptTemplate} />
        </>
      )}

      <div className="flex flex-wrap items-end gap-3">
        <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
          Maks. iter.
          <input
            type="number"
            min={1}
            max={MAX_ITERATIONS_HARD_CAP}
            value={maxIterations}
            onChange={(e) => setMaxIterations(e.target.value)}
            className="w-20 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none"
            title={`1 ile ${MAX_ITERATIONS_HARD_CAP} arası olmalı. 0 (sınırsız) kabul edilmiyor — sonsuz döngü riski. Sunucu da bu aralığı doğrular.`}
          />
        </label>
        <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
          Bekleme (sn)
          <input
            type="number"
            min={0}
            value={cooldownSec}
            onChange={(e) => setCooldownSec(e.target.value)}
            className="w-20 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none"
            title="İki tetik arası minimum saniye"
          />
        </label>
        <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
          Son tarih (ops.)
          <input
            type="datetime-local"
            value={expiresAt}
            onChange={(e) => setExpiresAt(e.target.value)}
            className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
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

      {kind === 'tag' && !editing && (
        <div className="flex flex-wrap items-center gap-2 border-t border-[var(--color-border)] pt-3 text-xs text-[var(--color-text-dim)]">
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
            <span className="text-[var(--color-warning)]">
              şablon aktif: fixer oturumu etiketsiz başlar (döngüsüz)
            </span>
          )}
        </div>
      )}
    </FormModal>
  )
}
