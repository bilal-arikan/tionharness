import { useState } from 'react'
import { Info } from 'lucide-react'
import type {
  AutomationTriggerKind,
  BoardAction,
  BoardColumnDef,
  BoardOp,
  TokenScope,
} from '@/types'
import {
  BOARD_ACTIONS,
  BOARD_OPS,
  BOARD_PROMPT_VARS,
  MIN_TOKEN_THRESHOLD,
  PROMPT_VARS,
  TOKEN_PROMPT_VARS,
  TOKEN_SCOPES,
} from './automationMeta'
import { inputCls } from './pickers'

const selCls =
  'rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]'

// BoardTriggerFields renders the op + source/target column filters for a
// board-triggered automation, plus the arbitration controls (fire order and
// exclusivity) that decide what happens when several rules watch the same
// column. Source is shown for move/any/delete, target for everything except
// delete.
export function BoardTriggerFields({
  op,
  from,
  to,
  priority,
  exclusive,
  action,
  columns,
  onChange,
}: {
  op: BoardOp
  from: string
  to: string
  priority: number
  exclusive: boolean
  action: BoardAction
  columns: BoardColumnDef[]
  onChange: (patch: {
    op?: BoardOp
    from?: string
    to?: string
    priority?: number
    exclusive?: boolean
    action?: BoardAction
  }) => void
}) {
  const showFrom = op === 'move' || op === 'any' || op === 'delete'
  const showTo = op !== 'delete'
  return (
    <div className="flex flex-wrap items-center gap-2">
      <label
        className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]"
        title="Tetiklendiğinde ne yapılır: hedef ajanı/akışı başlat (board yürütmeyi sürer) ya da kartı arşivle (LLM çağrısı yok)."
      >
        Aksiyon
        <select
          value={action}
          onChange={(e) => onChange({ action: e.target.value as BoardAction })}
          className={selCls}
        >
          {BOARD_ACTIONS.map((a) => (
            <option key={a.value} value={a.value}>
              {a.label}
            </option>
          ))}
        </select>
      </label>
      <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
        Olay
        <select
          value={op}
          onChange={(e) => onChange({ op: e.target.value as BoardOp })}
          className={selCls}
        >
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
          <select
            value={from}
            onChange={(e) => onChange({ from: e.target.value })}
            className={selCls}
          >
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
      <label
        className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]"
        title="Aynı kart değişimini yakalayan otomasyonlar arasında ateşleme sırası. Küçük olan önce çalışır."
      >
        Sıra
        <input
          type="number"
          value={priority}
          onChange={(e) => onChange({ priority: Number(e.target.value) || 0 })}
          className={`${selCls} w-16`}
        />
      </label>
      <label
        className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]"
        title="Bu otomasyon eşleşen kart değişimini tek başına sahiplenir; aynı olaya uyan diğer tüm pano otomasyonları bastırılır."
      >
        <input
          type="checkbox"
          checked={exclusive}
          onChange={(e) => onChange({ exclusive: e.target.checked })}
          className="accent-[var(--color-accent)]"
        />
        Tek sahip
      </label>
    </div>
  )
}

// TokenTriggerFields renders the scope selector and the token interval for a
// token-triggered automation. The automation fires each time the watched
// cumulative total crosses another multiple of the interval.
export function TokenTriggerFields({
  scope,
  threshold,
  onChange,
}: {
  scope: TokenScope
  threshold: number
  onChange: (patch: { scope?: TokenScope; threshold?: number }) => void
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
        Kapsam
        <select
          value={scope}
          onChange={(e) => onChange({ scope: e.target.value as TokenScope })}
          className={selCls}
        >
          {TOKEN_SCOPES.map((s) => (
            <option key={s.value} value={s.value}>
              {s.label}
            </option>
          ))}
        </select>
      </label>
      <label
        className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]"
        title="Token aralığı: kümülatif harcama her bu kadar tokenın katını geçtiğinde tetiklenir (ör. 100000 → 100k, 200k…). Token = giriş+çıkış+cache."
      >
        Eşik (token aralığı)
        <input
          type="number"
          min={MIN_TOKEN_THRESHOLD}
          step={1000}
          value={threshold}
          onChange={(e) => onChange({ threshold: Number(e.target.value) || 0 })}
          className={`${selCls} w-28`}
        />
      </label>
    </div>
  )
}

// PromptVarsField renders the prompt-template textarea plus the ℹ️ variable
// picker popover, choosing the variable list by trigger kind.
export function PromptVarsField({
  kind,
  value,
  onChange,
}: {
  kind: AutomationTriggerKind
  value: string
  onChange: (next: string) => void
}) {
  const [show, setShow] = useState(false)
  const vars =
    kind === 'board' ? BOARD_PROMPT_VARS : kind === 'token' ? TOKEN_PROMPT_VARS : PROMPT_VARS
  return (
    <div className="relative">
      <div className="mb-1 flex items-center gap-1 text-[11px] font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
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
        rows={5}
        placeholder="Prompt şablonu — ℹ️ ile değişkenleri gör."
        className={`${inputCls} resize-y`}
      />
    </div>
  )
}
