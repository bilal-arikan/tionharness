import { useCallback, useEffect, useState } from 'react'
import { Copy, X } from 'lucide-react'
import type { SessionContextPreview } from '../../types'
import { api } from '../../api'
import { Markdown } from '../markdown/Markdown'
import { Button } from '../common'

interface Props {
  sessionId: string
  title?: string
  onClose: () => void
}

// SessionContextModal previews the EXACT next-turn context a session's agent would
// be sent — the composed system prompt + dynamic suffix, the full message
// transcript (with author labels + tool recap folded in) and the tool catalog,
// each with a token estimate. A debug view: an optional sample message shows what
// the agent would receive if that were sent next. Read-only — no turn is run.
export function SessionContextModal({ sessionId, title, onClose }: Props) {
  const [data, setData] = useState<SessionContextPreview | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [message, setMessage] = useState('')
  const [loading, setLoading] = useState(false)

  const load = useCallback(
    (msg: string) => {
      setLoading(true)
      api
        .sessionContextPreview(sessionId, msg.trim() || undefined)
        .then(setData)
        .catch((e) => setErr((e as Error).message))
        .finally(() => setLoading(false))
    },
    [sessionId],
  )

  useEffect(() => load(''), [load])

  const copy = () => {
    if (!data) return
    const transcript = data.messages
      .map((m) => {
        const who = m.author ? ` (${m.role === 'user' ? '→ ' : ''}${m.author}${m.self && m.role !== 'user' ? ', siz' : ''})` : ''
        return `### ${m.role}${who}\n${m.text}`
      })
      .join('\n\n')
    const full = `# System\n${data.system}\n\n# Dynamic\n${data.dynamic}\n\n# Messages\n${transcript}`
    navigator.clipboard.writeText(full).then(() => {
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
        role="dialog"
        aria-modal="true"
        aria-label="Oturum bağlamı"
        data-testid="session-context-modal"
        className="flex max-h-[85vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-[var(--shadow-lg)]"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center gap-3 border-b border-[var(--color-border)] px-5 py-3">
          <div className="min-w-0 flex-1">
            <h2 className="truncate text-sm font-semibold">
              Sıradaki tur bağlam önizleme{title ? ` — ${title}` : ''}
            </h2>
            <p className="text-xs text-[var(--color-text-dim)]">
              {data ? `${data.agentName} bu oturumda bir sonraki turda alacağı tam istek` : 'Yükleniyor…'}
              {' · '}salt-okunur (tur çalıştırılmaz)
            </p>
          </div>
          {data && (
            <button
              onClick={copy}
              className="flex shrink-0 items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
            >
              <Copy size={13} /> {copied ? 'Kopyalandı' : 'Bağlamı kopyala'}
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
            <Stat label="Sistem" value={data.systemTokens} />
            <Stat label="Dinamik" value={data.dynamicTokens} />
            <Stat label={`Mesajlar (${data.messages.length})`} value={data.messageTokens} />
            <Stat label={`Araçlar (${data.tools.length})`} value={data.toolTokens} />
            {data.multiAgent && (
              <span className="rounded-md bg-[var(--color-accent-soft)] px-2 py-1 text-[var(--color-accent)]">
                çok-ajanlı
              </span>
            )}
            <span className="text-[var(--color-text-dim)]">~token tahmini</span>
          </div>
        )}

        {/* Sample "next" message */}
        <div className="flex items-center gap-2 border-b border-[var(--color-border)] px-5 py-2">
          <input
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && load(message)}
            placeholder="Örnek 'sıradaki' kullanıcı mesajı → bu mesaj gönderilseydi bağlam nasıl olurdu"
            className="min-w-0 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-xs outline-none focus:border-[var(--color-accent)]"
          />
          <Button onClick={() => load(message)} disabled={loading} className="shrink-0">
            {loading ? '…' : 'Önizle'}
          </Button>
        </div>

        {/* Body */}
        <div className="min-h-0 flex-1 overflow-y-auto p-5">
          {err && <p className="text-sm text-[var(--color-danger)]">{err}</p>}
          {!err && !data && <p className="text-sm text-[var(--color-text-dim)]">Yükleniyor…</p>}
          {data && (
            <>
              <Section title="Sistem promptu">
                <Markdown>{data.system || '(boş)'}</Markdown>
              </Section>
              <Section title="Dinamik bağlam">
                {data.dynamic ? <Markdown>{data.dynamic}</Markdown> : <Dim>(boş)</Dim>}
              </Section>

              <h3 className="mb-1.5 mt-4 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                Mesaj dizisi (modele gidecek) · {data.messages.length}
              </h3>
              {data.messages.length === 0 ? (
                <Dim>Henüz mesaj yok.</Dim>
              ) : (
                <div className="space-y-2">
                  {data.messages.map((m, i) => (
                    <div
                      key={i}
                      className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2"
                    >
                      <div className="mb-1 flex items-center gap-1.5">
                        <span className="text-[10px] font-semibold uppercase tracking-wide text-[var(--color-accent)]">
                          {m.role}
                        </span>
                        {m.author && (
                          <span
                            title={
                              m.role === 'user'
                                ? `Hedef ajan: ${m.author}`
                                : `Yazan ajan: ${m.author}`
                            }
                            className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] font-medium text-[var(--color-text-dim)]"
                          >
                            {m.role === 'user' ? `→ ${m.author}` : m.author}
                            {m.self && m.role !== 'user' && ' (siz)'}
                          </span>
                        )}
                      </div>
                      <pre className="overflow-x-auto whitespace-pre-wrap break-words text-xs text-[var(--color-text)]">
                        {m.text || '(boş)'}
                      </pre>
                    </div>
                  ))}
                </div>
              )}

              <h3 className="mb-1.5 mt-4 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                Araçlar — her tur şema gönderilen · {data.tools.length}
              </h3>
              {data.tools.length === 0 ? (
                <Dim>Bu ajana şema gönderilen araç yok.</Dim>
              ) : (
                <ul className="flex flex-wrap gap-1">
                  {data.tools.map((t) => (
                    <li
                      key={t.name}
                      title={t.description}
                      className="rounded border border-[var(--color-border)] px-1.5 py-0.5 text-[11px] text-[var(--color-text-dim)]"
                    >
                      {t.name}
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

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="mb-4">
      <h3 className="mb-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        {title}
      </h3>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-1">
        {children}
      </div>
    </div>
  )
}

function Dim({ children }: { children: React.ReactNode }) {
  return <p className="text-xs text-[var(--color-text-dim)]">{children}</p>
}

function Stat({ label, value, accent }: { label: string; value: number; accent?: boolean }) {
  const cls = accent
    ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
    : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
  return (
    <span className={`rounded-md px-2 py-1 ${cls}`}>
      {label}: <strong>{value.toLocaleString()}</strong>
    </span>
  )
}
