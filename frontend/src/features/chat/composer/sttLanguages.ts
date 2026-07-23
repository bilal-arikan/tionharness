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

export const DEFAULT_STT_LANG = 'tr-TR'

// The chosen dictation language persists across sessions/reloads.
export const STT_LANG_STORAGE_KEY = 'tionswarm.stt.lang'
