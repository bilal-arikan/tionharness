// CodexAuthDialog is the popup for logging the codex-cli provider into THIS
// workspace's isolated CODEX_HOME. Two methods, mirroring ClaudeAuthDialog:
//   - Device login: `codex login --device-auth` — show a verification URL +
//     one-time code, poll status until the user approves in their browser.
//   - API key: piped to `codex login --with-api-key` over stdin, never logged.
import { useEffect, useRef, useState } from 'react'
import { KeyRound, Globe, ExternalLink, Loader2, Check, Copy } from 'lucide-react'
import { api } from '@/api'
import { copyToClipboard } from '@/shared/lib/clipboard'
import { Button, ModalOverlay, toast } from '@/shared/components'
import { inputCls } from './primitives'

type Method = 'device' | 'apikey'

interface Props {
  isLoggedIn: boolean
  onClose: () => void
  onLoggedIn: () => void
}

// codexDeviceErrorLabel maps a device-flow terminal state to a Turkish message.
function codexDeviceErrorLabel(state: string, detail?: string): string {
  switch (state) {
    case 'expired':
      return 'Kodun süresi doldu. Tekrar dene.'
    case 'cancelled':
      return 'Giriş iptal edildi.'
    case 'failed':
    default:
      return detail || 'Giriş tamamlanamadı.'
  }
}

export function CodexAuthDialog({ isLoggedIn, onClose, onLoggedIn }: Props) {
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
        const r = await api.codexDeviceAuthStatus()
        if (r.state === 'success') {
          stopPolling()
          setDeviceDone(true)
        } else if (r.state === 'failed' || r.state === 'expired' || r.state === 'cancelled') {
          stopPolling()
          setDeviceErr(codexDeviceErrorLabel(r.state, r.error))
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
    try {
      const r = await api.startCodexDeviceAuth()
      setVerifyUrl(r.verifyUrl)
      setCode(r.code)
      window.open(r.verifyUrl, '_blank', 'noopener,noreferrer')
      startPolling()
    } catch (e) {
      const msg = (e as Error).message
      if (msg.includes('409')) {
        setDeviceErr('Bu workspace için zaten devam eden bir giriş var.')
      } else if (msg.includes('400')) {
        setDeviceErr('codex CLI bulunamadı (Sağlayıcılar altında yolunu belirt).')
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
      await api.cancelCodexDeviceAuth()
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
      await api.cancelCodexDeviceAuth()
    } catch {
      /* best-effort */
    }
    startDeviceLogin()
  }

  const copyCode = async () => {
    if (await copyToClipboard(code)) {
      setCopied(true)
      toast.info('Panoya kopyalandı')
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
      setApiErr('API anahtarı gerekli')
      apiKeyRef.current?.focus()
      return
    }
    setApiBusy(true)
    setApiErr(null)
    try {
      await api.codexAPIKeyLogin(k)
      toast.success('Giriş yapıldı')
      onLoggedIn()
      onClose()
    } catch (e) {
      const msg = (e as Error).message
      setApiErr(
        msg.includes('400') ? 'codex CLI bulunamadı (Sağlayıcılar altında yolunu belirt).' : msg,
      )
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
        aria-label="codex-cli kimlik doğrulama"
        data-testid="codex-auth-modal"
        className="w-full max-w-lg rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 shadow-2xl"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <h2 className="mb-1 text-base font-semibold">codex-cli kimlik doğrulama</h2>
        <p className="mb-4 text-xs text-[var(--color-text-dim)]">
          Bu workspace'in izole codex-home'unu yetkilendir — terminal gerekmez.
          {isLoggedIn && (
            <span className="ml-1 text-[var(--color-success)]">✓ Şu an giriş yapılmış.</span>
          )}
        </p>

        {/* Method tabs */}
        <div className="mb-4 flex gap-2">
          <button onClick={() => setMethod('device')} className={tabCls('device')}>
            <Globe size={14} /> Tarayıcıyla giriş
          </button>
          <button onClick={() => setMethod('apikey')} className={tabCls('apikey')}>
            <KeyRound size={14} /> API anahtarı
          </button>
        </div>

        {method === 'device' ? (
          <div className="mb-2 space-y-3">
            {deviceDone ? (
              <div className="flex flex-col items-center gap-2 rounded-lg border border-[var(--color-success)]/40 bg-[color-mix(in_srgb,var(--color-success)_8%,transparent)] px-4 py-6 text-center">
                <Check size={28} className="text-[var(--color-success)]" />
                <p className="text-sm font-medium text-[var(--color-text)]">Giriş başarılı</p>
                <p className="text-xs text-[var(--color-text-dim)]">
                  Bu workspace'in codex-home'una kimlik yazıldı.
                </p>
                <Button
                  onClick={() => {
                    onLoggedIn()
                    onClose()
                  }}
                  size="lg"
                  className="mt-2"
                >
                  Kapat
                </Button>
              </div>
            ) : !verifyUrl ? (
              <>
                <p className="text-xs text-[var(--color-text-dim)]">
                  ChatGPT hesabınla tarayıcıdan giriş yap. Bir doğrulama kodu üretilir; kodu
                  tarayıcıda onaylayınca kimlik doğrudan bu workspace'in codex-home'una yazılır.
                </p>
                <Button onClick={startDeviceLogin} size="lg" disabled={deviceBusy}>
                  {deviceBusy ? (
                    <Loader2 size={15} className="animate-spin" />
                  ) : (
                    <Globe size={15} />
                  )}
                  {deviceBusy ? 'Bağlantı alınıyor…' : 'Giriş başlat (tarayıcıyı aç)'}
                </Button>
              </>
            ) : (
              <div className="space-y-2">
                <div className="flex items-center gap-2 text-xs text-[var(--color-text-dim)]">
                  <span className="flex h-5 w-5 items-center justify-center rounded-full bg-[var(--color-accent)]/15 text-[10px] font-semibold text-[var(--color-accent)]">
                    1
                  </span>
                  Tarayıcıda sayfayı aç.
                  <a
                    href={verifyUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="ml-auto inline-flex items-center gap-1 text-[var(--color-accent)] hover:underline"
                  >
                    <ExternalLink size={12} /> Tekrar aç
                  </a>
                </div>
                <div className="flex items-center gap-2 text-xs text-[var(--color-text-dim)]">
                  <span className="flex h-5 w-5 items-center justify-center rounded-full bg-[var(--color-accent)]/15 text-[10px] font-semibold text-[var(--color-accent)]">
                    2
                  </span>
                  Aşağıdaki kodu gir ve onayla:
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
                    title="Kodu kopyala"
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
                  <span className="flex-1">
                    Tarayıcıda onaylamanı bekliyorum — onaylayınca otomatik döner.
                  </span>
                  <button
                    onClick={cancelDeviceLogin}
                    className="shrink-0 text-[var(--color-danger)] hover:underline"
                  >
                    İptal
                  </button>
                </div>
              </div>
            )}
            {deviceErr && (
              <div className="space-y-2">
                <p className="text-xs text-[var(--color-danger)]">{deviceErr}</p>
                <Button onClick={retryDeviceLogin} size="lg" disabled={deviceBusy}>
                  Tekrar dene
                </Button>
              </div>
            )}
          </div>
        ) : (
          <div className="mb-2 space-y-2">
            <p className="text-xs text-[var(--color-text-dim)]">
              platform.openai.com'dan bir API anahtarı yapıştır. Anahtar bu workspace'in
              codex-home'una yazılır, uygulamaya asla geri gösterilmez.
            </p>
            <label className="mb-1 block text-xs text-[var(--color-text-dim)]">API anahtarı</label>
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
                İptal
              </button>
              <Button onClick={submitApiKey} size="lg" disabled={apiBusy}>
                {apiBusy ? 'Kaydediliyor…' : 'Giriş yap'}
              </Button>
            </div>
          </div>
        )}
      </div>
    </ModalOverlay>
  )
}
