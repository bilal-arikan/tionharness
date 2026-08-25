// I18nRoot remounts its subtree when the UI language changes.
//
// react-i18next only re-renders components that subscribed through useTranslation.
// A large share of this app's user-visible text is produced outside that channel:
// formatter helpers (shared/lib/time, format, intl), sort comparators, label maps
// and values memoised with useMemo. Those would all keep their old-locale output
// until something unrelated happened to re-render them, which shows up as a
// half-translated screen after a language switch.
//
// Keying the subtree on the locale trades a rare full remount (a deliberate
// settings action, not a hot path) for the guarantee that nothing survives the
// switch in the previous language. The cost is that transient component state
// under it is reset — acceptable, and in fact desirable, for a language change.

import { Fragment, useEffect, useState, type ReactNode } from 'react'
import { i18next } from './index'
import { currentLocale } from './index'
import type { Locale } from './locales'

export function I18nRoot({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(() => currentLocale())

  useEffect(() => {
    const onChanged = () => setLocaleState(currentLocale())
    i18next.on('languageChanged', onChanged)
    // The locale may have been switched between the initial render and this
    // effect (settings resolve early during boot); re-sync rather than assume.
    onChanged()
    return () => {
      i18next.off('languageChanged', onChanged)
    }
  }, [])

  // A keyed Fragment remounts the subtree without introducing a DOM node, so the
  // app's layout (which relies on being a direct child of #root) is untouched.
  return <Fragment key={locale}>{children}</Fragment>
}
