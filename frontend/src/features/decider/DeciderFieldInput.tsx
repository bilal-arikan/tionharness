// One setting of a decision provider, laid out like a provider field
// (settings/providers/ProviderFieldInput): small dim label, the control, an
// optional help line. Backend-specific fields come from the backend's manifest
// alone, so nothing here knows which backend has which field.
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { DeciderField } from '@/types/decider'
import { fieldInputCls } from './providerStyles'

interface ShellProps {
  label: string
  help?: string
  children: ReactNode
}

// FieldShell is the label + control + help frame every field of the form uses.
export function FieldShell({ label, help, children }: ShellProps) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-xs font-medium text-[var(--color-text-dim)]">{label}</span>
      {children}
      {help && <span className="text-[10px] text-[var(--color-text-dim)]">{help}</span>}
    </label>
  )
}

interface Props {
  backendId: string
  field: DeciderField
  value: string
  onChange: (value: string) => void
}

export function DeciderFieldInput({ backendId, field, value, onChange }: Props) {
  const { t } = useTranslation('decider')
  const base = `backend.${backendId}.field.${field.key}`
  const label = t(`${base}.label`, { defaultValue: field.label })
  const help = t(`${base}.help`, { defaultValue: field.help ?? '' })
  const placeholder = field.placeholder || field.default || ''
  const testid = `decider-field-${field.key}`

  return (
    <FieldShell label={label} help={help || undefined}>
      {field.type === 'select' ? (
        <select
          className={fieldInputCls}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          data-testid={testid}
        >
          <option value="">{placeholder}</option>
          {(field.options ?? []).map((o) => (
            <option key={o} value={o}>
              {o}
            </option>
          ))}
        </select>
      ) : field.type === 'textarea' ? (
        <textarea
          rows={2}
          className={`${fieldInputCls} font-mono text-xs`}
          value={value}
          placeholder={placeholder}
          onChange={(e) => onChange(e.target.value)}
          data-testid={testid}
        />
      ) : (
        <input
          type={field.type === 'number' ? 'number' : 'text'}
          inputMode={field.type === 'number' ? 'numeric' : undefined}
          className={fieldInputCls}
          value={value}
          placeholder={placeholder}
          onChange={(e) => onChange(e.target.value)}
          data-testid={testid}
        />
      )}
    </FieldShell>
  )
}
