import { useEffect, useState } from 'react'
import { STT_LANGUAGES, sttLang, setSttLang } from '@/features/chat/composer/sttLanguages'
import {
  sttEngine,
  setSttEngine,
  resolveSttEngine,
  serverSttStatus,
  sttModel,
  setSttModel,
  initServerStt,
  type SttEngine,
} from '@/shared/lib/stt'
import { Field, Segmented, inputCls } from './primitives'
import { useTranslation } from 'react-i18next'

const ENGINE_VALUES: SttEngine[] = ['auto', 'browser', 'server']

// SttSettings owns the dictation LANGUAGE (used by both engines), plus the STT
// ENGINE choice (browser vs server whisper.cpp) and the server model when
// installed. setSttLang broadcasts a same-window event so the mic tooltip updates
// live. All prefs device-local. The mic language list/pref live with the composer.
export function SttSettings() {
  const { t } = useTranslation('settings')
  const [lang, setLang] = useState(() => sttLang())
  const [engine, setEngine] = useState<SttEngine>(() => sttEngine())
  const [model, setModel] = useState(() => sttModel())
  const [srv, setSrv] = useState(() => serverSttStatus())

  useEffect(() => {
    void initServerStt().then(setSrv)
  }, [])

  const pickLang = (v: string) => {
    setLang(v)
    setSttLang(v)
  }
  const pickEngine = (v: SttEngine) => {
    setEngine(v)
    setSttEngine(v)
  }
  const pickModel = (v: string) => {
    setModel(v)
    setSttModel(v)
  }
  const eff = resolveSttEngine()
  const engineOptions = ENGINE_VALUES.map((value) => ({
    value,
    label: t(`stt.engines.${value}.label`),
    hint: t(`stt.engines.${value}.hint`),
  }))

  return (
    <div className="flex flex-col gap-3">
      <Field label={t('stt.language')} hint={t('stt.languageHint')}>
        <select className={inputCls} value={lang} onChange={(e) => pickLang(e.target.value)}>
          {STT_LANGUAGES.map((l) => (
            <option key={l.value} value={l.value}>
              {l.icon} {l.label} — {l.value}
            </option>
          ))}
        </select>
      </Field>

      {/* Engine choice only matters when the server actually has whisper.cpp. */}
      {srv.available && (
        <Segmented
          label={t('stt.engine')}
          value={engine}
          options={engineOptions}
          onChange={(v) => pickEngine(v as SttEngine)}
        />
      )}

      {eff === 'server' && srv.available && (
        <Field label={t('stt.model')} hint={t('stt.modelHint', { count: srv.models.length })}>
          <select className={inputCls} value={model} onChange={(e) => pickModel(e.target.value)}>
            <option value="">{t('stt.autoModel')}</option>
            {srv.models.map((m) => (
              <option key={m.id} value={m.id}>
                {m.name}
              </option>
            ))}
          </select>
        </Field>
      )}
    </div>
  )
}
