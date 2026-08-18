// ProviderInstanceList renders every configured provider instance as a card:
// label, kind badge, transport, secret-set status, and edit/delete actions.
// Deleting reports how many agents were still pointing at the instance (never
// swallowed silently, _Docs/71 Faz 3).
import { Boxes, Trash2 } from 'lucide-react'
import type { ProviderInstance, ProviderKind } from '@/api/providers'

interface Props {
  instances: ProviderInstance[]
  kinds: ProviderKind[]
  onEdit: (instance: ProviderInstance) => void
  onDelete: (instance: ProviderInstance) => void
}

export function ProviderInstanceList({ instances, kinds, onEdit, onDelete }: Props) {
  if (instances.length === 0) {
    return (
      <p className="text-xs text-[var(--color-text-dim)]">
        Henüz sağlayıcı örneği yok. Aşağıdan bir taslak seçip ekle.
      </p>
    )
  }

  return (
    <div className="grid gap-2">
      {instances.map((inst) => {
        const kind = kinds.find((k) => k.id === inst.kindId)
        const secretKeys = Object.keys(inst.secretsSet)
        const secretsOk = secretKeys.length === 0 || secretKeys.every((k) => inst.secretsSet[k])
        return (
          <div
            key={inst.id}
            className="space-y-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
          >
            <div className="flex items-center justify-between gap-2">
              <div className="flex min-w-0 items-center gap-2">
                <Boxes size={15} className="shrink-0 text-[var(--color-accent)]" />
                <span className="truncate text-sm font-medium">{inst.label || inst.id}</span>
                <span className="shrink-0 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
                  {kind?.label ?? inst.kindId}
                </span>
                {!inst.enabled && (
                  <span className="shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)]">
                    devre dışı
                  </span>
                )}
              </div>
              <span
                className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium ${
                  secretsOk
                    ? 'bg-[var(--color-surface-2)] text-[var(--color-success)]'
                    : 'bg-[var(--color-surface-2)] text-[var(--color-warning)]'
                }`}
              >
                {secretsOk ? '✓ yapılandırıldı' : '⚠ eksik alan'}
              </span>
            </div>

            <div className="grid gap-x-3 gap-y-1 text-xs sm:grid-cols-2">
              <div className="min-w-0 truncate" title={inst.id}>
                <span className="text-[var(--color-text-dim)]">id: </span>
                {inst.id}
              </div>
              <div className="min-w-0 truncate" title={inst.defaultModel}>
                <span className="text-[var(--color-text-dim)]">model: </span>
                {inst.defaultModel || '—'}
              </div>
            </div>

            <div className="flex items-center justify-end gap-1.5">
              <button
                data-testid="provider-instance-edit"
                data-provider-id={inst.id}
                onClick={() => onEdit(inst)}
                className="rounded border border-[var(--color-border)] px-2 py-1 text-xs hover:border-[var(--color-accent)]"
              >
                Düzenle
              </button>
              <button
                data-testid="provider-instance-delete"
                data-provider-id={inst.id}
                onClick={() => onDelete(inst)}
                className="rounded border border-[var(--color-border)] p-1 text-[var(--color-danger)] hover:border-[var(--color-danger)]"
              >
                <Trash2 size={13} />
              </button>
            </div>
          </div>
        )
      })}
    </div>
  )
}
