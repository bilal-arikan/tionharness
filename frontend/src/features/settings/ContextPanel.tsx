import { SettingsDisclosure } from './SettingsDisclosure'
import { ContextRecovery } from './ContextRecovery'
import { ContextProgress } from './ContextProgress'
import { ContextHandoff } from './ContextHandoff'
import { Layers } from 'lucide-react'
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
const boundLabel: Record<'ceil' | 'floor' | 'fraction', string> = {
  ceil: 'tavana kırpıldı',
  floor: 'tabana yükseltildi',
  fraction: 'pencere × oran',
}

export function ContextPanel({ draft, set, setDraft }: PanelProps) {
  // Two representative windows so the clamp is tangible: a 1M model (Opus/Sonnet/
  // Fable, auto-fraction 0.45) and a 200K model (Haiku, 0.40). Computed live from
  // the current draft values.
  const previews = [
    {
      label: '1M model (Opus/Sonnet/Fable)',
      ...effectiveBudget(
        1_000_000,
        draft.maxContextTokens,
        draft.contextBudgetFraction,
        draft.contextBudgetCeil,
        0.45,
      ),
    },
    {
      label: '200K model (Haiku)',
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
      <SubHead icon={Layers}>Bağlam penceresi</SubHead>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <NumberField
          label="Maks. bağlam token — TABAN"
          hint="Bütçenin ALT sınırı: bütçeyi yalnızca YÜKSELTİR, asla düşürmez. Türetilen değer (pencere × oran, tavana kırpılı) bunun üstündeyse etkisizdir. Pencereyi KÜÇÜLTMEK için bunu değil, aşağıdaki 'Bütçe tavanı'nı düşür."
          min={0}
          value={draft.maxContextTokens}
          onChange={(v) => set('maxContextTokens', v)}
        />
        <NumberField
          label="Korunan son mesaj"
          hint="Her zaman aynen gönderilir."
          min={0}
          value={draft.keepRecentMsgs}
          onChange={(v) => set('keepRecentMsgs', v)}
        />
        <NumberField
          label="Bütçe tavanı (token) — ÜST SINIR"
          hint="Bütçenin ÜST sınırı ve pencereyi KÜÇÜLTMEK için değiştireceğin ayar BUDUR. Varsayılan 262144 (256K) — ham pencereyi gradyanın yüksek-hassasiyet bölgesinde tutar (context-rot). Düşürürsen etkin bütçe buraya kırpılır; yükseltmek daha çok ham geçmiş tutar ama recall hassasiyetiyle takas eder."
          min={0}
          value={draft.contextBudgetCeil}
          onChange={(v) => set('contextBudgetCeil', v)}
        />
        <NumberField
          label="Pencere oranı"
          hint="Transkripte ayrılan pay (0–1). 0 = otomatik (model-ailesine göre adaptif, önerilen). Pozitif değer sabit pay sabitler (ör. 0.45 → 1M model 450K, tavana kırpılır)."
          min={0}
          max={1}
          step={0.05}
          value={draft.contextBudgetFraction}
          onChange={(v) => set('contextBudgetFraction', v)}
        />
      </div>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">
        Etkin bütçe = clamp(pencere × oran, <span className="font-medium">taban</span>,{' '}
        <span className="font-medium">tavan</span>). Oran 0 = otomatik adaptif; bilinmeyen pencere →
        taban kullanılır.
      </p>

      {/* Live preview: shows the effective budget these three knobs resolve to, per
          representative window, plus WHICH bound is active — so it is obvious why
          lowering the floor below a ceil-capped value changes nothing. */}
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2">
        <div className="mb-1 text-[11px] font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
          Etkin bütçe önizlemesi (girdiğin değerlerle)
        </div>
        {previews.map((p) => (
          <div key={p.label} className="flex items-center justify-between gap-2 py-0.5 text-xs">
            <span className="min-w-0 truncate text-[var(--color-text)]">{p.label}</span>
            <span className="shrink-0 font-mono text-[var(--color-text-dim)]">
              <span className="font-semibold text-[var(--color-text)]">{fmtK(p.value)}</span>
              <span
                className={`ml-1.5 ${p.bound === 'ceil' ? 'text-[var(--color-warning)]' : p.bound === 'floor' ? 'text-[var(--color-accent)]' : ''}`}
              >
                · {boundLabel[p.bound]}
              </span>
            </span>
          </div>
        ))}
      </div>

      <Segmented
        label="Otomatik sıkıştırma modu"
        value={draft.autoCompactMode}
        onChange={(v) => set('autoCompactMode', v)}
        options={[
          {
            value: 'rolling',
            label: 'Rolling (TionHarness)',
            hint: "Varsayılan ve bugünkü davranış: bütçe aşıldığında en eski turlar TionHarness'in kendi rolling özetine katlanır; transkriptin geri kalanı aynen gider. Sıkıştırma tamamen TionHarness'in kontrolündedir.",
          },
          {
            value: 'native',
            label: 'Native (CLI)',
            hint: "Sıkıştırmayı CLI sağlayıcısının kendi native compaction'ına bırakır. BEDELİ: native compaction warm CLI oturumunu düşürür — sonraki tur cold start olur (prompt cache soğur, o tur pahalanır).",
          },
          {
            value: 'auto',
            label: 'Otomatik',
            hint: 'Sağlayıcı native compaction destekliyorsa ve canlı bir warm CLI oturumu varsa native, aksi halde rolling seçilir.',
          },
        ]}
      />

      <SettingsDisclosure title="Context handoff">
        <ContextHandoff draft={draft} set={set} setDraft={setDraft} />
      </SettingsDisclosure>
      <SettingsDisclosure title="Persistent progress">
        <ContextProgress draft={draft} set={set} setDraft={setDraft} />
      </SettingsDisclosure>
      <SettingsDisclosure title="Recovery and output limits">
        <ContextRecovery draft={draft} set={set} setDraft={setDraft} />
      </SettingsDisclosure>
      {/* Self-healing (döngü koruması & ders çıkarma) moved to İçgörü ▸ Dersler. */}
    </>
  )
}
