import { Component, type ErrorInfo, type ReactNode } from 'react'
import { reportClientError } from '@/shared/lib/reportError'
import { Button } from './'
import { i18next } from '@/i18n'

interface Props {
  children: ReactNode
}

interface State {
  error: Error | null
}

// ErrorBoundary catches render-time crashes in the React tree so the app shows a
// recoverable fallback instead of a blank white screen, and forwards the crash
// to the backend log stream (Logs screen) via reportClientError. Only render
// errors are caught here; async/event errors are handled by the global handlers
// in reportError.ts.
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    reportClientError({
      source: 'error-boundary',
      message: error.message,
      stack: `${error.stack ?? ''}\n--- component stack ---${info.componentStack ?? ''}`,
    })
  }

  render() {
    if (!this.state.error) return this.props.children
    return (
      <div className="flex h-screen flex-col items-center justify-center gap-4 bg-[var(--color-bg)] p-8 text-center text-[var(--color-text)]">
        <div className="text-4xl">{'⚠️'}</div>
        <h1 className="text-lg font-semibold">
          {i18next.t('errorBoundary.title', { ns: 'sharedUi' })}
        </h1>
        <p className="max-w-md text-sm text-[var(--color-text-dim)]">
          {i18next.t('errorBoundary.description', { ns: 'sharedUi' })}
        </p>
        <pre className="max-w-md overflow-auto rounded bg-[var(--color-surface-2)] p-3 text-left text-xs text-[var(--color-danger)]">
          {this.state.error.message}
        </pre>
        <div className="flex gap-2">
          <Button variant="secondary" onClick={() => this.setState({ error: null })}>
            {i18next.t('errorBoundary.retry', { ns: 'sharedUi' })}
          </Button>
          <Button onClick={() => location.reload()}>
            {i18next.t('errorBoundary.reload', { ns: 'sharedUi' })}
          </Button>
        </div>
      </div>
    )
  }
}
