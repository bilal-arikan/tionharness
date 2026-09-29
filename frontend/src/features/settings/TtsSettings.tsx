import { useEffect, useState } from 'react'
import { Play } from 'lucide-react'
import {
  ttsVoices,
  onVoicesChanged,
  ttsVoiceURI,
  setTtsVoiceURI,
  ttsRate,
  setTtsRate,
  ttsPitch,
  setTtsPitch,
  ttsVolume,
  setTtsVolume,
  onTtsVolumeChange,
  speak,
  ttsEngine,
  setTtsEngine,
  resolveEngine,
  serverTtsStatus,
  serverVoiceId,
  setServerVoiceId,
  initServerTts,
  ttsSupported,
  type TtsEngine,
  TTS_RATE_MIN,
  TTS_RATE_MAX,
  TTS_PITCH_MIN,
  TTS_PITCH_MAX,
} from '@/shared/lib/tts'
import { Field, Slider, Segmented, inputCls } from './primitives'
import { useTranslation } from 'react-i18next'

const ENGINE_VALUES: TtsEngine[] = ['auto', 'browser', 'server']

// TtsSettings exposes read-aloud engine + voice controls: server (Piper) vs
// browser engine, the matching voice list, rate and pitch, plus a test button.
// All prefs are device-local (tts.ts) and apply to auto-read and the 🔊 button.
export function TtsSettings() {
  const { t } = useTranslation('settings')
  const [engine, setEngine] = useState<TtsEngine>(() => ttsEngine())
  const [browserVoices, setBrowserVoices] = useState<SpeechSynthesisVoice[]>(() => ttsVoices())
  const [voiceURI, setVoice] = useState(() => ttsVoiceURI())
  const [srvVoice, setSrvVoice] = useState(() => serverVoiceId())
  const [rate, setRate] = useState(() => ttsRate())
  const [pitch, setPitch] = useState(() => ttsPitch())
  // Global volume: mirrors the per-bubble sliders live (shared broadcast).
  const [volume, setVolume] = useState(() => ttsVolume())
  // Refresh cached server status + browser voice list on mount (both may arrive async).
  const [srv, setSrv] = useState(() => serverTtsStatus())

  useEffect(() => {
    void initServerTts().then(setSrv)
    const refresh = () => setBrowserVoices(ttsVoices())
    const off = onVoicesChanged(refresh)
    const offVol = onTtsVolumeChange(setVolume)
    const t = window.setTimeout(refresh, 0)
    return () => {
      window.clearTimeout(t)
      off()
      offVol()
    }
  }, [])

  const eff = resolveEngine()
  const pickEngine = (v: TtsEngine) => {
    setEngine(v)
    setTtsEngine(v)
  }
  const pickBrowserVoice = (v: string) => {
    setVoice(v)
    setTtsVoiceURI(v)
  }
  const pickServerVoice = (v: string) => {
    setSrvVoice(v)
    setServerVoiceId(v)
  }
  const changeRate = (v: number) => {
    setRate(v)
    setTtsRate(v)
  }
  const changePitch = (v: number) => {
    setPitch(v)
    setTtsPitch(v)
  }
  const changeVolume = (v: number) => {
    setVolume(v)
    setTtsVolume(v) // broadcast → every bubble slider updates too
  }
  const test = () => speak(t('tts.sample'))

  const sortedBrowser = [...browserVoices].sort(
    (a, b) => a.lang.localeCompare(b.lang) || a.name.localeCompare(b.name),
  )
  const engineOptions = ENGINE_VALUES.map((value) => ({
    value,
    label: t(`tts.engines.${value}.label`),
    hint: t(`tts.engines.${value}.hint`),
  }))

  return (
    <div className="flex flex-col gap-3">
      {/* Engine choice is only meaningful when the server actually has Piper. */}
      {srv.available && (
        <Segmented
          label={t('tts.engine')}
          value={engine}
          options={engineOptions}
          onChange={(v) => pickEngine(v as TtsEngine)}
        />
      )}

      {eff === 'server' ? (
        <Field
          label={t('tts.serverVoice')}
          hint={t('tts.serverVoiceHint', { count: srv.voices.length })}
        >
          <select
            className={inputCls}
            value={srvVoice}
            onChange={(e) => pickServerVoice(e.target.value)}
          >
            <option value="">{t('tts.autoServerVoice')}</option>
            {srv.voices.map((v) => (
              <option key={v.id} value={v.id}>
                {v.name} — {v.lang}
              </option>
            ))}
          </select>
        </Field>
      ) : (
        ttsSupported() && (
          <Field
            label={t('tts.browserVoice')}
            hint={
              sortedBrowser.length
                ? t('tts.browserVoiceHint', { count: sortedBrowser.length })
                : t('tts.browserVoiceEmpty')
            }
          >
            <select
              className={inputCls}
              value={voiceURI}
              onChange={(e) => pickBrowserVoice(e.target.value)}
            >
              <option value="">{t('tts.autoBrowserVoice')}</option>
              {sortedBrowser.map((v) => (
                <option key={v.voiceURI} value={v.voiceURI}>
                  {v.localService ? '' : '☁ '}
                  {v.name} — {v.lang}
                  {v.default ? ` (${t('tts.defaultVoice')})` : ''}
                </option>
              ))}
            </select>
          </Field>
        )
      )}

      {/* Global volume — same value as every chat bubble's inline slider. */}
      <Slider
        label={t('tts.volume')}
        badge={`%${Math.round(volume * 100)}`}
        min={0}
        max={1}
        step={0.05}
        value={volume}
        onChange={changeVolume}
      />
      <Slider
        label={t('tts.rate')}
        badge={`${rate.toFixed(2)}×`}
        min={TTS_RATE_MIN}
        max={TTS_RATE_MAX}
        step={0.05}
        value={rate}
        onChange={changeRate}
      />
      {/* Pitch only affects the browser engine (Piper playback has no pitch knob). */}
      {eff !== 'server' && (
        <Slider
          label={t('tts.pitch')}
          badge={pitch.toFixed(2)}
          min={TTS_PITCH_MIN}
          max={TTS_PITCH_MAX}
          step={0.1}
          value={pitch}
          onChange={changePitch}
        />
      )}

      <button
        type="button"
        onClick={test}
        className="flex w-fit items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-1.5 text-sm font-medium transition hover:border-[var(--color-accent)]"
      >
        <Play size={14} />
        {t('tts.test')}
      </button>
    </div>
  )
}
