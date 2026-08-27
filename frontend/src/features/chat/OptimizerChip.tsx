import type { ShellOptimization } from '@/types'

interface Props {
  optimizer: ShellOptimization
}

// OptimizerChip marks a tool step whose output was shrunk by an external token
// optimizer before the model read it. Without it the rewrite is invisible: the
// agent sees sqz's abbreviated text (an inline legend + «A1» placeholders) and
// the user, reading the same card, has no way to tell that from truncation.
//
// 'sqz' reports exact token counts, so the chip shows the real reduction and the
// before/after pair on hover. 'rtk' wraps the command upstream of us — there is
// no before/after to measure, so it shows the name only rather than a made-up
// percentage.
export function OptimizerChip({ optimizer }: Props) {
  const { kind, inTokens, outTokens, dedup, command, degraded } = optimizer
  const measured =
    typeof inTokens === 'number' &&
    typeof outTokens === 'number' &&
    inTokens > 0 &&
    outTokens < inTokens
  const percent = measured ? Math.round(((inTokens - outTokens) / inTokens) * 100) : 0

  // A rewritten command that FAILED is the one case where the optimizer may be
  // actively misleading, so it takes precedence over any saving figure — a "−%93"
  // next to a hidden error would be celebrating the wrong thing. Then dedup, which
  // a reader would otherwise file as a bug (the command printed plenty, the card
  // shows one "§ref:…§" line).
  const title = degraded
    ? `${kind}: komut BAŞARISIZ oldu ve yukarıdaki metin ham çıktı değil, ${kind} özeti. ` +
      `Test/derleme hataları korunur ama kurulum hataları (bozuk go.mod, eksik toolchain) ` +
      `anlamsız bir özete inebilir. Gerçek nedeni görmek için aynı komutu no_compress ile tekrar çalıştır.` +
      (command ? `\n\nÇalıştırılan: ${command}` : '')
    : dedup
      ? `${kind}: bu çıktı oturumda daha önce görülenle birebir aynı — tekrar edilmek yerine "§ref:…§" işaretçisiyle değiştirildi. Komut düzgün çalıştı, çıktı kaybolmadı.`
      : (measured
          ? `${kind}: ${inTokens} → ${outTokens} token (%${percent} tasarruf) — çıktı kısaltılmış olarak bağlama girdi, kırpılmadı`
          : `${kind}: komut token-optimize edici üzerinden çalıştı`) +
        (command ? `\n\nÇalıştırılan: ${command}` : '')

  return (
    <span
      title={title}
      className={`shrink-0 rounded px-1.5 py-px font-mono text-[10px] ${
        degraded
          ? 'bg-[var(--color-surface-2)] text-[var(--color-warning)]'
          : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
      }`}
    >
      {kind}
      {degraded ? (
        <span className="ml-1">özet — ham değil</span>
      ) : dedup ? (
        <span className="ml-1 text-[var(--color-success)]">yinelenen</span>
      ) : (
        measured && <span className="ml-1 text-[var(--color-success)]">−%{percent}</span>
      )}
    </span>
  )
}
