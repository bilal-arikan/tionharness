// Adım Türleri category: read-only reference of the assistant turn's activity
// trace step kinds (mirrors internal/agent/trace.go).
import { STEP_KINDS } from '@/shared/stepKinds'

export function StepKindsPanel() {
  return (
    <>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
        Bir asistan turunun aktivite izi (<code className="rounded bg-[var(--color-bg)] px-1">TurnStep</code>) farklı <strong>türlerden</strong> oluşur. Aşağıda her türün ne anlama geldiği, kalıcı mı yoksa yalnız-canlı mı olduğu ve şu an aktif mi listelenir. (Kaynak: <code className="rounded bg-[var(--color-bg)] px-1">internal/agent/trace.go</code>)
      </div>
      <div className="flex flex-col gap-1.5">
        {STEP_KINDS.map((s) => (
          <div
            key={s.kind}
            className="flex items-start gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2"
          >
            <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-[var(--color-surface-2)] text-[var(--color-text-dim)]">
              <s.Icon size={16} />
            </span>
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium text-[var(--color-text)]">{s.label}</span>
                <code className="rounded bg-[var(--color-surface-2)] px-1 font-mono text-[11px] text-[var(--color-text-dim)]">
                  {s.kind}
                </code>
                <span
                  className={`rounded px-1.5 py-0.5 text-[10px] ${
                    s.status === 'active'
                      ? 'bg-[color-mix(in_srgb,var(--color-success)_15%,transparent)] text-[var(--color-success)]'
                      : 'bg-[color-mix(in_srgb,var(--color-warning)_15%,transparent)] text-[var(--color-warning)]'
                  }`}
                >
                  {s.status === 'active' ? 'aktif' : 'altyapı hazır'}
                </span>
                <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)]">
                  {s.persisted ? 'kalıcı' : 'yalnız-canlı'}
                </span>
              </div>
              <div className="mt-0.5 text-xs text-[var(--color-text-dim)]">{s.description}</div>
            </div>
          </div>
        ))}
      </div>
    </>
  )
}
