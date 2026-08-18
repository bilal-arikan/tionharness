// ProviderFieldInput renders one FieldSpec-driven form control. The field's
// type/options/secret-ness comes entirely from the kind manifest (backend) —
// nothing about which kind has which field is hardcoded here.
import type { ProviderFieldSpec } from '@/api/providers'

const inputCls =
  'w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]'

interface Props {
  field: ProviderFieldSpec
  value: string
  // Whether this secret field already has a stored value on the backend
  // (write-only convention: leaving the input blank keeps it).
  isSet: boolean
  onChange: (value: string) => void
}

export function ProviderFieldInput({ field, value, isSet, onChange }: Props) {
  const placeholder =
    field.secret && isSet ? '(kayıtlı — değiştirmek için yaz)' : field.placeholder || field.default

  return (
    <label className="flex flex-col gap-1">
      <span className="text-xs font-medium text-[var(--color-text-dim)]">
        {field.label}
        {field.required && <span className="text-[var(--color-danger)]"> *</span>}
      </span>
      {field.type === 'select' ? (
        <select
          data-testid={`provider-field-${field.key}`}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className={inputCls}
        >
          <option value="">{field.placeholder || 'seç…'}</option>
          {(field.options ?? []).map((o) => (
            <option key={o} value={o}>
              {o}
            </option>
          ))}
        </select>
      ) : (
        <input
          data-testid={`provider-field-${field.key}`}
          type={field.type === 'password' ? 'password' : 'text'}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          className={inputCls}
        />
      )}
      {field.help && <span className="text-[10px] text-[var(--color-text-dim)]">{field.help}</span>}
      {field.secret && isSet && (
        <span className="text-[10px] text-[var(--color-success)]">✓ kayıtlı (şifreli)</span>
      )}
    </label>
  )
}
