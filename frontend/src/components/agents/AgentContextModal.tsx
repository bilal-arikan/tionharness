import { useCallback, useEffect, useState } from 'react'
import { Copy, X } from 'lucide-react'
import type { AgentContextPreview } from '../../types'
import { api } from '../../api'
import { Markdown } from '../markdown/Markdown'

interface Props {
  agentId: string
  agentName: string
  onClose: () => void
}

// AgentContextModal previews the context an agent starts a turn with: the static
// system prompt + the tool catalog. With an optional sample message it also
// simulates the message-dependent dynamic suffix (recalled memory + cross-session
// block); session-only parts (summary/artifacts/todos) need a live session.
export function AgentContextModal({ agentId, agentName, onClose }: Props) {
  const [data, setData] = useState<AgentContextPreview | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [raw, setRaw] = useState(false)
  const [message, setMessage] = useState('')
  const [loading, setLoading] = useState(false)

  const load = useCallback(
    (msg: string) => {
      setLoading(true)
      api
        .agentContext(agentId, msg.trim() || undefined)
        .then(setData)
        .catch((e) => setErr((e as Error).message))
        .finally(() => setLoading(false))
    },
    [agentId],
  )

  useEffect(() => load(''), [load])

  const copy = () => {
    if (!data) return
    navigator.clipboard.writeText(data.system).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-6"
      onClick={onClose}
    >
      <div
        className="flex max-h-[85vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-[var(--shadow-lg)]"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center gap-3 border-b border-[var(--color-border)] px-5 py-3">
          <div className="min-w-0 flex-1">
            <h2 className="truncate text-sm font-semibold">Bağlam önizleme — {agentName}</h2>
            <p className="text-xs text-[var(--color-text-dim)]">
              Ajanın sıfırdan (oturum yokken) bir tura başlarken aldığı sistem promptu + araçlar
            </p>
          </div>
          {data && (
            <button
              onClick={copy}
              className="flex shrink-0 items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
            >
              <Copy size={13} /> {copied ? 'Kopyalandı' : 'Promptu kopyala'}
            </button>
          )}
          <button
            onClick={onClose}
            className="shrink-0 rounded-md p-1.5 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
          >
            <X size={16} />
          </button>
        </div>

        {/* Token summary */}
        {data && (
          <div className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] px-5 py-2 text-xs">
            <Stat label="Toplam" value={data.totalTokens} accent />
            <Stat label="Sistem promptu" value={data.systemTokens} />
            <Stat label={`Araçlar (${data.tools.length})`} value={data.toolTokens} />
            <Stat label="Dinamik" value={data.dynamicTokens} />
            <span className="text-[var(--color-text-dim)]">~token tahmini</span>
          </div>
        )}

        {/* Sample message → simulate the dynamic suffix */}
        <div className="flex items-center gap-2 border-b border-[var(--color-border)] px-5 py-2">
          <input
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && load(message)}
            placeholder="Örnek mesaj yaz → bu mesaj için hafıza recall + çapraz-oturum bağlamı simüle edilir"
            className="min-w-0 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-xs outline-none focus:border-[var(--color-accent)]"
          />
          <button
            onClick={() => load(message)}
            disabled={loading}
            className="shrink-0 rounded bg-[var(--color-accent)] px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
          >
            {loading ? '…' : 'Simüle et'}
          </button>
        </div>

        {/* Body */}
        <div className="min-h-0 flex-1 overflow-y-auto p-5">
          {err && <p className="text-sm text-[var(--color-danger)]">{err}</p>}
          {!err && !data && <p className="text-sm text-[var(--color-text-dim)]">Yükleniyor…</p>}
          {data && (
            <>
              <div className="mb-1.5 flex items-center justify-between gap-2">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                  Sistem promptu
                </h3>
                <div className="flex items-center overflow-hidden rounded-md border border-[var(--color-border)] text-[11px]">
                  {(['Markdown', 'Ham'] as const).map((mode) => {
                    const isRaw = mode === 'Ham'
                    const activeMode = raw === isRaw
                    return (
                      <button
                        key={mode}
                        onClick={() => setRaw(isRaw)}
                        className={`px-2 py-0.5 ${
                          activeMode
                            ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                            : 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
                        }`}
                      >
                        {mode}
                      </button>
                    )
                  })}
                </div>
              </div>
              {raw ? (
                <pre className="mb-5 whitespace-pre-wrap break-words rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-3 font-mono text-xs leading-relaxed text-[var(--color-text)]">
                  {data.system || '(boş)'}
                </pre>
              ) : (
                <div className="mb-5 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-1">
                  <Markdown>{data.system || '(boş)'}</Markdown>
                </div>
              )}

              {/* Dynamic suffix (simulated). */}
              <h3 className="mb-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                Dinamik bağlam {message.trim() ? '(örnek mesaja göre)' : ''}
              </h3>
              {data.dynamic ? (
                raw ? (
                  <pre className="mb-5 whitespace-pre-wrap break-words rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-3 font-mono text-xs leading-relaxed text-[var(--color-text)]">
                    {data.dynamic}
                  </pre>
                ) : (
                  <div className="mb-5 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-1">
                    <Markdown>{data.dynamic}</Markdown>
                  </div>
                )
              ) : (
                <p className="mb-5 text-xs text-[var(--color-text-dim)]">
                  Bu mesaj için recall yok. Özet · oturum artifact'ları · todo listesi gerçek bir
                  oturumda, tur anında eklenir (burada simüle edilmez).
                </p>
              )}

              <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                Araçlar — her tur gönderilen · {data.tools.length}
              </h3>
              <p className="mb-1.5 text-[11px] text-[var(--color-text-dim)]">
                Bunların tam şeması her tura girer. Talep-üzerine (lazy) araçlar — self-management ve
                MCP — yalnızca sistem promptundaki{' '}
                <code className="rounded bg-[var(--color-surface-2)] px-1">Available Tools (load on demand)</code>{' '}
                bölümünde özetle durur; ajan <code className="rounded bg-[var(--color-surface-2)] px-1">activate_tools</code>{' '}
                ile yükler.
              </p>
              {data.tools.length === 0 ? (
                <p className="text-xs text-[var(--color-text-dim)]">Bu ajana araç sunulmuyor.</p>
              ) : (
                <ul className="space-y-1">
                  {data.tools.map((t) => (
                    <li
                      key={t.name}
                      className="rounded-md border border-[var(--color-border)] px-2.5 py-1.5"
                    >
                      <code className="text-xs font-medium text-[var(--color-accent)]">{t.name}</code>
                      <p className="mt-0.5 text-[11px] text-[var(--color-text-dim)]">{t.description}</p>
                    </li>
                  ))}
                </ul>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  )
}

function Stat({ label, value, accent }: { label: string; value: number; accent?: boolean }) {
  return (
    <span
      className={`rounded-md px-2 py-1 ${
        accent
          ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
          : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
      }`}
    >
      {label}: <strong>{value.toLocaleString()}</strong>
    </span>
  )
}
