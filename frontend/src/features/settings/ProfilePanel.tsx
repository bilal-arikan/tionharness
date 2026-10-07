import { useTranslation } from 'react-i18next'
import type { AppSettings } from '@/types'
import { Field, inputCls } from './primitives'
import { PromptEditor } from '@/shared/components'
import { LOCALES, localeMeta } from '@/i18n'
import type { PanelProps } from './settingsPanelShared'

export function ProfilePanel({ draft, set }: PanelProps) {
  const { t } = useTranslation(['settings', 'common'])
  return (
    <>
      {/* The page intro (profile.intro) sits behind the (ⓘ) next to the category
          title in SettingsPanel. */}
      <Field label={t('profile.name')} hint={t('profile.nameHint')}>
        <input
          value={draft.userName}
          onChange={(e) => set('userName', e.target.value)}
          placeholder={t('profile.namePlaceholder')}
          className={inputCls}
        />
      </Field>
      <Field label={t('profile.timezone')} hint={t('profile.timezoneHint')}>
        <input
          value={draft.userTimezone}
          onChange={(e) => set('userTimezone', e.target.value)}
          placeholder={t('profile.timezonePlaceholder')}
          className={inputCls}
        />
      </Field>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field label={t('profile.city')}>
          <input
            value={draft.userCity}
            onChange={(e) => set('userCity', e.target.value)}
            placeholder={t('profile.cityPlaceholder')}
            className={inputCls}
          />
        </Field>
        <Field label={t('profile.country')}>
          <input
            value={draft.userCountry}
            onChange={(e) => set('userCountry', e.target.value)}
            placeholder={t('profile.countryPlaceholder')}
            className={inputCls}
          />
        </Field>
      </div>
      <Field label={t('profile.notes')} hint={t('profile.notesHint')}>
        <PromptEditor
          value={draft.userNotes}
          onChange={(v) => set('userNotes', v)}
          rows={5}
          placeholder={t('profile.notesPlaceholder')}
        />
      </Field>
      {/* Two independent language axes, deliberately adjacent so the difference is
          visible at a glance: the interface can be English while the agent still
          answers in Turkish, or the reverse. See _Docs/73. */}
      <Field
        label={t('language.uiLabel', { ns: 'common' })}
        hint={t('language.uiHint', { ns: 'common' })}
      >
        <select
          value={draft.uiLanguage}
          onChange={(e) => set('uiLanguage', e.target.value as AppSettings['uiLanguage'])}
          className={inputCls}
        >
          <option value="">
            {t('language.followAgent', {
              ns: 'common',
              language: localeMeta(draft.language).label,
            })}
          </option>
          {LOCALES.map((l) => (
            <option key={l.code} value={l.code}>
              {l.label}
            </option>
          ))}
        </select>
      </Field>
      <Field
        label={t('language.agentLabel', { ns: 'common' })}
        hint={t('language.agentHint', { ns: 'common' })}
      >
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
