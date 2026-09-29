// CodexAuthDialog is the popup for logging the codex-cli provider into THIS
// workspace's isolated CODEX_HOME. Two methods, mirroring ClaudeAuthDialog:
//   - Device login: `codex login --device-auth` — show a verification URL +
//     one-time code, poll status until the user approves in their browser.
//   - API key: piped to `codex login --with-api-key` over stdin, never logged.
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { KeyRound, Globe, ExternalLink, Loader2, Check, Copy } from 'lucide-react'
import { api } from '@/api'
import { copyToClipboard } from '@/shared/lib/clipboard'
import { Button, ModalOverlay, toast } from '@/shared/components'
import { inputCls } from './primitives'

type Method = 'device' | 'apikey'

interface Props {
  providerId: string
  providerLabel: string
  isLoggedIn: boolean
  onClose: () => void
  onLoggedIn: () => void
}

// codexDeviceErrorLabel maps a device-flow terminal state to a Turkish message.
function codexDeviceErrorLabel(state: string, t: (key: string) => string, detail?: string): string {
  switch (state) {
    case 'expired':
      return t('auth.codex.errors.expired')
    case 'cancelled':
      return t('auth.codex.errors.cancelled')
    case 'failed':
    default:
      return detail || t('auth.loginFailed')
  }
}

export function CodexAuthDialog({
  providerId,
  providerLabel,
  isLoggedIn,
  onClose,
  onLoggedIn,
}: Props) {
  const { t } = useTranslation('settingsMain')
  const [method, setMethod] = useState<Method>('device')

  // Device-auth flow state.
  const [verifyUrl, setVerifyUrl] = useState('')
  const [code, setCode] = useState('')
  const [deviceBusy, setDeviceBusy] = useState(false)
  const [deviceErr, setDeviceErr] = useState<string | null>(null)
  const [deviceDone, setDeviceDone] = useState(false)
  const [copied, setCopied] = useState(false)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const stopPolling = () => {
    if (pollRef.current) {
      clearInterval(pollRef.current)
      pollRef.current = null
    }
  }

  // Clean up the poll interval on unmount so a closed modal never leaks a timer.
  useEffect(() => stopPolling, [])

  const startPolling = () => {
    stopPolling()
    pollRef.current = setInterval(async () => {
      try {
        const r = await api.codexDeviceAuthStatus(providerId)
        if (r.state === 'success') {
          stopPolling()
          setDeviceDone(true)
        } else if (r.state === 'failed' || r.state === 'expired' || r.state === 'cancelled') {
          stopPolling()
          setDeviceErr(codexDeviceErrorLabel(r.state, t, r.error))
        }
        // 'pending' → keep polling
      } catch {
        /* transient — keep polling */
      }
    }, 2500)
  }

  const startDeviceLogin = async () => {
    setDeviceBusy(true)
    setDeviceErr(null)
    // Open the tab NOW, synchronously inside the click handler: a window.open()
    // issued after `await` has lost the user-gesture context and is blocked by
    // the popup blocker. The blank tab is redirected once the URL arrives (and
    // closed again if the request failed).
    //
    // 'noopener' MUST NOT be passed here: with it the browser returns null
    // instead of a window handle, leaving an un-navigable blank tab behind. The
    // opener link is severed on the handle instead, which gives the same
    // isolation while keeping the handle we need to navigate.
    const tab = window.open('about:blank', '_blank')
    if (tab) tab.opener = null
    try {
      const r = await api.startCodexDeviceAuth(providerId)
      setVerifyUrl(r.verifyUrl)
      setCode(r.code)
      if (tab && !tab.closed) tab.location.href = r.verifyUrl
      else window.open(r.verifyUrl, '_blank', 'noopener,noreferrer')
      startPolling()
    } catch (e) {
      if (tab && !tab.closed) tab.close()
      const msg = (e as Error).message
      if (msg.includes('409')) {
        setDeviceErr(t('auth.codex.errors.inProgress'))
      } else if (msg.includes('400')) {
        setDeviceErr(t('auth.codex.errors.cliMissing'))
      } else {
        setDeviceErr(msg)
      }
    } finally {
      setDeviceBusy(false)
    }
  }

  const cancelDeviceLogin = async () => {
    stopPolling()
    try {
      await api.cancelCodexDeviceAuth(providerId)
    } catch {
      /* best-effort */
    }
    setVerifyUrl('')
    setCode('')
    setDeviceErr(null)
  }

  const retryDeviceLogin = async () => {
    setDeviceErr(null)
    setVerifyUrl('')
    setCode('')
    // A previous flow may still be tracked server-side (409) — cancel first.
    try {
      await api.cancelCodexDeviceAuth(providerId)
    } catch {
      /* best-effort */
    }
    startDeviceLogin()
  }

  const copyCode = async () => {
    if (await copyToClipboard(code)) {
      setCopied(true)
      toast.info(t('auth.codex.copied'))
      setTimeout(() => setCopied(false), 1500)
    }
  }

  // API-key method state.
  const [apiKey, setApiKey] = useState('')
  const [apiBusy, setApiBusy] = useState(false)
  const [apiErr, setApiErr] = useState<string | null>(null)
  const apiKeyRef = useRef<HTMLInputElement>(null)

  const submitApiKey = async () => {
    const k = apiKey.trim()
    if (!k) {
      setApiErr(t('auth.codex.apiKeyRequired'))
      apiKeyRef.current?.focus()
      return
    }
    setApiBusy(true)
    setApiErr(null)
    try {
      await api.codexAPIKeyLogin(providerId, k)
      toast.success(t('auth.loggedIn'))
      onLoggedIn()
      onClose()
    } catch (e) {
      const msg = (e as Error).message
      setApiErr(msg.includes('400') ? t('auth.codex.errors.cliMissing') : msg)
    } finally {
      setApiBusy(false)
    }
  }

  const close = () => {
    // Only cancel an in-flight flow — a completed one should stay in place.
    if (verifyUrl && !deviceDone) cancelDeviceLogin()
    onClose()
  }

  const tabCls = (m: Method) =>
    `flex flex-1 items-center justify-center gap-1.5 rounded-lg border px-3 py-2 text-sm transition ${
      method === m
        ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)]'
        : 'border-[var(--color-border)] hover:bg-[var(--color-surface-2)]'
    }`

  return (
    <ModalOverlay onClose={close}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label={t('auth.codex.title')}
        data-testid="codex-auth-modal"
        className="w-full max-w-lg rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 shadow-2xl"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <h2 className="mb-1 text-base font-semibold">{t('auth.codex.title')}</h2>
        <p className="mb-4 text-xs text-[var(--color-text-dim)]">
          {t('auth.codex.description', { provider: providerLabel })}
          {isLoggedIn && (
            <span className="ml-1 text-[var(--color-success)]">
              ✓ {t('auth.currentlyLoggedIn')}
            </span>
          )}
        </p>

        {/* Method tabs */}
        <div className="mb-4 flex gap-2">
          <button onClick={() => setMethod('device')} className={tabCls('device')}>
            <Globe size={14} /> {t('auth.codex.browserTab')}
          </button>
          <button onClick={() => setMethod('apikey')} className={tabCls('apikey')}>
            <KeyRound size={14} /> {t('auth.codex.apiKeyTab')}
          </button>
        </div>

        {method === 'device' ? (
          <div className="mb-2 space-y-3">
            {deviceDone ? (
              <div className="flex flex-col items-center gap-2 rounded-lg border border-[var(--color-success)]/40 bg-[color-mix(in_srgb,var(--color-success)_8%,transparent)] px-4 py-6 text-center">
                <Check size={28} className="text-[var(--color-success)]" />
                <p className="text-sm font-medium text-[var(--color-text)]">{t('auth.success')}</p>
                <p className="text-xs text-[var(--color-text-dim)]">
                  {t('auth.codex.successDetail')}
                </p>
                <Button
                  onClick={() => {
                    onLoggedIn()
                    onClose()
                  }}
                  size="lg"
                  className="mt-2"
                >
                  {t('shared.close')}
                </Button>
              </div>
            ) : !verifyUrl ? (
              <>
                <p className="text-xs text-[var(--color-text-dim)]">
                  {t('auth.codex.browserDescription')}
                </p>
                <Button onClick={startDeviceLogin} size="lg" disabled={deviceBusy}>
                  {deviceBusy ? (
                    <Loader2 size={15} className="animate-spin" />
                  ) : (
                    <Globe size={15} />
                  )}
                  {deviceBusy ? t('auth.connecting') : t('auth.startBrowser')}
                </Button>
              </>
            ) : (
              <div className="space-y-2">
                <div className="flex items-center gap-2 text-xs text-[var(--color-text-dim)]">
                  <span className="flex h-5 w-5 items-center justify-center rounded-full bg-[var(--color-accent)]/15 text-[10px] font-semibold text-[var(--color-accent)]">
                    1
                  </span>
                  {t('auth.codex.stepOpen')}
                  <a
                    href={verifyUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="ml-auto inline-flex items-center gap-1 text-[var(--color-accent)] hover:underline"
                  >
                    <ExternalLink size={12} /> {t('auth.reopen')}
                  </a>
                </div>
                <div className="flex items-center gap-2 text-xs text-[var(--color-text-dim)]">
                  <span className="flex h-5 w-5 items-center justify-center rounded-full bg-[var(--color-accent)]/15 text-[10px] font-semibold text-[var(--color-accent)]">
                    2
                  </span>
                  {t('auth.codex.stepCode')}
                </div>
                <div className="flex items-stretch gap-1">
                  <code
                    data-testid="codex-device-code"
                    className="flex-1 overflow-x-auto whitespace-nowrap rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-center text-base font-semibold tracking-widest"
                  >
                    {code}
                  </code>
                  <button
                    onClick={copyCode}
                    title={t('auth.codex.copyCode')}
                    className="shrink-0 rounded-lg border border-[var(--color-border)] px-2 hover:bg-[var(--color-surface-2)]"
                  >
                    {copied ? (
                      <Check size={14} className="text-[var(--color-success)]" />
                    ) : (
                      <Copy size={14} />
                    )}
                  </button>
                </div>
                <div className="flex items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-3 text-xs text-[var(--color-text-dim)]">
                  <Loader2 size={14} className="shrink-0 animate-spin text-[var(--color-accent)]" />
                  <span className="flex-1">{t('auth.codex.waiting')}</span>
                  <button
                    onClick={cancelDeviceLogin}
                    className="shrink-0 text-[var(--color-danger)] hover:underline"
                  >
                    {t('shared.cancel')}
                  </button>
                </div>
              </div>
            )}
            {deviceErr && (
              <div className="space-y-2">
                <p className="text-xs text-[var(--color-danger)]">{deviceErr}</p>
                <Button onClick={retryDeviceLogin} size="lg" disabled={deviceBusy}>
                  {t('shared.retry')}
                </Button>
              </div>
            )}
          </div>
        ) : (
          <div className="mb-2 space-y-2">
            <p className="text-xs text-[var(--color-text-dim)]">
              {t('auth.codex.apiKeyDescription')}
            </p>
            <label className="mb-1 block text-xs text-[var(--color-text-dim)]">
              {t('auth.codex.apiKeyLabel')}
            </label>
            <input
              ref={apiKeyRef}
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && submitApiKey()}
              placeholder="sk-...'"
              className={`${inputCls} w-full`}
              data-testid="codex-auth-apikey-input"
            />
            {apiErr && <p className="mt-1 text-xs text-[var(--color-danger)]">{apiErr}</p>}
            <div className="mt-3 flex justify-end gap-2">
              <button
                onClick={close}
                className="rounded-lg px-3 py-2 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]"
              >
                {t('shared.cancel')}
              </button>
              <Button onClick={submitApiKey} size="lg" disabled={apiBusy}>
                {apiBusy ? t('shared.saving') : t('auth.signIn')}
              </Button>
            </div>
          </div>
        )}
      </div>
    </ModalOverlay>
  )
}
