// GoalEditor — the user's review/edit form over a writer-drafted goal. Every
// field the server validates is editable here; the metric pickers are bound to
// the closed catalog and the scope pickers to the workspace's candidates.
import { useMemo, useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { Button, SectionHead } from '@/shared/components'
import type { Goal, GoalCatalog } from '@/types/goal'
import {
  DIRECTION_LABEL,
  KIND_LABEL,
  MODE_HINT,
  MODE_LABEL,
  PRIORITY_LABEL,
  SCOPE_LABEL,
  SURFACE_LABEL,
} from './goalMeta'
import { fromFormState, toFormState, validateForm, type GoalFormState } from './goalForm'

interface Props {
  goal: Goal
  catalog: GoalCatalog
  saving: boolean
  onSave: (goal: Goal) => void
  onCancel: () => void
}

const INPUT =
  'w-full rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2.5 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]'
const LABEL = 'mb-1 block text-xs text-[var(--color-text-dim)]'

export function GoalEditor({ goal, catalog, saving, onSave, onCancel }: Props) {
  const [f, setF] = useState<GoalFormState>(() => toFormState(goal))
  const [touched, setTouched] = useState(false)
  const set = <K extends keyof GoalFormState>(k: K, v: GoalFormState[K]) =>
    setF((s) => ({ ...s, [k]: v }))
  const problem = useMemo(() => validateForm(f, catalog.metrics), [f, catalog.metrics])
  const metricUnit = (key: string) => catalog.metrics.find((m) => m.key === key)?.unit ?? ''

  const submit = () => {
    setTouched(true)
    if (problem) return
    onSave(fromFormState(goal, f))
  }

  return (
    <form
      className="flex flex-col gap-5"
      data-testid="goal-editor"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <section className="grid gap-3 md:grid-cols-2">
        <div className="md:col-span-2">
          <label className={LABEL}>Ad</label>
          <input
            className={INPUT}
            value={f.name}
            maxLength={120}
            onChange={(e) => set('name', e.target.value)}
            data-testid="goal-name"
          />
        </div>
        <div className="md:col-span-2">
          <label className={LABEL}>Özet (tek satır)</label>
          <input
            className={INPUT}
            value={f.summary}
            onChange={(e) => set('summary', e.target.value)}
          />
        </div>
        <div className="md:col-span-2">
          <label className={LABEL}>Açıklama</label>
          <textarea
            className={INPUT}
            rows={3}
            value={f.description}
            onChange={(e) => set('description', e.target.value)}
          />
        </div>
        <div>
          <label className={LABEL}>Tür</label>
          <select
            className={INPUT}
            value={f.kind}
            onChange={(e) => set('kind', e.target.value as GoalFormState['kind'])}
          >
            <option value="">(otomatik)</option>
            {(Object.keys(KIND_LABEL) as (keyof typeof KIND_LABEL)[]).map((k) => (
              <option key={k} value={k}>
                {KIND_LABEL[k]}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className={LABEL}>Öncelik</label>
          <select
            className={INPUT}
            value={f.priority}
            onChange={(e) => set('priority', e.target.value)}
          >
            {[1, 2, 3, 4, 5].map((p) => (
              <option key={p} value={String(p)}>
                {p} · {PRIORITY_LABEL[p]}
              </option>
            ))}
          </select>
        </div>
      </section>

      <section className="flex flex-col gap-3">
        <SectionHead>Ana metrik</SectionHead>
        <div className="grid gap-3 md:grid-cols-[2fr_1fr_1fr]">
          <div>
            <label className={LABEL}>Metrik</label>
            <select
              className={INPUT}
              value={f.primaryMetric}
              data-testid="goal-primary-metric"
              onChange={(e) => {
                const key = e.target.value
                const def = catalog.metrics.find((m) => m.key === key)
                setF((s) => ({
                  ...s,
                  primaryMetric: key,
                  primaryDirection: def?.defaultDirection ?? s.primaryDirection,
                }))
              }}
            >
              <option value="">— seç —</option>
              {catalog.metrics.map((m) => (
                <option key={m.key} value={m.key}>
                  {m.label} · {m.key}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className={LABEL}>Yön</label>
            <select
              className={INPUT}
              value={f.primaryDirection}
              onChange={(e) => set('primaryDirection', e.target.value as 'min' | 'max')}
            >
              <option value="min">{DIRECTION_LABEL.min}</option>
              <option value="max">{DIRECTION_LABEL.max}</option>
            </select>
          </div>
          <div>
            <label className={LABEL}>Hedef değer ({metricUnit(f.primaryMetric) || '—'})</label>
            <input
              className={INPUT}
              value={f.primaryTarget}
              placeholder="isteğe bağlı"
              onChange={(e) => set('primaryTarget', e.target.value)}
            />
          </div>
        </div>
        {f.primaryMetric && (
          <p className="text-xs text-[var(--color-text-dim)]">
            {catalog.metrics.find((m) => m.key === f.primaryMetric)?.hint}
          </p>
        )}
      </section>

      <section className="flex flex-col gap-2">
        <div className="flex items-center justify-between">
          <SectionHead>Guardrail'ler</SectionHead>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={() => set('guardrails', [...f.guardrails, { metric: '', min: '', max: '' }])}
            data-testid="goal-add-guardrail"
          >
            <Plus size={13} /> Ekle
          </Button>
        </div>
        <p className="text-xs text-[var(--color-text-dim)]">
          Ana metrik itilirken bozulmaması gereken sınırlar. "Ucuz" için başarı oranı alt sınırı,
          "hızlı" için maliyet üst sınırı gibi.
        </p>
        {f.guardrails.length === 0 && (
          <p className="text-xs text-[var(--color-warning)]">
            Guardrail yok — tek metrikli hedefler oyuna açıktır (ölçülmeyen kesilir).
          </p>
        )}
        {f.guardrails.map((r, i) => (
          <div key={i} className="grid items-end gap-2 md:grid-cols-[2fr_1fr_1fr_auto]">
            <div>
              <label className={LABEL}>Metrik</label>
              <select
                className={INPUT}
                value={r.metric}
                onChange={(e) =>
                  set(
                    'guardrails',
                    f.guardrails.map((x, j) => (j === i ? { ...x, metric: e.target.value } : x)),
                  )
                }
              >
                <option value="">— seç —</option>
                {catalog.metrics
                  .filter((m) => m.key !== f.primaryMetric)
                  .map((m) => (
                    <option key={m.key} value={m.key}>
                      {m.label} · {m.key}
                    </option>
                  ))}
              </select>
            </div>
            <div>
              <label className={LABEL}>En az ({metricUnit(r.metric) || '—'})</label>
              <input
                className={INPUT}
                value={r.min}
                onChange={(e) =>
                  set(
                    'guardrails',
                    f.guardrails.map((x, j) => (j === i ? { ...x, min: e.target.value } : x)),
                  )
                }
              />
            </div>
            <div>
              <label className={LABEL}>En çok</label>
              <input
                className={INPUT}
                value={r.max}
                onChange={(e) =>
                  set(
                    'guardrails',
                    f.guardrails.map((x, j) => (j === i ? { ...x, max: e.target.value } : x)),
                  )
                }
              />
            </div>
            <button
              type="button"
              title="Kaldır"
              onClick={() =>
                set(
                  'guardrails',
                  f.guardrails.filter((_, j) => j !== i),
                )
              }
              className="mb-1 rounded p-1.5 text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
            >
              <Trash2 size={14} />
            </button>
          </div>
        ))}
      </section>

      <section className="flex flex-col gap-3">
        <SectionHead>Kapsam</SectionHead>
        <p className="text-xs text-[var(--color-text-dim)]">
          Boş bırakılan her liste "tüm workspace" demektir.
        </p>
        <div className="grid gap-3 md:grid-cols-2">
          <ScopePicker
            label={SCOPE_LABEL.recipes}
            options={catalog.candidates.recipes}
            value={f.recipes}
            onChange={(v) => set('recipes', v)}
          />
          <ScopePicker
            label={SCOPE_LABEL.agents}
            options={catalog.candidates.agents}
            value={f.agents}
            onChange={(v) => set('agents', v)}
          />
          <ScopePicker
            label={SCOPE_LABEL.automations}
            options={catalog.candidates.automations}
            value={f.automations}
            onChange={(v) => set('automations', v)}
          />
          <ScopePicker
            label={SCOPE_LABEL.tags}
            options={catalog.candidates.tags.map((t) => ({ id: t, name: t }))}
            value={f.tags}
            onChange={(v) => set('tags', v)}
          />
        </div>
      </section>

      <section className="flex flex-col gap-2">
        <SectionHead>Rubrik</SectionHead>
        <p className="text-xs text-[var(--color-text-dim)]">
          Açık uçlu hedefler için: 0, 0.5 ve 1 puanın neye benzediğini yaz. Ayrı bağlamda çalışan
          yargıç bununla puanlar; tek başına terfi kararı vermez.
        </p>
        <textarea
          className={INPUT}
          rows={4}
          value={f.rubric}
          onChange={(e) => set('rubric', e.target.value)}
          placeholder={f.primaryMetric === 'judge.rubricScore' ? 'zorunlu' : 'isteğe bağlı'}
        />
      </section>

      <section className="flex flex-col gap-3">
        <SectionHead>Politika</SectionHead>
        <div className="grid gap-3 md:grid-cols-3">
          <div>
            <label className={LABEL}>Mod</label>
            <select
              className={INPUT}
              value={f.mode}
              data-testid="goal-mode"
              onChange={(e) => set('mode', e.target.value as GoalFormState['mode'])}
            >
              {(Object.keys(MODE_LABEL) as (keyof typeof MODE_LABEL)[]).map((m) => (
                <option key={m} value={m}>
                  {MODE_LABEL[m]}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className={LABEL}>Cooldown (saat)</label>
            <input
              className={INPUT}
              value={f.cooldownHours}
              placeholder="72"
              onChange={(e) => set('cooldownHours', e.target.value)}
            />
          </div>
          <div>
            <label className={LABEL}>Geçiş için en az koşu</label>
            <input
              className={INPUT}
              value={f.minRuns}
              placeholder="5"
              onChange={(e) => set('minRuns', e.target.value)}
            />
          </div>
        </div>
        <p className="text-xs text-[var(--color-text-dim)]">{MODE_HINT[f.mode]}</p>
        {f.mode === 'auto' && (
          <div className="flex flex-wrap gap-1.5">
            {catalog.autoApplySurfaces.map((s) => {
              const on = f.autoApplySurfaces.includes(s)
              return (
                <button
                  key={s}
                  type="button"
                  aria-pressed={on}
                  onClick={() =>
                    set(
                      'autoApplySurfaces',
                      on ? f.autoApplySurfaces.filter((x) => x !== s) : [...f.autoApplySurfaces, s],
                    )
                  }
                  className={`rounded-full border px-2.5 py-1 text-xs ${
                    on
                      ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                      : 'border-[var(--color-border)] text-[var(--color-text-dim)]'
                  }`}
                >
                  {SURFACE_LABEL[s] ?? s}
                </button>
              )
            })}
          </div>
        )}
      </section>

      <section className="grid gap-3 md:grid-cols-2">
        <div>
          <label className={LABEL}>
            Açık sorular (satır başına bir; etkinleştirmeden önce boşalt)
          </label>
          <textarea
            className={INPUT}
            rows={3}
            value={f.questions}
            onChange={(e) => set('questions', e.target.value)}
            data-testid="goal-questions"
          />
        </div>
        <div>
          <label className={LABEL}>Notlar / varsayımlar</label>
          <textarea
            className={INPUT}
            rows={3}
            value={f.notes}
            onChange={(e) => set('notes', e.target.value)}
          />
        </div>
      </section>

      <div className="flex items-center gap-2 border-t border-[var(--color-border)] pt-3">
        {touched && problem && (
          <span className="text-xs text-[var(--color-danger)]" data-testid="goal-editor-problem">
            {problem}
          </span>
        )}
        <div className="ml-auto flex gap-2">
          <Button type="button" variant="secondary" onClick={onCancel} disabled={saving}>
            Vazgeç
          </Button>
          <Button type="submit" disabled={saving} data-testid="goal-editor-save">
            {saving ? 'Kaydediliyor…' : 'Kaydet'}
          </Button>
        </div>
      </div>
    </form>
  )
}

function ScopePicker({
  label,
  options,
  value,
  onChange,
}: {
  label: string
  options: { id: string; name: string }[]
  value: string[]
  onChange: (v: string[]) => void
}) {
  // Values that are no longer candidates (deleted agent, renamed recipe) stay
  // listed so the user can see and remove them.
  const stale = value.filter((v) => !options.some((o) => o.id === v))
  const all = [...options, ...stale.map((id) => ({ id, name: `${id} (bulunamadı)` }))]
  return (
    <div>
      <label className={LABEL}>{label}</label>
      {all.length === 0 ? (
        <p className="text-xs text-[var(--color-text-dim)]">(aday yok)</p>
      ) : (
        <div className="flex max-h-32 flex-wrap gap-1.5 overflow-y-auto">
          {all.map((o) => {
            const on = value.includes(o.id)
            return (
              <button
                key={o.id}
                type="button"
                aria-pressed={on}
                title={o.id}
                onClick={() => onChange(on ? value.filter((x) => x !== o.id) : [...value, o.id])}
                className={`rounded-full border px-2.5 py-1 text-xs ${
                  on
                    ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                    : 'border-[var(--color-border)] text-[var(--color-text-dim)]'
                }`}
              >
                {o.name || o.id}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}
