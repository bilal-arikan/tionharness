import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
// Side-effect import: initialises i18next (synchronously, from bundled catalogs)
// before the first render, so no component can observe an untranslated frame.
import './i18n'
import { I18nRoot } from './i18n/I18nRoot'
import App from './app/App'
import { ErrorBoundary } from './shared/components/ErrorBoundary'
import { installGlobalErrorHandlers } from './shared/lib/reportError'
import { removeBootSplash } from './shared/lib/bootSplash'

// Catch uncaught exceptions + unhandled promise rejections process-wide and
// forward them to the backend log stream.
installGlobalErrorHandlers()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ErrorBoundary>
      <I18nRoot>
        <App />
      </I18nRoot>
    </ErrorBoundary>
  </StrictMode>,
)
removeBootSplash()
