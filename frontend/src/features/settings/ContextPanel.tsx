import { useTranslation } from 'react-i18next'
import { SettingsDisclosure } from './SettingsDisclosure'
import { ContextRecovery } from './ContextRecovery'
import { ContextProgress } from './ContextProgress'
import { ContextHandoff } from './ContextHandoff'
import { Layers } from 'lucide-react'
import { InfoPopover } from '@/shared/components/InfoPopover'
import { NumberField, Segmented } from './primitives'
import { SubHead } from './settingsPanelShared'
import type { PanelProps } from './settingsPanelShared'

// Package defaults mirrored from the backend so the live preview matches the
// engine exactly: internal/conversation.defaultMaxTokens / defaultBudgetAutoCeil.
const DEFAULT_MAX_TOKENS = 12000
const DEFAULT_CEIL = 262144

// effectiveBudget mirrors internal/conversation.EffectiveBudget (budget.go):
// clamp(window × fraction, floor, ceil). It also reports which bound is ACTIVE so
// the preview can explain WHY a value lands where it does — the whole point of the
// fix, since "maks. bağlam token" is a FLOOR (it only lifts) and the confusion is
// that lowering it below the ceil-capped derived value changes nothing. autoFraction
// mirrors providers.AdaptiveBudgetFraction for the representative model family.
function effectiveBudget(
  window: number,
  floor: number,
  fraction: number,
  ceil: number,
  autoFraction: number,
): { value: number; bound: 'ceil' | 'floor' | 'fraction' } {
  const cfg = floor > 0 ? floor : DEFAULT_MAX_TOKENS
  const cl = ceil > 0 ? ceil : DEFAULT_CEIL
  const f = fraction > 0 ? fraction : autoFraction
  let derived = Math.floor(window * f)
  let bound: 'ceil' | 'floor' | 'fraction' = 'fraction'
  if (derived > cl) {
    derived = cl
    bound = 'ceil'
  }
  if (derived < cfg) {
    derived = cfg
    bound = 'floor'
  }
  return { value: derived, bound }
}

const fmtK = (n: number) => `${(n / 1000).toFixed(1)}K`
const boundLabelKey: Record<'ceil' | 'floor' | 'fraction', string> = {
  ceil: 'context.bound.ceil',
  floor: 'context.bound.floor',
  fraction: 'context.bound.fraction',
}

export function ContextPanel({ draft, set, setDraft }: PanelProps) {
  const { t } = useTranslation('settingsMain')
  // Two representative windows so the clamp is tangible: a 1M model (Opus/Sonnet/
  // Fable, auto-fraction 0.45) and a 200K model (Haiku, 0.40). Computed live from
  // the current draft values.
  const previews = [
    {
      label: t('context.preview.model1m'),
      ...effectiveBudget(
        1_000_000,
        draft.maxContextTokens,
        draft.contextBudgetFraction,
        draft.contextBudgetCeil,
        0.45,
      ),
    },
    {
      label: t('context.preview.model200k'),
      ...effectiveBudget(
        200_000,
        draft.maxContextTokens,
        draft.contextBudgetFraction,
        draft.contextBudgetCeil,
        0.4,
      ),
    },
  ]
  return (
    <>
      <SubHead icon={Layers}>{t('context.heading')}</SubHead>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <NumberField
          label={t('context.floor.label')}
          hint={t('context.floor.hint')}
          min={0}
          value={draft.maxContextTokens}
          onChange={(v) => set('maxContextTokens', v)}
        />
        <NumberField
          label={t('context.recent.label')}
          hint={t('context.recent.hint')}
          min={0}
          value={draft.keepRecentMsgs}
          onChange={(v) => set('keepRecentMsgs', v)}
        />
        <NumberField
          label={t('context.ceil.label')}
          hint={t('context.ceil.hint')}
          min={0}
          value={draft.contextBudgetCeil}
          onChange={(v) => set('contextBudgetCeil', v)}
        />
        <NumberField
          label={t('context.fraction.label')}
          hint={t('context.fraction.hint')}
          min={0}
          max={1}
          step={0.05}
          value={draft.contextBudgetFraction}
          onChange={(v) => set('contextBudgetFraction', v)}
        />
      </div>
      {/* Live preview: shows the effective budget these three knobs resolve to, per
          representative window, plus WHICH bound is active — so it is obvious why
          lowering the floor below a ceil-capped value changes nothing. The clamp
          formula itself sits behind the heading's (ⓘ). */}
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2">
        <div className="mb-1 flex items-center gap-1 text-[11px] font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
          {t('context.preview.heading')}
          <InfoPopover
            text={
              <>
                {t('context.formula.prefix')}{' '}
                <span className="font-medium">{t('context.formula.floor')}</span>,{' '}
                <span className="font-medium">{t('context.formula.ceil')}</span>
                {t('context.formula.suffix')}
              </>
            }
          />
        </div>
        {previews.map((p) => (
          <div key={p.label} className="flex items-center justify-between gap-2 py-0.5 text-xs">
            <span className="min-w-0 truncate text-[var(--color-text)]">{p.label}</span>
            <span className="shrink-0 font-mono text-[var(--color-text-dim)]">
              <span className="font-semibold text-[var(--color-text)]">{fmtK(p.value)}</span>
              <span
                className={`ml-1.5 ${p.bound === 'ceil' ? 'text-[var(--color-warning)]' : p.bound === 'floor' ? 'text-[var(--color-accent)]' : ''}`}
              >
                · {t(boundLabelKey[p.bound])}
              </span>
            </span>
          </div>
        ))}
      </div>

      <Segmented
        label={t('context.compaction.label')}
        value={draft.autoCompactMode}
        onChange={(v) => set('autoCompactMode', v)}
        options={[
          {
            value: 'rolling',
            label: t('context.compaction.rolling.label'),
            hint: t('context.compaction.rolling.hint'),
          },
          {
            value: 'native',
            label: t('context.compaction.native.label'),
            hint: t('context.compaction.native.hint'),
          },
          {
            value: 'auto',
            label: t('context.compaction.auto.label'),
            hint: t('context.compaction.auto.hint'),
          },
        ]}
      />

      <SettingsDisclosure title={t('context.sections.handoff')}>
        <ContextHandoff draft={draft} set={set} setDraft={setDraft} />
      </SettingsDisclosure>
      <SettingsDisclosure title={t('context.sections.progress')}>
        <ContextProgress draft={draft} set={set} setDraft={setDraft} />
      </SettingsDisclosure>
      <SettingsDisclosure title={t('context.sections.recovery')}>
        <ContextRecovery draft={draft} set={set} setDraft={setDraft} />
      </SettingsDisclosure>
      {/* Self-healing (döngü koruması & ders çıkarma) moved to İçgörü ▸ Dersler. */}
    </>
  )
}
