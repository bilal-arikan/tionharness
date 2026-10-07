// ProviderFieldInput renders one FieldSpec-driven form control. The field's
// type/options/secret-ness comes entirely from the kind manifest (backend) —
// nothing about which kind has which field is hardcoded here.
import type { ProviderFieldSpec } from '@/api/providers'
import { useTranslation } from 'react-i18next'
import { FieldHint, InfoPopover } from '@/shared/components/InfoPopover'

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
  const { t } = useTranslation('settings')
  const placeholder =
    field.secret && isSet
      ? t('providerFields.savedPlaceholder')
      : field.placeholder || field.default

  return (
    <label className="flex flex-col gap-1">
      <span className="flex items-center gap-1 text-xs font-medium text-[var(--color-text-dim)]">
        <span>
          {field.label}
          {field.required && <span className="text-[var(--color-danger)]"> *</span>}
        </span>
        {field.help && <InfoPopover text={field.help} mode="long" />}
      </span>
      {field.type === 'select' ? (
        <select
          data-testid={`provider-field-${field.key}`}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className={inputCls}
        >
          <option value="">{field.placeholder || t('providerFields.select')}</option>
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
      {field.secret && isSet && (
        <span className="text-[10px] text-[var(--color-success)]">
          {t('providerFields.savedEncrypted')}
        </span>
      )}
      <FieldHint text={field.help} />
    </label>
  )
}
