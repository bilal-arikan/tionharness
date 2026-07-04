// ClaudeAuthDialog is the popup for giving the claude-cli provider a credential
// so an isolated CLAUDE_CONFIG_DIR authenticates without an interactive in-dir
// `claude login`. Two methods:
//   - Max/Pro (OAuth): a one-year token from `claude setup-token`, injected as
//     CLAUDE_CODE_OAUTH_TOKEN (uses the subscription, no API billing).
//   - API key: an sk-ant-... key, injected as ANTHROPIC_API_KEY (API billing).
// The token is stored AES-GCM encrypted server-side and never returned.
import { useEffect, useRef, useState } from 'react'
import { KeyRound, Sparkles, Copy, Check } from 'lucide-react'
import { api } from '../../api'
import { copyToClipboard } from '../../lib/clipboard'
import type { AppSettings } from '../../types'
import { Button, ModalOverlay } from '../common'
import { inputCls } from './primitives'

type Method = 'oauth' | 'apikey'

interface Props {
  configDir: string // current claudeConfigDir (shown in the setup-token command)
  currentKind: string // existing stored kind ("oauth" | "apikey" | "")
  isSet: boolean // whether a token is already stored
  onClose: () => void
  onSaved: (next: AppSettings) => void
}

export function ClaudeAuthDialog({ configDir, currentKind, isSet, onClose, onSaved }: Props) {
  const [method, setMethod] = useState<Method>(currentKind === 'apikey' ? 'apikey' : 'oauth')
  const [token, setToken] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const tokenRef = useRef<HTMLInputElement>(null)

  // Focus the token field on open. Escape-to-close is handled by ModalOverlay.
  useEffect(() => {
    tokenRef.current?.focus()
  }, [])

  // The exact command the user runs once to mint a subscription OAuth token. The
  // CLAUDE_CONFIG_DIR prefix is only needed so the login lands in the same isolated
  // dir SwarmGo drives (harmless if empty → omit it).
  const setupCmd = configDir
    ? `$env:CLAUDE_CONFIG_DIR="${configDir}"; claude setup-token`
    : 'claude setup-token'

  const copyCmd = async () => {
    if (await copyToClipboard(setupCmd)) {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    }
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
          İzole config dizininde ayrı login yapmadan claude-cli'yi yetkilendir. Token şifreli saklanır ve
          subprocess'e ortam değişkeni olarak verilir.
          {isSet && (
            <span className="ml-1 text-[var(--color-success)]">
              ✓ Şu an kayıtlı{currentKind ? ` (${currentKind === 'oauth' ? 'Max/Pro' : 'API'})` : ''}.
            </span>
          )}
        </p>

        {/* Method tabs */}
        <div className="mb-4 flex gap-2">
          <button onClick={() => setMethod('oauth')} className={tabCls('oauth')}>
            <Sparkles size={14} /> Max / Pro (abonelik)
          </button>
          <button onClick={() => setMethod('apikey')} className={tabCls('apikey')}>
            <KeyRound size={14} /> API anahtarı
          </button>
        </div>

        {method === 'oauth' ? (
          <div className="mb-4 space-y-2">
            <p className="text-xs text-[var(--color-text-dim)]">
              1) Bir terminalde aşağıdaki komutu çalıştır — tarayıcıda Max/Pro hesabınla giriş yap, 1 yıllık
              token üretilir.
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
                {copied ? <Check size={14} className="text-[var(--color-success)]" /> : <Copy size={14} />}
              </button>
            </div>
            <p className="text-xs text-[var(--color-text-dim)]">2) Çıkan token'ı aşağıya yapıştır:</p>
          </div>
        ) : (
          <p className="mb-2 text-xs text-[var(--color-text-dim)]">
            console.anthropic.com'dan bir API anahtarı (<code>sk-ant-…</code>) yapıştır. Bu yöntem aboneliği
            değil API kredisini kullanır.
          </p>
        )}

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
      </div>
    </ModalOverlay>
  )
}
