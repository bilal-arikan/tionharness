import { useTranslation } from 'react-i18next'
import type { AppSettings } from '@/types'
import { Field, inputCls } from './primitives'
import { PromptEditor } from '@/shared/components'
import { LOCALES, localeMeta } from '@/i18n'
import type { PanelProps } from './settingsPanelShared'

export function ProfilePanel({ draft, set }: PanelProps) {
  const { t } = useTranslation('common')
  return (
    <>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
        Bu bilgiler ajanların yanıtlarını sana göre kişiselleştirmesi için sohbet bağlamına eklenir.
      </div>
      <Field label="Ad" hint="Ajan sana nasıl hitap etsin.">
        <input
          value={draft.userName}
          onChange={(e) => set('userName', e.target.value)}
          placeholder="örn. Ada"
          className={inputCls}
        />
      </Field>
      <Field label="Saat dilimi" hint="'yarın', 'gelecek hafta' gibi göreli tarihler için.">
        <input
          value={draft.userTimezone}
          onChange={(e) => set('userTimezone', e.target.value)}
          placeholder="örn. Europe/Istanbul"
          className={inputCls}
        />
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Şehir">
          <input
            value={draft.userCity}
            onChange={(e) => set('userCity', e.target.value)}
            placeholder="örn. İstanbul"
            className={inputCls}
          />
        </Field>
        <Field label="Ülke">
          <input
            value={draft.userCountry}
            onChange={(e) => set('userCountry', e.target.value)}
            placeholder="örn. Türkiye"
            className={inputCls}
          />
        </Field>
      </div>
      <Field label="Notlar" hint="Tercihlerini anlatan serbest metin (talimatlar, çalışma şekli…).">
        <PromptEditor
          value={draft.userNotes}
          onChange={(v) => set('userNotes', v)}
          rows={5}
          placeholder="Ajanların bilmesi gereken tercihlerin…"
        />
      </Field>
      {/* Two independent language axes, deliberately adjacent so the difference is
          visible at a glance: the interface can be English while the agent still
          answers in Turkish, or the reverse. See _Docs/73. */}
      <Field label={t('language.uiLabel')} hint={t('language.uiHint')}>
        <select
          value={draft.uiLanguage}
          onChange={(e) => set('uiLanguage', e.target.value as AppSettings['uiLanguage'])}
          className={inputCls}
        >
          <option value="">
            {t('language.followAgent', { language: localeMeta(draft.language).label })}
          </option>
          {LOCALES.map((l) => (
            <option key={l.code} value={l.code}>
              {l.label}
            </option>
          ))}
        </select>
      </Field>
      <Field label={t('language.agentLabel')} hint={t('language.agentHint')}>
        <select
          value={draft.language}
          onChange={(e) => set('language', e.target.value as AppSettings['language'])}
          className={inputCls}
        >
          {LOCALES.map((l) => (
            <option key={l.code} value={l.code}>
              {l.label}
            </option>
          ))}
        </select>
      </Field>
    </>
  )
}
