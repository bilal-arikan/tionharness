import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
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
      <App />
    </ErrorBoundary>
  </StrictMode>,
)
removeBootSplash()
