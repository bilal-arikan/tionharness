import { useEffect, useState } from 'react'
import { Copy, X } from 'lucide-react'
import type { AgentContextPreview } from '../../types'
import { api } from '../../api'
import { Markdown } from '../markdown/Markdown'

interface Props {
  agentId: string
  agentName: string
  onClose: () => void
}

// AgentContextModal previews the exact context an agent starts a fresh turn with:
// the assembled static system prompt and the tool catalog it is offered. The
// dynamic suffix (memory recall, running summary, session artifacts) is added
// per-turn from the conversation, so it is not shown here.
export function AgentContextModal({ agentId, agentName, onClose }: Props) {
  const [data, setData] = useState<AgentContextPreview | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [raw, setRaw] = useState(false)

  useEffect(() => {
    api
      .agentContext(agentId)
      .then(setData)
      .catch((e) => setErr((e as Error).message))
  }, [agentId])

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
            <span className="text-[var(--color-text-dim)]">
              ~token tahmini · dinamik kısım (hafıza/özet/artifact) tur anında eklenir
            </span>
          </div>
        )}

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

              <h3 className="mb-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                Araçlar · {data.tools.length}
              </h3>
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
