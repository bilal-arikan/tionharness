// ClaudeAuthDialog is the popup for giving the claude-cli provider a credential
// so an isolated CLAUDE_CONFIG_DIR authenticates without an interactive in-dir
// `claude login`. Two methods:
//   - Max/Pro (OAuth): a one-year token from `claude setup-token`, injected as
//     CLAUDE_CODE_OAUTH_TOKEN (uses the subscription, no API billing).
//   - API key: an sk-ant-... key, injected as ANTHROPIC_API_KEY (API billing).
// The token is stored AES-GCM encrypted server-side and never returned.
import { useEffect, useRef, useState } from 'react'
import { KeyRound, Sparkles, Copy, Check, Globe, ExternalLink, Loader2 } from 'lucide-react'
import { api } from '@/api'
import { copyToClipboard } from '@/shared/lib/clipboard'
import type { AppSettings } from '@/types'
import { Button, ModalOverlay, toast } from '@/shared/components'
import { inputCls } from './primitives'

// 'browser'  → in-app OAuth: open the auth URL, paste the code back (no terminal).
// 'oauth'    → manual: run `claude setup-token` yourself, paste the resulting token.
// 'apikey'   → paste an sk-ant-... API key (API billing).
type Method = 'browser' | 'oauth' | 'apikey'

// isRemoteAccess reports whether the app is being viewed from a different machine
// than the backend runs on (e.g. a VPS reached over the network). The automatic
// (loopback) OAuth sub-mode binds an ephemeral 127.0.0.1 listener ON THE BACKEND and
// hands the browser a http://localhost:<port>/callback redirect — which resolves to
// the *viewer's* machine, not the backend, when they differ. So loopback only works
// when browser and backend share a host; remote access must use the paste flow.
function isRemoteAccess(): boolean {
  const h = typeof window !== 'undefined' ? window.location.hostname : 'localhost'
  return !['localhost', '127.0.0.1', '::1', '[::1]', ''].includes(h)
}

interface Props {
  configDir: string // current claudeConfigDir (shown in the setup-token command)
  currentKind: string // existing stored kind ("oauth" | "apikey" | "")
  isSet: boolean // whether a token is already stored
  onClose: () => void
  onSaved: (next: AppSettings) => void
}

export function ClaudeAuthDialog({ configDir, currentKind, isSet, onClose, onSaved }: Props) {
  const [method, setMethod] = useState<Method>(currentKind === 'apikey' ? 'apikey' : 'browser')
  const [token, setToken] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const tokenRef = useRef<HTMLInputElement>(null)

  // In-app OAuth (browser) flow state: request an auth URL, open it, paste the
  // "<code>#<state>" the callback page shows, exchange it server-side.
  const [flowId, setFlowId] = useState('')
  const [authUrl, setAuthUrl] = useState('')
  const [oauthCode, setOauthCode] = useState('')
  const [oauthBusy, setOauthBusy] = useState(false)
  const [oauthErr, setOauthErr] = useState<string | null>(null)
  const [oauthDone, setOauthDone] = useState(false)
  // 'auto'   → loopback: browser redirects back to a local listener, no paste.
  // 'manual' → paste the "<code>#<state>" from the callback page.
  // Default to manual when accessed remotely (VPS): loopback's localhost redirect
  // would land on the viewer's machine, not the backend, and silently fail.
  const remote = isRemoteAccess()
  const [browserMode, setBrowserMode] = useState<'auto' | 'manual'>(remote ? 'manual' : 'auto')
  const [loopbackFlowId, setLoopbackFlowId] = useState('')

  // Poll the loopback flow's status once started, until it flips to ok/error.
  useEffect(() => {
    if (!loopbackFlowId || oauthDone) return
    let alive = true
    const t = setInterval(async () => {
      try {
        const r = await api.claudeOAuthLoopbackStatus(loopbackFlowId)
        if (!alive) return
        if (r.status === 'ok') {
          setOauthDone(true)
          setLoopbackFlowId('')
        } else if (r.status === 'error' || r.status === 'unknown') {
          setOauthErr(r.detail || 'Giriş tamamlanamadı')
          setLoopbackFlowId('')
        }
      } catch {
        /* transient — keep polling */
      }
    }, 1500)
    return () => {
      alive = false
      clearInterval(t)
    }
  }, [loopbackFlowId, oauthDone])

  // Auto (loopback): bind a local callback, open the URL, then poll for completion.
  const startAutoLogin = async () => {
    setOauthBusy(true)
    setOauthErr(null)
    try {
      const { flowId, authUrl } = await api.startClaudeOAuthLoopback()
      setAuthUrl(authUrl)
      setLoopbackFlowId(flowId)
      window.open(authUrl, '_blank', 'noopener,noreferrer')
    } catch (e) {
      setOauthErr((e as Error).message)
    } finally {
      setOauthBusy(false)
    }
  }

  // Step 1: ask the backend for an authorization URL, then open it in the browser.
  const startBrowserLogin = async () => {
    setOauthBusy(true)
    setOauthErr(null)
    try {
      const { flowId, authUrl } = await api.startClaudeOAuth()
      setFlowId(flowId)
      setAuthUrl(authUrl)
      window.open(authUrl, '_blank', 'noopener,noreferrer')
    } catch (e) {
      setOauthErr((e as Error).message)
    } finally {
      setOauthBusy(false)
    }
  }

  // Step 2: exchange the pasted code for a credential written to this workspace's home.
  const completeBrowserLogin = async () => {
    const c = oauthCode.trim()
    if (!c) {
      setOauthErr('Tarayıcıdaki kodu yapıştır')
      return
    }
    setOauthBusy(true)
    setOauthErr(null)
    try {
      await api.completeClaudeOAuth(flowId, c)
      setOauthDone(true)
    } catch (e) {
      setOauthErr((e as Error).message)
    } finally {
      setOauthBusy(false)
    }
  }

  // Focus the token field on open. Escape-to-close is handled by ModalOverlay.
  useEffect(() => {
    tokenRef.current?.focus()
  }, [])

  // The exact command the user runs once to mint a subscription OAuth token. The
  // CLAUDE_CONFIG_DIR prefix is only needed so the login lands in the same isolated
  // dir TionSwarm drives (harmless if empty → omit it).
  const setupCmd = configDir
    ? `$env:CLAUDE_CONFIG_DIR="${configDir}"; claude setup-token`
    : 'claude setup-token'

  const copyCmd = async () => {
    if (await copyToClipboard(setupCmd)) toast.info('Panoya kopyalandı')
  }

  const save = async () => {
    const t = token.trim()
    if (!t) {
      setError('Token / anahtar gerekli')
      tokenRef.current?.focus()
      return
    }
    setBusy(true)
    setError(null)
    try {
      const next = await api.updateSettings({ claudeCliAuthKind: method, claudeCliAuthToken: t })
      onSaved(next)
      toast.success('Kaydedildi')
      onClose()
    } catch (e) {
      setError('Kaydedilemedi: ' + (e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const clear = async () => {
    setBusy(true)
    setError(null)
    try {
      const next = await api.updateSettings({ claudeCliAuthKind: '', claudeCliAuthToken: '' })
      onSaved(next)
      toast.success('Silindi')
      onClose()
    } catch (e) {
      setError('Silinemedi: ' + (e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const tabCls = (m: Method) =>
    `flex flex-1 items-center justify-center gap-1.5 rounded-lg border px-3 py-2 text-sm transition ${
      method === m
        ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)]'
        : 'border-[var(--color-border)] hover:bg-[var(--color-surface-2)]'
    }`

  return (
    <ModalOverlay onClose={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label="claude-cli kimlik doğrulama"
        data-testid="claude-auth-modal"
        className="w-full max-w-lg rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 shadow-2xl"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <h2 className="mb-1 text-base font-semibold">claude-cli kimlik doğrulama</h2>
        <p className="mb-4 text-xs text-[var(--color-text-dim)]">
          İzole config dizininde ayrı login yapmadan claude-cli'yi yetkilendir. Token şifreli
          saklanır ve subprocess'e ortam değişkeni olarak verilir.
          {isSet && (
            <span className="ml-1 text-[var(--color-success)]">
              ✓ Şu an kayıtlı
              {currentKind ? ` (${currentKind === 'oauth' ? 'Max/Pro' : 'API'})` : ''}.
            </span>
          )}
        </p>

        {/* Method tabs */}
        <div className="mb-4 flex gap-2">
          <button onClick={() => setMethod('browser')} className={tabCls('browser')}>
            <Globe size={14} /> Tarayıcıyla giriş
          </button>
          <button onClick={() => setMethod('oauth')} className={tabCls('oauth')}>
            <Sparkles size={14} /> Token yapıştır
          </button>
          <button onClick={() => setMethod('apikey')} className={tabCls('apikey')}>
            <KeyRound size={14} /> API anahtarı
          </button>
        </div>

        {method === 'browser' ? (
          <div className="mb-2 space-y-3">
            {oauthDone ? (
              <div className="flex flex-col items-center gap-2 rounded-lg border border-[var(--color-success)]/40 bg-[color-mix(in_srgb,var(--color-success)_8%,transparent)] px-4 py-6 text-center">
                <Check size={28} className="text-[var(--color-success)]" />
                <p className="text-sm font-medium text-[var(--color-text)]">Giriş başarılı</p>
                <p className="text-xs text-[var(--color-text-dim)]">
                  Bu workspace'in claude-home'una kimlik yazıldı. Yeni bir tur artık çalışmalı.
                </p>
                <Button onClick={onClose} size="lg" className="mt-2">
                  Kapat
                </Button>
              </div>
            ) : (
              <>
                <p className="text-xs text-[var(--color-text-dim)]">
                  Max/Pro aboneliğinle tarayıcıdan giriş yap — terminal gerekmez. Kimlik doğrudan bu
                  workspace'in claude-home'una yazılır.
                </p>
                {/* Auto (loopback) vs manual (paste) sub-mode */}
                <div className="flex gap-1 rounded-lg border border-[var(--color-border)] p-0.5 text-xs">
                  <button
                    onClick={() => setBrowserMode('auto')}
                    className={`flex-1 rounded-md px-2 py-1 ${browserMode === 'auto' ? 'bg-[var(--color-accent)]/15 text-[var(--color-accent)]' : 'text-[var(--color-text-dim)]'}`}
                  >
                    Otomatik (önerilen)
                  </button>
                  <button
                    onClick={() => setBrowserMode('manual')}
                    className={`flex-1 rounded-md px-2 py-1 ${browserMode === 'manual' ? 'bg-[var(--color-accent)]/15 text-[var(--color-accent)]' : 'text-[var(--color-text-dim)]'}`}
                  >
                    Elle kod
                  </button>
                </div>

                {browserMode === 'auto' && remote && (
                  <div className="rounded-lg border border-[var(--color-warning)]/40 bg-[color-mix(in_srgb,var(--color-warning)_8%,transparent)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
                    ⚠️ Uygulamaya uzaktan (ör. VPS) erişiyorsun. Otomatik giriş yalnız tarayıcı ile
                    sunucu aynı makinedeyken çalışır — dönüş adresi (<code>localhost</code>) senin
                    kendi cihazını işaret eder, sunucuyu değil. Uzaktan giriş için{' '}
                    <button
                      onClick={() => setBrowserMode('manual')}
                      className="font-medium text-[var(--color-accent)] hover:underline"
                    >
                      Elle kod
                    </button>{' '}
                    modunu kullan.
                  </div>
                )}
                {browserMode === 'auto' ? (
                  !loopbackFlowId ? (
                    <Button onClick={startAutoLogin} size="lg" disabled={oauthBusy}>
                      {oauthBusy ? (
                        <Loader2 size={15} className="animate-spin" />
                      ) : (
                        <Globe size={15} />
                      )}
                      {oauthBusy ? 'Bağlantı alınıyor…' : 'Giriş başlat (tarayıcıyı aç)'}
                    </Button>
                  ) : (
                    <div className="flex items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-3 text-xs text-[var(--color-text-dim)]">
                      <Loader2
                        size={14}
                        className="shrink-0 animate-spin text-[var(--color-accent)]"
                      />
                      <span className="flex-1">
                        Tarayıcıda giriş yapmanı bekliyorum — yetkilendirince otomatik döner.
                      </span>
                      <a
                        href={authUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="inline-flex items-center gap-1 text-[var(--color-accent)] hover:underline"
                      >
                        <ExternalLink size={12} /> Tekrar aç
                      </a>
                    </div>
                  )
                ) : !authUrl ? (
                  <Button onClick={startBrowserLogin} size="lg" disabled={oauthBusy}>
                    {oauthBusy ? (
                      <Loader2 size={15} className="animate-spin" />
                    ) : (
                      <Globe size={15} />
                    )}
                    {oauthBusy ? 'Bağlantı alınıyor…' : 'Giriş başlat (tarayıcıyı aç)'}
                  </Button>
                ) : (
                  <div className="space-y-2">
                    <div className="flex items-center gap-2 text-xs text-[var(--color-text-dim)]">
                      <span className="flex h-5 w-5 items-center justify-center rounded-full bg-[var(--color-accent)]/15 text-[10px] font-semibold text-[var(--color-accent)]">
                        1
                      </span>
                      Tarayıcıda giriş yapıp yetkilendir.
                      <a
                        href={authUrl}
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
                      Sayfadaki kodu kopyala ve aşağıya yapıştır:
                    </div>
                    <input
                      autoFocus
                      value={oauthCode}
                      onChange={(e) => setOauthCode(e.target.value)}
                      onKeyDown={(e) => e.key === 'Enter' && completeBrowserLogin()}
                      placeholder="kod#state"
                      className={`${inputCls} w-full font-mono`}
                      data-testid="claude-oauth-code-input"
                    />
                    <Button
                      onClick={completeBrowserLogin}
                      size="lg"
                      disabled={oauthBusy}
                      className="w-full"
                    >
                      {oauthBusy ? (
                        <Loader2 size={15} className="animate-spin" />
                      ) : (
                        <Check size={15} />
                      )}
                      {oauthBusy ? 'Doğrulanıyor…' : 'Girişi tamamla'}
                    </Button>
                  </div>
                )}
                {oauthErr && <p className="text-xs text-[var(--color-danger)]">{oauthErr}</p>}
              </>
            )}
          </div>
        ) : method === 'oauth' ? (
          <div className="mb-4 space-y-2">
            <p className="text-xs text-[var(--color-text-dim)]">
              1) Bir terminalde aşağıdaki komutu çalıştır — tarayıcıda Max/Pro hesabınla giriş yap,
              1 yıllık token üretilir.
            </p>
            <div className="flex items-stretch gap-1">
              <code className="flex-1 overflow-x-auto whitespace-nowrap rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-xs">
                {setupCmd}
              </code>
              <button
                onClick={copyCmd}
                title="Komutu kopyala"
                className="shrink-0 rounded-lg border border-[var(--color-border)] px-2 hover:bg-[var(--color-surface-2)]"
              >
                <Copy size={14} />
              </button>
            </div>
            <p className="text-xs text-[var(--color-text-dim)]">
              2) Çıkan token'ı aşağıya yapıştır:
            </p>
          </div>
        ) : (
          <p className="mb-2 text-xs text-[var(--color-text-dim)]">
            console.anthropic.com'dan bir API anahtarı (<code>sk-ant-…</code>) yapıştır. Bu yöntem
            aboneliği değil API kredisini kullanır.
          </p>
        )}

        {method !== 'browser' && (
          <>
            <label className="mb-1 block text-xs text-[var(--color-text-dim)]">
              {method === 'oauth' ? 'OAuth token' : 'API anahtarı'}
            </label>
            <input
              ref={tokenRef}
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && save()}
              placeholder={method === 'oauth' ? 'setup-token çıktısını yapıştır' : 'sk-ant-...'}
              className={`${inputCls} w-full`}
              data-testid="claude-auth-token-input"
            />

            {error && <p className="mt-3 text-xs text-[var(--color-danger)]">{error}</p>}

            <div className="mt-5 flex items-center justify-between gap-2">
              <div>
                {isSet && (
                  <button
                    onClick={clear}
                    disabled={busy}
                    className="rounded-lg border border-[var(--color-border)] px-3 py-2 text-xs text-[var(--color-danger)] hover:border-[var(--color-danger)] disabled:opacity-50"
                  >
                    Kayıtlı token'ı sil
                  </button>
                )}
              </div>
              <div className="flex gap-2">
                <button
                  onClick={onClose}
                  className="rounded-lg px-3 py-2 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]"
                >
                  İptal
                </button>
                <Button onClick={save} size="lg" disabled={busy}>
                  {busy ? 'Kaydediliyor…' : 'Kaydet'}
                </Button>
              </div>
            </div>
          </>
        )}
      </div>
    </ModalOverlay>
  )
}
