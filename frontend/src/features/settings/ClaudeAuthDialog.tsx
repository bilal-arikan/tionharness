import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, ExternalLink, Globe, Loader2 } from 'lucide-react'
import { api } from '@/api'
import { workspaceApi } from '@/api/workspaces'
import { Button, ModalOverlay } from '@/shared/components'
import { inputCls } from './primitives'

function isRemoteAccess(): boolean {
  const hostname = typeof window !== 'undefined' ? window.location.hostname : 'localhost'
  return !['localhost', '127.0.0.1', '::1', '[::1]', ''].includes(hostname)
}

interface Props {
  providerId?: string
  providerLabel: string
  isLoggedIn: boolean
  onClose: () => void
  onLoggedIn: () => void
}

export function ClaudeAuthDialog({
  providerId,
  providerLabel,
  isLoggedIn,
  onClose,
  onLoggedIn,
}: Props) {
  const { t } = useTranslation('settingsMain')
  const remote = isRemoteAccess()
  const startOAuth = () =>
    providerId ? api.startClaudeOAuth(providerId) : workspaceApi.startClaudeOAuth()
  const startOAuthLoopback = () =>
    providerId ? api.startClaudeOAuthLoopback(providerId) : workspaceApi.startClaudeOAuthLoopback()
  const completeOAuth = (flow: string, value: string) =>
    providerId
      ? api.completeClaudeOAuth(providerId, flow, value)
      : workspaceApi.completeClaudeOAuth(flow, value)
  const [mode, setMode] = useState<'auto' | 'manual'>(remote ? 'manual' : 'auto')
  const [flowId, setFlowId] = useState('')
  const [authUrl, setAuthUrl] = useState('')
  const [code, setCode] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [done, setDone] = useState(false)

  useEffect(() => {
    if (!flowId || mode !== 'auto' || done) return
    let active = true
    const timer = setInterval(async () => {
      try {
        const result = providerId
          ? await api.claudeOAuthLoopbackStatus(providerId, flowId)
          : await workspaceApi.claudeOAuthLoopbackStatus(flowId)
        if (!active) return
        if (result.status === 'ok') {
          setDone(true)
          setFlowId('')
          onLoggedIn()
        } else if (result.status === 'error' || result.status === 'unknown') {
          setError(result.detail || t('auth.loginFailed'))
          setFlowId('')
        }
      } catch {
        // Polling errors are transient; the next request can still complete the flow.
      }
    }, 1500)
    return () => {
      active = false
      clearInterval(timer)
    }
  }, [done, flowId, mode, onLoggedIn, providerId, t])

  const start = async () => {
    setBusy(true)
    setError(null)
    try {
      const result = mode === 'auto' ? await startOAuthLoopback() : await startOAuth()
      setFlowId(result.flowId)
      setAuthUrl(result.authUrl)
      window.open(result.authUrl, '_blank', 'noopener,noreferrer')
    } catch (caught) {
      setError((caught as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const complete = async () => {
    const value = code.trim()
    if (!value) {
      setError(t('auth.claude.pasteCode'))
      return
    }
    setBusy(true)
    setError(null)
    try {
      await completeOAuth(flowId, value)
      setDone(true)
      onLoggedIn()
    } catch (caught) {
      setError((caught as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <ModalOverlay onClose={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label={t('auth.claude.title')}
        data-testid="claude-auth-modal"
        className="w-full max-w-lg rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 shadow-2xl"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <h2 className="mb-1 text-base font-semibold">{t('auth.claude.title')}</h2>
        <p className="mb-4 text-xs text-[var(--color-text-dim)]">
          {t('auth.claude.description', { provider: providerLabel })}
          {isLoggedIn && (
            <span className="ml-1 text-[var(--color-success)]">✓ {t('auth.loggedIn')}</span>
          )}
        </p>

        {done ? (
          <div className="flex flex-col items-center gap-2 rounded-lg border border-[var(--color-success)]/40 px-4 py-6 text-center">
            <Check size={28} className="text-[var(--color-success)]" />
            <p className="text-sm font-medium">{t('auth.success')}</p>
            <Button onClick={onClose} size="lg" className="mt-2">
              {t('shared.close')}
            </Button>
          </div>
        ) : (
          <div className="space-y-3">
            <div className="flex gap-1 rounded-lg border border-[var(--color-border)] p-0.5 text-xs">
              <button
                onClick={() => setMode('auto')}
                className={`flex-1 rounded-md px-2 py-1 ${mode === 'auto' ? 'bg-[var(--color-accent)]/15 text-[var(--color-accent)]' : 'text-[var(--color-text-dim)]'}`}
              >
                {t('auth.claude.auto')}
              </button>
              <button
                onClick={() => setMode('manual')}
                className={`flex-1 rounded-md px-2 py-1 ${mode === 'manual' ? 'bg-[var(--color-accent)]/15 text-[var(--color-accent)]' : 'text-[var(--color-text-dim)]'}`}
              >
                {t('auth.claude.manual')}
              </button>
            </div>

            {mode === 'auto' && remote && (
              <p className="rounded-lg border border-[var(--color-warning)]/40 px-3 py-2 text-xs text-[var(--color-text-dim)]">
                {t('auth.claude.remoteWarning')}
              </p>
            )}

            {!authUrl ? (
              <Button onClick={start} size="lg" disabled={busy}>
                {busy ? <Loader2 size={15} className="animate-spin" /> : <Globe size={15} />}
                {busy ? t('auth.connecting') : t('auth.startBrowser')}
              </Button>
            ) : mode === 'auto' ? (
              <div className="flex items-center gap-2 rounded-lg border border-[var(--color-border)] px-3 py-3 text-xs text-[var(--color-text-dim)]">
                <Loader2 size={14} className="animate-spin text-[var(--color-accent)]" />
                <span className="flex-1">{t('auth.claude.waiting')}</span>
                <a
                  href={authUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1 text-[var(--color-accent)]"
                >
                  <ExternalLink size={12} /> {t('auth.reopen')}
                </a>
              </div>
            ) : (
              <div className="space-y-2">
                <a
                  href={authUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1 text-xs text-[var(--color-accent)]"
                >
                  <ExternalLink size={12} /> {t('auth.claude.reopenLogin')}
                </a>
                <input
                  autoFocus
                  value={code}
                  onChange={(event) => setCode(event.target.value)}
                  onKeyDown={(event) => event.key === 'Enter' && complete()}
                  placeholder={t('auth.claude.codePlaceholder')}
                  className={`${inputCls} w-full font-mono`}
                  data-testid="claude-oauth-code-input"
                />
                <Button onClick={complete} size="lg" disabled={busy} className="w-full">
                  {busy ? <Loader2 size={15} className="animate-spin" /> : <Check size={15} />}
                  {busy ? t('auth.verifying') : t('auth.complete')}
                </Button>
              </div>
            )}
            {error && <p className="text-xs text-[var(--color-danger)]">{error}</p>}
          </div>
        )}
      </div>
    </ModalOverlay>
  )
}
