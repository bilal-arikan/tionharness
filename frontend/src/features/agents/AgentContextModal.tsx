import { useCallback, useEffect, useState } from 'react'
import { Check, ChevronRight, ChevronsDownUp, ChevronsUpDown, Copy, X } from 'lucide-react'
import type { AgentContextPreview } from '@/types'
import { api } from '@/api'
import { copyToClipboard } from '@/shared/lib/clipboard'
import { Markdown } from '@/shared/components/markdown/Markdown'
import { Button, CollapsibleSection, InfoPopover, ModalOverlay, useBulkToggle } from '@/shared/components'

// LAZY_VIS_CHIP labels a lazy tool's visibility tier next to its name so the
// load-on-demand list reflects the same Tam/Özet/İsim/Gizli chips set in the tools
// screen: "summary" keeps its description, "name-only"/"hidden" show the name alone.
const LAZY_VIS_CHIP: Record<string, string> = {
  summary: 'Özet',
  'name-only': 'İsim',
  hidden: 'Gizli',
}

interface Props {
  agentId: string
  agentName: string
  onClose: () => void
}

// AgentContextModal previews the context an agent starts a turn with: the static
// system prompt + the tool catalog. With an optional sample message it also
// simulates the message-dependent dynamic suffix (cross-session block);
// session-only parts (summary/artifacts/todos) need a live session.
export function AgentContextModal({ agentId, agentName, onClose }: Props) {
  const [data, setData] = useState<AgentContextPreview | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [raw, setRaw] = useState(false)
  const [message, setMessage] = useState('')
  const [loading, setLoading] = useState(false)
  // Token summary + CLI-overhead strip: collapsible, default collapsed.
  const [statsOpen, setStatsOpen] = useState(false)
  const { bulk, expandAll, collapseAll } = useBulkToggle(true)

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
    copyToClipboard(data.system).then((ok) => {
      if (!ok) return
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }

  // Lazy ("load on demand") tools are already named inside the previewed Context:
  // the system prompt's "Available Tools (load on demand)" block lists them. So we
  // don't re-list any lazy tool whose name already appears in the Context — that
  // separate list only duplicated what the Context already carries. (The eager
  // "schema sent every turn" tools are likewise part of the per-turn Context and
  // are not enumerated below at all; their count/cost stays in the summary chips.)
  const inContext = (name: string) => !!data && data.system.includes(name)
  const shownLazyTools = data ? data.lazyTools.filter((t) => !inContext(t.name)) : []

  return (
    <ModalOverlay onClose={onClose} padding="p-6">
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Ajan bağlamı"
        data-testid="agent-context-modal"
        className="flex max-h-[85vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-[var(--shadow-lg)]"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header — title with the copy + close actions inline beside it. */}
        <div className="flex items-center gap-2 border-b border-[var(--color-border)] px-5 py-3">
          <div className="min-w-0 flex-1">
            <h2 className="truncate text-sm font-semibold">Bağlam — {agentName}</h2>
            <p className="truncate text-xs text-[var(--color-text-dim)]">
              Ajanın sıfırdan (oturum yokken) bir tura başlarken aldığı sistem promptu + araçlar
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            {data && (
              <button
                onClick={copy}
                title={copied ? 'Kopyalandı' : 'Promptu kopyala'}
                aria-label={copied ? 'Kopyalandı' : 'Promptu kopyala'}
                className="flex shrink-0 items-center rounded-md border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
              >
                {copied ? <Check size={13} className="text-[var(--color-success)]" /> : <Copy size={13} />}
              </button>
            )}
            <button
              onClick={onClose}
              className="shrink-0 rounded-md p-1.5 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
            >
              <X size={16} />
            </button>
          </div>
        </div>

        {/* Token summary + CLI overhead — collapsible strip, default collapsed. */}
        {data && (
          <div className="border-b border-[var(--color-border)] text-xs">
            <button
              onClick={() => setStatsOpen((v) => !v)}
              aria-expanded={statsOpen}
              className="flex w-full items-center gap-2 px-5 py-2 text-left hover:bg-[var(--color-surface-2)]"
            >
              <ChevronRight
                size={14}
                className={`shrink-0 text-[var(--color-text-dim)] transition-transform ${statsOpen ? 'rotate-90' : ''}`}
              />
              <span className="font-medium">Token özeti</span>
              <span className="text-[var(--color-text-dim)]">
                · Toplam {data.totalTokens.toLocaleString()} <span className="opacity-70">(~tahmini)</span>
              </span>
              {data.cliOverhead && data.cliOverhead.predictedOverhead > 0 && (
                <span className="rounded bg-[color-mix(in_srgb,var(--color-warning)_18%,transparent)] px-1.5 py-0.5 font-medium text-[var(--color-warning)]">
                  CLI ek yükü
                </span>
              )}
            </button>

            {statsOpen && (
              <>
                <div className="flex flex-wrap items-center gap-2 px-5 pb-2">
                  <Stat label="Toplam" value={data.totalTokens} accent />
                  <Stat label="Sistem promptu" value={data.systemTokens} />
                  {data.skills && <Stat label="Skills" value={data.skillsTokens} />}
                  <Stat label={`Şema araçlar (${data.tools.length})`} value={data.toolTokens} />
                  {data.lazyTools.length > 0 && (
                    <Stat label={`Talep-üzerine (${data.lazyTools.length})`} value={0} dim />
                  )}
                  <Stat label="Dinamik" value={data.dynamicTokens} />
                  {data.cliOverhead && data.cliOverhead.predictedOverhead > 0 && (
                    <Stat
                      label="Beklenen taban (CLI)"
                      value={data.totalTokens + data.cliOverhead.predictedOverhead}
                      accent
                    />
                  )}
                  <span className="text-[var(--color-text-dim)]">~token tahmini</span>
                </div>

                {/* CLI-wrapper overhead: totalTokens under-reports for claude-cli.
                    Agent preview has no session → predicted-only. */}
                {data.cliOverhead && data.cliOverhead.predictedOverhead > 0 && (
                  <div className="bg-[color-mix(in_srgb,var(--color-warning)_8%,transparent)] px-5 py-2 text-[11px]">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="inline-flex items-center gap-1 rounded bg-[color-mix(in_srgb,var(--color-warning)_18%,transparent)] px-1.5 py-0.5 font-medium text-[var(--color-warning)]">
                        CLI ek yükü
                        <InfoPopover
                          text={`${data.cliOverhead.note}\n\n"Beklenen taban" = bir turun ALT SINIRI (yalnız CLI tabanı + eager araç şemaları). Gerçek girdi, biriken bağlam + aktive edilen deferred araçlarla bunu aşabilir; kesin değer ilk turdan sonra ölçülür.`}
                          label="CLI ek yükü nasıl hesaplanır?"
                        />
                      </span>
                      <span className="text-[var(--color-text-dim)]">
                        Tahmin <strong>{data.totalTokens.toLocaleString()}</strong> → beklenen taban ~
                        <strong>{(data.totalTokens + data.cliOverhead.predictedOverhead).toLocaleString()}</strong>
                        {' '}(+<strong>{data.cliOverhead.predictedOverhead.toLocaleString()}</strong> taban ek yük)
                      </span>
                    </div>
                  </div>
                )}
              </>
            )}
          </div>
        )}

        {/* Sample message → simulate the dynamic suffix */}
        <div className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] px-5 py-2">
          <input
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && load(message)}
            placeholder="Örnek mesaj yaz → bu mesaj için çapraz-oturum bağlamı simüle edilir"
            className="min-w-0 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-xs outline-none focus:border-[var(--color-accent)]"
          />
          {/* Expand/collapse-all (icon-only), sitting next to the simulate button. */}
          {data && <BulkButtons onExpand={expandAll} onCollapse={collapseAll} />}
          <Button onClick={() => load(message)} disabled={loading} className="shrink-0">
            {loading ? '…' : 'Simüle et'}
          </Button>
        </div>

        {/* Body */}
        <div className="min-h-0 flex-1 overflow-y-auto p-5">
          {err && <p className="text-sm text-[var(--color-danger)]">{err}</p>}
          {!err && !data && <p className="text-sm text-[var(--color-text-dim)]">Yükleniyor…</p>}
          {data && (
            <>
              {/* Dynamic suffix (simulated) — moved to the top per user request,
                  ahead of the stable system prompt / skills / tools prefix. */}
              <CollapsibleSection
                title={<>Dinamik bağlam {message.trim() ? '(örnek mesaja göre)' : ''}</>}
                bulk={bulk}
              >
                {data.provider === 'claude-cli' && (
                  <div className="mb-2 rounded-md border border-[color-mix(in_srgb,var(--color-warning)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_8%,transparent)] px-2.5 py-1.5 text-[11px] leading-relaxed text-[var(--color-text-dim)]">
                    claude-cli: dinamik bağlam ayrı bir system bloğu olarak değil,{' '}
                    <strong>son kullanıcı mesajının içine dokunularak</strong> gönderilir
                    (sıcak cache prefix'ini bozmaz).
                  </div>
                )}
                {data.dynamic ? (
                  raw ? (
                    <pre className="whitespace-pre-wrap break-words rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-3 font-mono text-xs leading-relaxed text-[var(--color-text)]">
                      {data.dynamic}
                    </pre>
                  ) : (
                    <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-1">
                      <Markdown>{data.dynamic}</Markdown>
                    </div>
                  )
                ) : (
                  <p className="text-xs text-[var(--color-text-dim)]">
                    Bu mesaj için dinamik bağlam yok. Özet · oturum artifact'ları · todo listesi gerçek bir
                    oturumda, tur anında eklenir (burada simüle edilmez).
                  </p>
                )}
              </CollapsibleSection>

              <CollapsibleSection
                title="Sistem promptu"
                bulk={bulk}
                right={
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
                }
              >
                {raw ? (
                  <pre className="whitespace-pre-wrap break-words rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-3 font-mono text-xs leading-relaxed text-[var(--color-text)]">
                    {data.system || '(boş)'}
                  </pre>
                ) : (
                  <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-1">
                    <Markdown>{data.system || '(boş)'}</Markdown>
                  </div>
                )}
              </CollapsibleSection>

              {/* Skills catalog block, split out of the system prompt. */}
              {data.skills && (
                <CollapsibleSection title="Skills" bulk={bulk}>
                  {raw ? (
                    <pre className="whitespace-pre-wrap break-words rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-3 font-mono text-xs leading-relaxed text-[var(--color-text)]">
                      {data.skills}
                    </pre>
                  ) : (
                    <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-1">
                      <Markdown>{data.skills}</Markdown>
                    </div>
                  )}
                </CollapsibleSection>
              )}

              {/* The "schema sent every turn" tools are already part of the
                  per-turn Context (shipped via the API tools field — that's what
                  "her tur şema gönderilen" means), and their count + token cost is
                  shown in the summary chips above ("Şema araçlar (N)"). So they are
                  intentionally NOT re-listed here: the separate enumeration only
                  duplicated what the Context already carries. */}

              {shownLazyTools.length > 0 && (
                <CollapsibleSection
                  title={<>Araçlar — talep üzerine (lazy) · {shownLazyTools.length}</>}
                  bulk={bulk}
                >
                  <p className="mb-1.5 text-[11px] text-[var(--color-text-dim)]">
                    Şema tura girmez — yalnızca ad+özet sistem promptundaki{' '}
                    <code className="rounded bg-[var(--color-surface-2)] px-1">Available Tools (load on demand)</code>{' '}
                    bölümünde durur. Ajan{' '}
                    <code className="rounded bg-[var(--color-surface-2)] px-1">activate_tools</code>{' '}
                    ile istediğini bir sonraki adımda etkinleştirir.
                  </p>
                  <ul className="space-y-1">
                    {shownLazyTools.map((t) => (
                      <li
                        key={t.name}
                        className="rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2.5 py-1.5 opacity-75"
                      >
                        <div className="flex items-center gap-1.5">
                          <code className="text-xs font-medium text-[var(--color-text-dim)]">{t.name}</code>
                          {t.visibility && LAZY_VIS_CHIP[t.visibility] && (
                            <span className="rounded bg-[var(--color-surface-3)] px-1 py-0.5 text-[9px] font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
                              {LAZY_VIS_CHIP[t.visibility]}
                            </span>
                          )}
                        </div>
                        {t.description && (
                          <p className="mt-0.5 text-[11px] text-[var(--color-text-dim)]">{t.description}</p>
                        )}
                      </li>
                    ))}
                  </ul>
                </CollapsibleSection>
              )}
            </>
          )}
        </div>
      </div>
    </ModalOverlay>
  )
}

// BulkButtons is the header pair that broadcasts expand-all / collapse-all to
// every CollapsibleSection in the modal.
function BulkButtons({ onExpand, onCollapse }: { onExpand: () => void; onCollapse: () => void }) {
  const cls =
    'flex h-[30px] w-[30px] shrink-0 items-center justify-center rounded-md border border-[var(--color-border)] text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'
  return (
    <div className="flex shrink-0 items-center gap-1">
      <button onClick={onExpand} title="Tümünü aç" aria-label="Tümünü aç" className={cls}>
        <ChevronsUpDown size={14} />
      </button>
      <button onClick={onCollapse} title="Tümünü kapat" aria-label="Tümünü kapat" className={cls}>
        <ChevronsDownUp size={14} />
      </button>
    </div>
  )
}

function Stat({
  label,
  value,
  accent,
  dim,
}: {
  label: string
  value: number
  accent?: boolean
  dim?: boolean
}) {
  const cls = accent
    ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
    : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
  return (
    <span className={`rounded-md px-2 py-1 ${cls} ${dim ? 'opacity-60' : ''}`}>
      {dim ? label : <>{label}: <strong>{value.toLocaleString()}</strong></>}
    </span>
  )
}
