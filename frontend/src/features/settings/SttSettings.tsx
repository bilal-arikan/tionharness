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

const ENGINE_OPTIONS: { value: SttEngine; label: string; hint?: string }[] = [
  { value: 'auto', label: 'Otomatik', hint: 'Sunucuda whisper varsa onu, yoksa tarayıcı tanımasını kullanır.' },
  { value: 'browser', label: 'Tarayıcı', hint: 'Web Speech API (Chromium; anlık ara sonuç).' },
  { value: 'server', label: 'Sunucu (whisper)', hint: 'Kaydı sunucuya yükler, whisper.cpp çevirir — offline, WebView2 dahil.' },
]

// SttSettings owns the dictation LANGUAGE (used by both engines), plus the STT
// ENGINE choice (browser vs server whisper.cpp) and the server model when
// installed. setSttLang broadcasts a same-window event so the mic tooltip updates
// live. All prefs device-local. The mic language list/pref live with the composer.
export function SttSettings() {
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

  return (
    <div className="flex flex-col gap-3">
      <Field
        label="Mikrofon dili"
        hint="Sesle yazma (dikte) için tanıma dili. Composer'daki mikrofon bu dili kullanır; aktif dil butonun ipucunda görünür."
      >
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
          label="Tanıma motoru"
          value={engine}
          options={ENGINE_OPTIONS}
          onChange={(v) => pickEngine(v as SttEngine)}
        />
      )}

      {eff === 'server' && srv.available && (
        <Field
          label="whisper modeli"
          hint={`${srv.models.length} model yüklü. Büyük model = daha doğru ama daha yavaş (base < small < medium).`}
        >
          <select className={inputCls} value={model} onChange={(e) => pickModel(e.target.value)}>
            <option value="">Otomatik (ilk yüklü model)</option>
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
