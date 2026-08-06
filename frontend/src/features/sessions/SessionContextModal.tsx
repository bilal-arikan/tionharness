import { useCallback, useEffect, useState } from 'react'
import { ChevronRight, ChevronsDownUp, ChevronsUpDown, Copy, FoldVertical, X } from 'lucide-react'
import type { SessionContextPreview } from '@/types'
import { api } from '@/api'
import { copyToClipboard } from '@/shared/lib/clipboard'
import { Markdown } from '@/shared/components/markdown/Markdown'
import {
  Button,
  CollapsibleSection,
  InfoPopover,
  ModalOverlay,
  toast,
  useBulkToggle,
  type BulkToggle,
} from '@/shared/components'
import { CacheWarmthBadge } from './CacheWarmthBadge'
import { cacheRemaining } from './sessionDetailFormat'
import { serverNow } from '@/shared/lib/serverClock'

// FLOOR_NOTE clarifies that the predicted CLI overhead is a per-turn FLOOR (base
// system + built-ins + eager tools only), so a measured turn can exceed it: the
// gap is accumulated warm context + runtime-activated (deferred) tools. Appended
// to the CLI-overhead info popover.
const FLOOR_NOTE =
  '"Beklenen taban" = bir sonraki minimal tur için ALT SINIR (yalnız CLI tabanı + eager araç şemaları). ' +
  'Ölçülen "Gerçek" bunu aşabilir: fark, oturum boyunca biriken sıcak bağlam (--resume ile server-side tutulan geçmiş, her iç çağrıda cacheRead) + çalışma-anında aktive edilen deferred araçlardır.'

// LAZY_VIS_CHIP labels a lazy tool's visibility tier next to its name so the
// load-on-demand list reflects the same Tam/Özet/İsim/Gizli chips set in the tools
// screen: "summary" keeps its description, "name-only"/"hidden" show the name alone.
// ("full" tools are eager, never in this list.)
const LAZY_VIS_CHIP: Record<string, string> = {
  summary: 'Özet',
  'name-only': 'İsim',
  hidden: 'Gizli',
}

interface Props {
  sessionId: string
  title?: string
  // Session last-activity timestamp (unix seconds) — drives the prompt-cache
  // warmth countdown in the cache legend. Optional: omit to hide the badge.
  updatedAt?: number
  onClose: () => void
}

// SessionContextModal previews the EXACT next-turn context a session's agent would
// be sent — the composed system prompt + dynamic suffix, the full message
// transcript (with author labels + tool recap folded in) and the tool catalog,
// each with a token estimate. A debug view: an optional sample message shows what
// the agent would receive if that were sent next. Read-only — no turn is run.
export function SessionContextModal({ sessionId, title, updatedAt, onClose }: Props) {
  const [data, setData] = useState<SessionContextPreview | null>(null)
  // Live 1s tick for the prompt-cache warmth countdown; self-stops once cold.
  const [nowSec, setNowSec] = useState(() => serverNow())
  const [err, setErr] = useState<string | null>(null)
  const [message, setMessage] = useState('')
  const [loading, setLoading] = useState(false)
  // When on, the preview simulates this turn's budgeted compaction (fewer
  // messages) so the array matches what the model actually receives.
  const [simulate, setSimulate] = useState(false)
  // When on, the provider's REAL tokenizer counts the composed request
  // server-side (?accurate=1 — anthropic only, one free API call); the result
  // renders next to the heuristic total so drift is visible.
  const [accurate, setAccurate] = useState(false)
  // Token summary + CLI-overhead strip: collapsible, default collapsed.
  const [statsOpen, setStatsOpen] = useState(false)
  const { bulk, expandAll, collapseAll } = useBulkToggle(true)

  const load = useCallback(
    (msg: string, compact: boolean, exact = false) => {
      setLoading(true)
      api
        .sessionContextPreview(sessionId, msg.trim() || undefined, compact, exact)
        .then(setData)
        .catch((e) => setErr((e as Error).message))
        .finally(() => setLoading(false))
    },
    [sessionId],
  )

  // Reload whenever the sample message is (re)submitted or a toggle flips.
  // simulate/accurate are dependencies so toggling them refetches immediately.
  useEffect(() => load(message, simulate, accurate), [load, simulate, accurate]) // eslint-disable-line react-hooks/exhaustive-deps

  // Tick every second while the prompt cache is still warm, then self-stop.
  useEffect(() => {
    if (!updatedAt) return
    const warm = () => cacheRemaining(updatedAt, serverNow()) > 0
    if (!warm()) return
    const t = setInterval(() => {
      setNowSec(serverNow())
      if (!warm()) clearInterval(t)
    }, 1000)
    return () => clearInterval(t)
  }, [updatedAt])

  const copy = () => {
    if (!data) return
    const transcript = data.messages
      .map((m) => {
        const who = m.author
          ? ` (${m.role === 'user' ? '→ ' : ''}${m.author}${m.self && m.role !== 'user' ? ', siz' : ''})`
          : ''
        return `### ${m.role}${who}\n${m.text}`
      })
      .join('\n\n')
    const skills = data.skills ? `\n\n# Skills\n${data.skills}` : ''
    const summary = data.summary ? `\n\n# Summary (folded)\n${data.summary}` : ''
    const full = `# System\n${data.system}${skills}${summary}\n\n# Dynamic\n${data.dynamic}\n\n# Messages\n${transcript}`
    copyToClipboard(full).then((ok) => {
      if (ok) toast.info('Panoya kopyalandı')
    })
  }

  return (
    <ModalOverlay onClose={onClose} padding="p-6">
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Oturum bağlamı"
        data-testid="session-context-modal"
        className="flex max-h-[85vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-[var(--shadow-lg)]"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header — title with the copy + close actions inline beside it. */}
        <div className="flex items-center gap-2 border-b border-[var(--color-border)] px-5 py-3">
          <div className="min-w-0 flex-1">
            <h2 className="truncate text-sm font-semibold">
              Sıradaki tur bağlam önizleme{title ? ` — ${title}` : ''}
            </h2>
            <p className="truncate text-xs text-[var(--color-text-dim)]">
              {data
                ? `${data.agentName} bu oturumda bir sonraki turda alacağı tam istek`
                : 'Yükleniyor…'}
              {' · '}salt-okunur (tur çalıştırılmaz)
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            {data && (
              <button
                onClick={copy}
                title="Bağlamı kopyala"
                aria-label="Bağlamı kopyala"
                className="flex shrink-0 items-center rounded-md border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
              >
                <Copy size={13} />
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
                · Toplam {data.totalTokens.toLocaleString()}{' '}
                <span className="opacity-70">(~tahmini)</span>
                {!!data.accurateTokens && (
                  <span className="ml-1 font-medium text-[var(--color-accent)]">
                    · Gerçek {data.accurateTokens.toLocaleString()}
                    <span className="opacity-70">
                      {' '}
                      (
                      {data.totalTokens > 0
                        ? `${data.accurateTokens >= data.totalTokens ? '+' : ''}${Math.round(((data.accurateTokens - data.totalTokens) / data.totalTokens) * 100)}% sapma`
                        : ''}
                      )
                    </span>
                  </span>
                )}
              </span>
              {data.cliOverhead && (
                <span className="rounded bg-[color-mix(in_srgb,var(--color-warning)_18%,transparent)] px-1.5 py-0.5 font-medium text-[var(--color-warning)]">
                  CLI ek yükü
                </span>
              )}
            </button>

            {statsOpen && (
              <>
                <div className="flex flex-wrap items-center gap-2 px-5 pb-2">
                  <Stat label="Toplam" value={data.totalTokens} accent />
                  <Stat label="Sistem" value={data.systemTokens} />
                  {data.skills && <Stat label="Skills" value={data.skillsTokens} />}
                  {data.summary && <Stat label="Özet" value={data.summaryTokens} />}
                  <Stat label="Dinamik" value={data.dynamicTokens} />
                  <Stat label={`Mesajlar (${data.messages.length})`} value={data.messageTokens} />
                  {data.droppedMessages.length > 0 && (
                    <Stat
                      label={`Katlanmış (${data.droppedMessages.length})`}
                      value={data.droppedTokens}
                      dropped
                    />
                  )}
                  <Stat label={`Araçlar (${data.tools.length})`} value={data.toolTokens} />
                  {data.lazyTools.length > 0 && (
                    <Stat label={`Talep-üzerine (${data.lazyTools.length})`} value={0} dim />
                  )}
                  {data.cliOverhead && data.cliOverhead.measuredTokens > 0 && (
                    <Stat
                      label="Gerçek (CLI, ölçülen)"
                      value={data.cliOverhead.measuredTokens}
                      accent
                    />
                  )}
                  {/* Predicted CLI projection — shown alongside the measured figure (dim)
                      and as the primary accent chip before the first turn is measured. */}
                  {data.cliOverhead && data.cliOverhead.predictedOverhead > 0 && (
                    <Stat
                      label="Beklenen taban (CLI)"
                      value={data.cliOverhead.estimatedTokens + data.cliOverhead.predictedOverhead}
                      accent={data.cliOverhead.measuredTokens === 0}
                    />
                  )}
                  {data.multiAgent && (
                    <span className="rounded-md bg-[var(--color-accent-soft)] px-2 py-1 text-[var(--color-accent)]">
                      çok-ajanlı
                    </span>
                  )}
                  <span className="text-[var(--color-text-dim)]">~token tahmini</span>
                </div>

                {/* CLI-wrapper overhead warning: TotalTokens under-reports for claude-cli */}
                {data.cliOverhead && (
                  <div className="bg-[color-mix(in_srgb,var(--color-warning)_8%,transparent)] px-5 py-2 text-[11px]">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="inline-flex items-center gap-1 rounded bg-[color-mix(in_srgb,var(--color-warning)_18%,transparent)] px-1.5 py-0.5 font-medium text-[var(--color-warning)]">
                        CLI ek yükü
                        <InfoPopover
                          text={`${data.cliOverhead.note}\n\n${FLOOR_NOTE}`}
                          label="CLI ek yükü nasıl hesaplanır?"
                        />
                      </span>
                      {data.cliOverhead.measuredTokens > 0 ? (
                        <span className="text-[var(--color-text-dim)]">
                          Tahmin{' '}
                          <strong>{data.cliOverhead.estimatedTokens.toLocaleString()}</strong> →
                          gerçek <strong>{data.cliOverhead.measuredTokens.toLocaleString()}</strong>{' '}
                          (+<strong>{data.cliOverhead.overheadTokens.toLocaleString()}</strong> ek
                          yük
                          {data.cliOverhead.estimatedTokens > 0 &&
                            `, ~${(data.cliOverhead.measuredTokens / data.cliOverhead.estimatedTokens).toFixed(1)}×`}
                          {`, ${data.cliOverhead.calls} çağrı ort.`})
                          {/* Also surface the reference-based FLOOR next to the measured value;
                      the gap (measured − floor) is accumulated warm context. */}
                          {data.cliOverhead.predictedOverhead > 0 && (
                            <>
                              {' · '}beklenen taban ~
                              <strong>
                                {(
                                  data.cliOverhead.estimatedTokens +
                                  data.cliOverhead.predictedOverhead
                                ).toLocaleString()}
                              </strong>{' '}
                              (fark = birikmiş sıcak bağlam)
                            </>
                          )}
                        </span>
                      ) : data.cliOverhead.predictedOverhead > 0 ? (
                        <span className="text-[var(--color-text-dim)]">
                          Tahmin{' '}
                          <strong>{data.cliOverhead.estimatedTokens.toLocaleString()}</strong> →
                          beklenen taban ~
                          <strong>
                            {(
                              data.cliOverhead.estimatedTokens + data.cliOverhead.predictedOverhead
                            ).toLocaleString()}
                          </strong>{' '}
                          (+<strong>{data.cliOverhead.predictedOverhead.toLocaleString()}</strong>{' '}
                          taban ek yük, henüz ölçülmedi)
                        </span>
                      ) : (
                        <span className="text-[var(--color-text-dim)]">henüz ölçülmedi</span>
                      )}
                    </div>
                  </div>
                )}
              </>
            )}
          </div>
        )}

        {/* Cache legend */}
        {data && (
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1 border-b border-[var(--color-border)] px-5 py-1.5 text-[11px]">
            <span className="inline-flex items-center gap-1 rounded bg-[color-mix(in_srgb,var(--color-success)_15%,transparent)] px-1.5 py-0.5 font-medium text-[var(--color-success)]">
              <span className="h-2 w-2 rounded-sm bg-[color-mix(in_srgb,var(--color-success)_70%,transparent)]" />
              cache'li (sıcak, yeniden kullanılır)
              {data.cache.note && (
                <InfoPopover text={data.cache.note} label="Cache nasıl çalışır?" />
              )}
            </span>
            {/* Prompt-cache TTL countdown: how long this warm prefix survives
                before the 1h ephemeral cache goes cold (time axis, distinct from
                the per-segment cached/uncached flags above). */}
            {updatedAt ? (
              <span className="ml-auto inline-flex items-center gap-1">
                <span className="text-[var(--color-text-dim)]">TTL:</span>
                <CacheWarmthBadge updatedAt={updatedAt} nowSec={nowSec} />
              </span>
            ) : null}
          </div>
        )}

        {/* Sample "next" message + compaction-simulation toggle */}
        <div className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] px-5 py-2">
          <input
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && load(message, simulate)}
            placeholder="Örnek 'sıradaki' kullanıcı mesajı → bu mesaj gönderilseydi bağlam nasıl olurdu"
            className="min-w-0 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-xs outline-none focus:border-[var(--color-accent)]"
          />
          {/* Expand/collapse-all (icon-only), sitting next to the Compaction toggle. */}
          {data && <BulkButtons onExpand={expandAll} onCollapse={collapseAll} />}
          <button
            onClick={() => setSimulate((s) => !s)}
            disabled={loading}
            title="Bu turun sıkıştırmasını (compaction) simüle et — mesaj dizisini modele gerçekte gidecek hale indir (salt-okunur, özet üretmez/kaydetmez)"
            className={`flex shrink-0 items-center gap-1 rounded-md border px-2.5 py-1.5 text-xs ${
              simulate
                ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'
            }`}
          >
            <FoldVertical size={13} /> Compaction {simulate ? 'açık' : 'simüle'}
          </button>
          <button
            onClick={() => setAccurate((a) => !a)}
            disabled={loading}
            title="Gerçek sayım: birleştirilmiş istek, sağlayıcının GERÇEK tokenizer'ıyla sunucuda sayılır (count_tokens; yalnız anthropic, üretim yok — ücretsiz bir API çağrısı). Sezgisel tahminle sapma yüzdesi Token özetinde görünür."
            className={`flex shrink-0 items-center gap-1 rounded-md border px-2.5 py-1.5 text-xs ${
              accurate
                ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'
            }`}
          >
            Σ Gerçek sayım
          </button>
          <Button
            onClick={() => load(message, simulate, accurate)}
            disabled={loading}
            className="shrink-0"
          >
            {loading ? '…' : 'Önizle'}
          </Button>
        </div>

        {/* Body */}
        <div className="min-h-0 flex-1 overflow-y-auto p-5">
          {err && <p className="text-sm text-[var(--color-danger)]">{err}</p>}
          {!err && !data && <p className="text-sm text-[var(--color-text-dim)]">Yükleniyor…</p>}
          {data && (
            <>
              {/* Section order (top→bottom), per user request: the fresh
                  model-bound message array first, then the dynamic suffix, then
                  the stable cached prefix (system / skills / warm message prefix).
                  The IIFE splits the message array so the fresh half can lead and
                  the cached half sits down with the other cached segments. */}
              {(() => {
                const cachedCount = data.cache.cachedMsgCount
                const cachedMsgs = data.messages.slice(0, cachedCount)
                const freshMsgs = data.messages.slice(cachedCount)
                return (
                  <>
                    <CollapsibleSection
                      title={<>Mesaj dizisi (modele gidecek) · {freshMsgs.length}</>}
                      bulk={bulk}
                    >
                      {data.compactionSimulated ? (
                        <HintNote>
                          <strong>Compaction simülasyonu açık.</strong> Bu dizi bu turun bütçeli
                          katlamasını da yansıtır (salt-okunur — yeni özet üretilmedi/kaydedilmedi).{' '}
                          {data.foldedCount > 0
                            ? `${data.foldedCount} bekleyen mesaj daha özete katlanırdı; aşağıdaki "Artık gönderilmeyen" grubunda turuncu olarak görünür.`
                            : 'Bu turda ek katlanacak mesaj yok — dizi zaten bütçeye sığıyor.'}
                        </HintNote>
                      ) : (
                        data.droppedMessages.length === 0 && (
                          <HintNote>
                            Bu önizleme kalıcı özet sınırını uygular ama{' '}
                            <strong>bu turun ek bütçe katlamasını uygulamaz</strong>. Bütçeye yakın
                            oturumda gerçek tur daha fazla mesaj katlayabilir — üstteki{' '}
                            <strong>“Compaction simüle”</strong> düğmesiyle onu da görebilirsin.
                          </HintNote>
                        )
                      )}
                      {freshMsgs.length === 0 ? (
                        <Dim>
                          {cachedCount > 0
                            ? 'Taze mesaj yok (hepsi cache önekinde).'
                            : 'Henüz mesaj yok.'}
                        </Dim>
                      ) : (
                        <div className="space-y-2">
                          {freshMsgs.map((m, i) => (
                            <MessageCard key={i} m={m} cached={false} />
                          ))}
                        </div>
                      )}
                    </CollapsibleSection>

                    {/* Rolling summary — the compacted stand-in for the dropped
                        turns below. Shown as its own category (was buried in Dynamic). */}
                    {data.summary && (
                      <Section
                        title="Özet (katlanmış mesajların yerine geçer)"
                        cached={data.cache.summaryCached}
                        bulk={bulk}
                      >
                        {data.cache.summaryCached && (
                          <HintNote>
                            Özet artık volatile Dinamik'te değil;{' '}
                            <strong>cache'li önekin başında bir mesaj</strong> olarak gönderiliyor
                            (P2) → iki katlama arasında <strong>cache-read</strong> (her tur taze
                            değil).
                          </HintNote>
                        )}
                        <Markdown>{data.summary}</Markdown>
                      </Section>
                    )}

                    {/* Dropped messages — folded into the summary, no longer sent.
                        Light-orange group so it is clearly "off the wire". */}
                    {data.droppedMessages.length > 0 && (
                      <CollapsibleSection
                        title={
                          <span className="text-[var(--color-warning)]">
                            Artık gönderilmeyen (özete katlanmış) · {data.droppedMessages.length}
                          </span>
                        }
                        right={<DroppedTag />}
                        defaultOpen={false}
                        bulk={bulk}
                      >
                        <HintNote>
                          Bu mesajlar <strong>özete katlandı</strong> ve modele artık ham olarak
                          gönderilmiyor — içerikleri yukarıdaki <strong>Özet</strong> bölümünde
                          temsil ediliyor. Token'ları toplamda sayılmaz. Tam metni gerekirse{' '}
                          <code className="rounded bg-[var(--color-surface-2)] px-1">
                            conversation_search
                          </code>{' '}
                          ile geri alınır.
                        </HintNote>
                        <div className="space-y-2">
                          {data.droppedMessages.map((m, i) => (
                            <MessageCard key={i} m={m} cached={false} dropped />
                          ))}
                        </div>
                      </CollapsibleSection>
                    )}

                    <Section title="Dinamik bağlam" cached={data.cache.dynamicCached} bulk={bulk}>
                      {data.cliOverhead && (
                        <HintNote>
                          claude-cli: dinamik bağlam ayrı bir system bloğu olarak değil,{' '}
                          <strong>son kullanıcı mesajının içine dokunularak</strong> gönderilir
                          (sıcak cache prefix'ini bozmaz).
                        </HintNote>
                      )}
                      {data.dynamic ? <Markdown>{data.dynamic}</Markdown> : <Dim>(boş)</Dim>}
                    </Section>

                    <Section title="Sistem promptu" cached={data.cache.systemCached} bulk={bulk}>
                      <Markdown>{data.system || '(boş)'}</Markdown>
                    </Section>
                    {data.skills && (
                      <Section title="Skills" cached={data.cache.systemCached} bulk={bulk}>
                        <Markdown>{data.skills}</Markdown>
                      </Section>
                    )}

                    {cachedCount > 0 && (
                      <CollapsibleSection
                        title={<>Cache'li mesaj dizisi (sıcak önek) · {cachedMsgs.length}</>}
                        right={<CacheTag cached />}
                        defaultOpen={false}
                        bulk={bulk}
                      >
                        <div className="space-y-2">
                          {cachedMsgs.map((m, i) => (
                            <MessageCard key={i} m={m} cached />
                          ))}
                        </div>
                      </CollapsibleSection>
                    )}
                  </>
                )
              })()}

              <CollapsibleSection
                title={<>Araçlar — her tur şema gönderilen · {data.tools.length}</>}
                right={<CacheTag cached={data.cache.toolsCached} />}
                bulk={bulk}
              >
                {data.cliOverhead ? (
                  <HintNote>
                    claude-cli: bu araçlar TionSwarm'nun kendi isteğinde şema olarak DEĞİL,{' '}
                    <strong>CLI'nin built-in araçları + MCP köprüsüyle</strong> iletilir; aşağıdaki
                    token sayısı yaklaşıktır (gerçek yük CLI'nin kendi temsiline göre değişir — bkz.
                    yukarıdaki “CLI ek yükü”).
                  </HintNote>
                ) : (
                  <p className="mb-1.5 text-[11px] text-[var(--color-text-dim)]">
                    Bu araçların TAM şeması (açıklama + JSON girdi şeması + örnekler) her tur
                    gönderilir. İçeriğini görmek için bir aracı genişlet.
                  </p>
                )}
                {data.tools.length === 0 ? (
                  <Dim>Bu ajana şema gönderilen araç yok.</Dim>
                ) : (
                  <ul className="space-y-1">
                    {data.tools.map((t) => (
                      <li
                        key={t.name}
                        className="overflow-hidden rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)]"
                      >
                        <details>
                          <summary className="cursor-pointer select-none px-2.5 py-1.5 text-xs">
                            <code
                              className={`font-medium ${
                                data.cache.toolsCached ? CACHED : 'text-[var(--color-text)]'
                              }`}
                            >
                              {t.name}
                            </code>
                          </summary>
                          <div className="border-t border-[var(--color-border)] px-2.5 py-2">
                            {t.description && (
                              <p className="mb-2 whitespace-pre-wrap text-[11px] leading-relaxed text-[var(--color-text-dim)]">
                                {t.description}
                              </p>
                            )}
                            {t.inputSchema != null && (
                              <pre className="overflow-x-auto rounded bg-[var(--color-surface)] p-2 text-[10px] leading-relaxed text-[var(--color-text-dim)]">
                                {JSON.stringify(t.inputSchema, null, 2)}
                              </pre>
                            )}
                            {!t.description && t.inputSchema == null && (
                              <p className="text-[11px] text-[var(--color-text-dim)]">(şema yok)</p>
                            )}
                          </div>
                        </details>
                      </li>
                    ))}
                  </ul>
                )}
              </CollapsibleSection>

              {/* Lazy ("load on demand") tools: schemas are NOT shipped each turn —
                  only name+summary live in the system prompt's catalog block (already
                  counted under Sistem). Listing them here explains why the info screen
                  counts a much larger effective catalog (eager + these) than the small
                  per-turn "Araçlar" schema set — the gap the user sees at 129 vs few.
                  For a claude-cli agent these are the deferred MCP + self-management
                  tools it activates on demand via ToolSearch across the session. */}
              {data.lazyTools.length > 0 && (
                <CollapsibleSection
                  title={<>Araçlar — talep üzerine (lazy) · {data.lazyTools.length}</>}
                  bulk={bulk}
                >
                  <p className="mb-1.5 text-[11px] text-[var(--color-text-dim)]">
                    Şema tura girmez — yalnızca ad+özet sistem promptundaki{' '}
                    <code className="rounded bg-[var(--color-surface-2)] px-1">
                      Available Tools (load on demand)
                    </code>{' '}
                    bölümünde durur (token maliyeti “Sistem”de sayılır). Ajan{' '}
                    {data.cliOverhead ? (
                      <>
                        bunlara CLI'nin kendi{' '}
                        <code className="rounded bg-[var(--color-surface-2)] px-1">ToolSearch</code>
                        'üyle ulaşır; oturum boyunca aktive edilenler sıcak kalır ama bu popup'ın
                        “Araçlar” (her tur şema) sayısına girmez.
                      </>
                    ) : (
                      <>
                        <code className="rounded bg-[var(--color-surface-2)] px-1">
                          activate_tools
                        </code>{' '}
                        ile istediğini bir sonraki adımda etkinleştirir.
                      </>
                    )}
                  </p>
                  <ul className="space-y-1">
                    {data.lazyTools.map((t) => (
                      <li
                        key={t.name}
                        className="rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2.5 py-1.5 opacity-75"
                      >
                        <div className="flex items-center gap-1.5">
                          <code className="text-xs font-medium text-[var(--color-text-dim)]">
                            {t.name}
                          </code>
                          {t.visibility && LAZY_VIS_CHIP[t.visibility] && (
                            <span className="rounded bg-[var(--color-surface-3)] px-1 py-0.5 text-[9px] font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
                              {LAZY_VIS_CHIP[t.visibility]}
                            </span>
                          )}
                        </div>
                        {t.description && (
                          <p className="mt-0.5 text-[11px] text-[var(--color-text-dim)]">
                            {t.description}
                          </p>
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

// CACHED is the soft-green tint applied to request segments served from the warm
// prompt cache (reused across turns). Markdown plain text inherits this via
// currentColor; code/links keep their own colour. Uncached segments stay the
// default text colour.
const CACHED = 'text-[color-mix(in_srgb,var(--color-success)_60%,var(--color-text))]'

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

function Section({
  title,
  cached,
  bulk,
  children,
}: {
  title: string
  cached?: boolean
  bulk?: BulkToggle
  children: React.ReactNode
}) {
  return (
    <CollapsibleSection title={title} right={<CacheTag cached={!!cached} />} bulk={bulk}>
      <div
        className={`rounded-lg border bg-[var(--color-bg)] px-3 py-1 ${
          cached
            ? `border-[color-mix(in_srgb,var(--color-success)_30%,transparent)] ${CACHED}`
            : 'border-[var(--color-border)]'
        }`}
      >
        {children}
      </div>
    </CollapsibleSection>
  )
}

// MessageCard renders one transcript message with its role/author labels. `cached`
// tints it green (served from the warm prefix); `dropped` tints it light orange
// (folded into the summary and no longer sent). At most one should be set.
function MessageCard({
  m,
  cached,
  dropped,
}: {
  m: { role: string; text: string; author?: string; self?: boolean }
  cached: boolean
  dropped?: boolean
}) {
  const border = dropped
    ? 'border-[color-mix(in_srgb,var(--color-warning)_35%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_6%,transparent)]'
    : cached
      ? 'border-[color-mix(in_srgb,var(--color-success)_30%,transparent)] bg-[var(--color-bg)]'
      : 'border-[var(--color-border)] bg-[var(--color-bg)]'
  return (
    <div className={`rounded-lg border px-3 py-2 ${border}`}>
      <div className="mb-1 flex items-center gap-1.5">
        <span className="text-[10px] font-semibold uppercase tracking-wide text-[var(--color-accent)]">
          {m.role}
        </span>
        {m.author && (
          <span
            title={m.role === 'user' ? `Hedef ajan: ${m.author}` : `Yazan ajan: ${m.author}`}
            className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] font-medium text-[var(--color-text-dim)]"
          >
            {m.role === 'user' ? `→ ${m.author}` : m.author}
            {m.self && m.role !== 'user' && ' (siz)'}
          </span>
        )}
        {dropped ? <DroppedTag /> : <CacheTag cached={cached} />}
      </div>
      <pre
        className={`overflow-x-auto whitespace-pre-wrap break-words text-xs ${
          dropped ? 'text-[var(--color-text-dim)]' : cached ? CACHED : 'text-[var(--color-text)]'
        }`}
      >
        {m.text || '(boş)'}
      </pre>
    </div>
  )
}

// DroppedTag marks a message folded into the rolling summary (no longer sent).
function DroppedTag() {
  return (
    <span className="rounded bg-[color-mix(in_srgb,var(--color-warning)_18%,transparent)] px-1.5 py-0.5 text-[9px] font-medium normal-case text-[var(--color-warning)]">
      katlandı · gönderilmiyor
    </span>
  )
}

// CacheTag is the per-segment pill: green "cache'li" (served warm) or a neutral
// "cache dışı" (sent fresh).
function CacheTag({ cached }: { cached: boolean }) {
  return cached ? (
    <span className="rounded bg-[color-mix(in_srgb,var(--color-success)_15%,transparent)] px-1.5 py-0.5 text-[9px] font-medium normal-case text-[var(--color-success)]">
      cache'li
    </span>
  ) : (
    <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[9px] font-medium normal-case text-[var(--color-text-dim)]">
      cache dışı
    </span>
  )
}

function Dim({ children }: { children: React.ReactNode }) {
  return <p className="text-xs text-[var(--color-text-dim)]">{children}</p>
}

// HintNote is the soft amber "this preview is an approximation" callout used to
// flag where the debug view intentionally diverges from the exact wire payload
// (no this-turn compaction; provider-specific tool/dynamic delivery).
function HintNote({ children }: { children: React.ReactNode }) {
  return (
    <div className="mb-2 rounded-md border border-[color-mix(in_srgb,var(--color-warning)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_8%,transparent)] px-2.5 py-1.5 text-[11px] leading-relaxed text-[var(--color-text-dim)]">
      {children}
    </div>
  )
}

function Stat({
  label,
  value,
  accent,
  dropped,
  dim,
}: {
  label: string
  value: number
  accent?: boolean
  dropped?: boolean
  // dim marks a zero-cost chip (e.g. lazy tools): the label is shown alone, muted,
  // with no ": token" tail so it doesn't read as "0 tokens" when the point is that
  // the schemas never ship (their cost already lives in the Sistem segment).
  dim?: boolean
}) {
  const cls = dropped
    ? 'bg-[color-mix(in_srgb,var(--color-warning)_14%,transparent)] text-[var(--color-warning)]'
    : accent
      ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
      : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
  return (
    <span
      className={`rounded-md px-2 py-1 ${cls} ${dim ? 'opacity-60' : ''}`}
      title={
        dropped
          ? 'Özete katlandı — toplama dahil değil'
          : dim
            ? 'Şema tura girmez — maliyeti Sistem segmentinde'
            : undefined
      }
    >
      {dim ? (
        label
      ) : (
        <>
          {label}: <strong>{value.toLocaleString()}</strong>
        </>
      )}
    </span>
  )
}
