// Languages offered for voice input (Web Speech API BCP-47 codes). Rendered by
// the composer's language picker (ComposerPicker) next to the mic button. Turkish
// leads since the app's primary UI language is Turkish; the rest are a compact
// multilingual set. Extend freely — any BCP-47 tag the browser engine supports.
import type { PickerOption } from './pickerOptions'

// The picker never renders an "auto/default" row, so every entry carries a real
// language code. `icon` is a flag glyph purely for at-a-glance recognition.
export const STT_LANGUAGES: PickerOption[] = [
  { value: 'tr-TR', label: 'Türkçe', hint: 'Turkish', icon: '🇹🇷' },
  { value: 'en-US', label: 'English', hint: 'English (US)', icon: '🇺🇸' },
  { value: 'de-DE', label: 'Deutsch', hint: 'German', icon: '🇩🇪' },
  { value: 'es-ES', label: 'Español', hint: 'Spanish', icon: '🇪🇸' },
  { value: 'fr-FR', label: 'Français', hint: 'French', icon: '🇫🇷' },
  { value: 'it-IT', label: 'Italiano', hint: 'Italian', icon: '🇮🇹' },
  { value: 'ru-RU', label: 'Русский', hint: 'Russian', icon: '🇷🇺' },
  { value: 'ar-SA', label: 'العربية', hint: 'Arabic', icon: '🇸🇦' },
]

const DEFAULT_STT_LANG = 'tr-TR'

// The chosen dictation language persists across sessions/reloads.
const STT_LANG_STORAGE_KEY = 'tionharness.stt.lang'

// Same-window change signal: the language is now set from the Settings screen but
// consumed by the composer's mic button, so a custom event syncs them live (the
// native 'storage' event only fires across windows, not within one).
const STT_LANG_EVENT = 'tionharness:stt-lang'

// sttLang returns the persisted dictation language, validated against the list
// (falls back to the default for an unknown/absent value).
export function sttLang(): string {
  try {
    const v = localStorage.getItem(STT_LANG_STORAGE_KEY)
    return STT_LANGUAGES.some((l) => l.value === v) ? (v as string) : DEFAULT_STT_LANG
  } catch {
    return DEFAULT_STT_LANG
  }
}

// setSttLang persists the choice and notifies same-window listeners (the mic
// button) so its active language + tooltip update without a reload.
export function setSttLang(v: string) {
  try {
    localStorage.setItem(STT_LANG_STORAGE_KEY, v)
  } catch {
    // best-effort
  }
  try {
    window.dispatchEvent(new CustomEvent(STT_LANG_EVENT, { detail: v }))
  } catch {
    // best-effort
  }
}

export function onSttLangChange(cb: (lang: string) => void): () => void {
  const handler = (e: Event) => cb((e as CustomEvent).detail ?? sttLang())
  window.addEventListener(STT_LANG_EVENT, handler)
  return () => window.removeEventListener(STT_LANG_EVENT, handler)
}

// sttLangLabel maps a language code to its display label (e.g. 'tr-TR' → 'Türkçe').
export function sttLangLabel(code: string): string {
  return STT_LANGUAGES.find((l) => l.value === code)?.label ?? code
}
