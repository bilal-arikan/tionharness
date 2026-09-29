import { useCallback, useEffect, useState } from 'react'
import { ChevronRight, ChevronsDownUp, ChevronsUpDown, Copy, FoldVertical, X } from 'lucide-react'
import type { SessionContextPreview } from '@/types'
import { api } from '@/api'
import { useKeyedReset } from '@/shared/lib/useKeyedReset'
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
import { count } from '@/shared/lib/format'
import { Trans, useTranslation } from 'react-i18next'
import { i18next } from '@/i18n'

// FLOOR_NOTE clarifies that the predicted CLI overhead is a per-turn FLOOR (base
// system + built-ins + eager tools only), so a measured turn can exceed it: the
// gap is accumulated warm context + runtime-activated (deferred) tools. Appended
// to the CLI-overhead info popover.
const floorNote = () => i18next.t('context.floorNote', { ns: 'sessions' })

// LAZY_VIS_CHIP labels a lazy tool's visibility tier next to its name so the
// load-on-demand list reflects the same Tam/Özet/İsim/Gizli chips set in the tools
// screen: "summary" keeps its description, "name-only"/"hidden" show the name alone.
// ("full" tools are eager, never in this list.)
const LAZY_VIS_KEY: Record<string, string> = {
  summary: 'summary',
  'name-only': 'nameOnly',
  hidden: 'hidden',
}

const MESSAGE_ROLE_KEYS: Record<string, string> = {
  user: 'context.messageRole.user',
  assistant: 'context.messageRole.assistant',
  tool: 'context.messageRole.tool',
  system: 'context.messageRole.system',
}

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
  const { t } = useTranslation('sessions')
  const [data, setData] = useState<SessionContextPreview | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [message, setMessage] = useState('')
  const [loading, setLoading] = useState(true)
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

  // run lands the preview through callbacks only (so the effect may call it);
  // load is the submit/button entry point that also re-arms the spinner.
  const run = useCallback(
    (msg: string, compact: boolean, exact = false) =>
      api
        .sessionContextPreview(sessionId, msg.trim() || undefined, compact, exact)
        .then(setData)
        .catch((e) => setErr((e as Error).message))
        .finally(() => setLoading(false)),
    [sessionId],
  )
  const load = useCallback(
    (msg: string, compact: boolean, exact = false) => {
      setLoading(true)
      void run(msg, compact, exact)
    },
    [run],
  )

  // Reload whenever the sample message is (re)submitted or a toggle flips.
  // simulate/accurate are dependencies so toggling them refetches immediately;
  // the message only refetches on submit.
  useKeyedReset(`${sessionId}|${simulate}|${accurate}`, () => setLoading(true))
  useEffect(() => {
    void run(message, simulate, accurate)
  }, [run, simulate, accurate]) // eslint-disable-line react-hooks/exhaustive-deps

  const copy = () => {
    if (!data) return
    const transcript = data.messages
      .map((m) => {
        const role = MESSAGE_ROLE_KEYS[m.role] ? t(MESSAGE_ROLE_KEYS[m.role]) : m.role
        const who = m.author
          ? ` (${m.role === 'user' ? '→ ' : ''}${m.author}${m.self && m.role !== 'user' ? `, ${t('context.you')}` : ''})`
          : ''
        return `### ${role}${who}\n${m.text}`
      })
      .join('\n\n')
    const skills = data.skills ? `\n\n# ${t('context.copy.skills')}\n${data.skills}` : ''
    const summary = data.summary ? `\n\n# ${t('context.copy.summary')}\n${data.summary}` : ''
    const full = `# ${t('context.copy.system')}\n${data.system}${skills}${summary}\n\n# ${t('context.copy.dynamic')}\n${data.dynamic}\n\n# ${t('context.copy.messages')}\n${transcript}`
    copyToClipboard(full).then((ok) => {
      if (ok) toast.info(t('context.copied'))
    })
  }

  return (
    <ModalOverlay onClose={onClose} padding="p-6">
      <div
        role="dialog"
        aria-modal="true"
        aria-label={t('context.dialogLabel')}
        data-testid="session-context-modal"
        className="flex max-h-[85vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-[var(--shadow-lg)]"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header — title with the copy + close actions inline beside it. */}
        <div className="flex items-center gap-2 border-b border-[var(--color-border)] px-5 py-3">
          <div className="min-w-0 flex-1">
            <h2 className="truncate text-sm font-semibold">
              {t('context.title')}
              {title ? ` — ${title}` : ''}
            </h2>
            <p className="truncate text-xs text-[var(--color-text-dim)]">
              {data ? t('context.subtitle', { agent: data.agentName }) : t('common.loading')}
              {' · '}
              {t('context.readOnly')}
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            {data && (
              <button
                onClick={copy}
                title={t('context.copyAction')}
                aria-label={t('context.copyAction')}
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
              <span className="font-medium">{t('context.tokenSummary')}</span>
              <span className="text-[var(--color-text-dim)]">
                · {t('context.totalTokens', { tokens: count(data.totalTokens) })}{' '}
                <span className="opacity-70">(~{t('common.estimated')})</span>
                {!!data.accurateTokens && (
                  <span className="ml-1 font-medium text-[var(--color-accent)]">
                    · {t('context.actualTokens', { tokens: count(data.accurateTokens) })}
                    <span className="opacity-70">
                      {' '}
                      (
                      {data.totalTokens > 0
                        ? t('context.deviation', {
                            value: `${data.accurateTokens >= data.totalTokens ? '+' : ''}${Math.round(((data.accurateTokens - data.totalTokens) / data.totalTokens) * 100)}`,
                          })
                        : ''}
                      )
                    </span>
                  </span>
                )}
              </span>
              {data.cliOverhead && (
                <span className="rounded bg-[color-mix(in_srgb,var(--color-warning)_18%,transparent)] px-1.5 py-0.5 font-medium text-[var(--color-warning)]">
                  {t('context.cliOverhead')}
                </span>
              )}
            </button>

            {statsOpen && (
              <>
                <div className="flex flex-wrap items-center gap-2 px-5 pb-2">
                  <Stat label={t('context.stat.total')} value={data.totalTokens} accent />
                  <Stat label={t('context.stat.system')} value={data.systemTokens} />
                  {data.skills && (
                    <Stat label={t('context.stat.skills')} value={data.skillsTokens} />
                  )}
                  {data.summary && (
                    <Stat label={t('context.stat.summary')} value={data.summaryTokens} />
                  )}
                  <Stat label={t('context.stat.dynamic')} value={data.dynamicTokens} />
                  <Stat
                    label={t('context.stat.messages', { count: data.messages.length })}
                    value={data.messageTokens}
                  />
                  {data.droppedMessages.length > 0 && (
                    <Stat
                      label={t('context.stat.folded', { count: data.droppedMessages.length })}
                      value={data.droppedTokens}
                      dropped
                    />
                  )}
                  <Stat
                    label={t('context.stat.tools', { count: data.tools.length })}
                    value={data.toolTokens}
                  />
                  {data.lazyTools.length > 0 && (
                    <Stat
                      label={t('context.stat.onDemand', { count: data.lazyTools.length })}
                      value={0}
                      dim
                    />
                  )}
                  {data.cliOverhead && data.cliOverhead.chatMeasuredTokens > 0 && (
                    <Stat
                      label={t('context.stat.actualChat')}
                      value={data.cliOverhead.chatMeasuredTokens}
                      accent
                    />
                  )}
                  {data.cliOverhead && data.cliOverhead.workerMeasuredTokens > 0 && (
                    <Stat
                      label={t('context.stat.actualWorker', { kind: data.cliOverhead.workerKind })}
                      value={data.cliOverhead.workerMeasuredTokens}
                    />
                  )}
                  {/* Predicted CLI projection — shown alongside the measured figure (dim)
                      and as the primary accent chip before the first turn is measured. */}
                  {data.cliOverhead && data.cliOverhead.predictedOverhead > 0 && (
                    <Stat
                      label={
                        data.cliOverhead.predictedSource === 'measured'
                          ? t('context.stat.measuredBaseline', {
                              count: data.cliOverhead.predictedSamples ?? 0,
                            })
                          : t('context.stat.expectedBaseline')
                      }
                      value={data.cliOverhead.estimatedTokens + data.cliOverhead.predictedOverhead}
                      accent={data.cliOverhead.chatMeasuredTokens === 0}
                    />
                  )}
                  {data.multiAgent && (
                    <span className="rounded-md bg-[var(--color-accent-soft)] px-2 py-1 text-[var(--color-accent)]">
                      {t('context.multiAgent')}
                    </span>
                  )}
                  <span className="text-[var(--color-text-dim)]">
                    ~{t('context.tokenEstimate')}
                  </span>
                </div>

                {/* CLI-wrapper overhead warning: TotalTokens under-reports for claude-cli */}
                {data.cliOverhead && (
                  <div className="bg-[color-mix(in_srgb,var(--color-warning)_8%,transparent)] px-5 py-2 text-[11px]">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="inline-flex items-center gap-1 rounded bg-[color-mix(in_srgb,var(--color-warning)_18%,transparent)] px-1.5 py-0.5 font-medium text-[var(--color-warning)]">
                        {t('context.cliOverhead')}
                        <InfoPopover
                          text={`${
                            data.cliOverhead.chatMeasuredTokens > 0
                              ? t('context.cliNoteMeasured', { provider: data.provider })
                              : t('context.cliNoteReference', { provider: data.provider })
                          }\n\n${floorNote()}`}
                          label={t('context.cliOverheadHelp')}
                        />
                      </span>
                      {data.cliOverhead.chatMeasuredTokens > 0 ? (
                        <span className="text-[var(--color-text-dim)]">
                          {t('context.cliMeasured', {
                            estimated: count(data.cliOverhead.estimatedTokens),
                            actual: count(data.cliOverhead.chatMeasuredTokens),
                            overhead: count(data.cliOverhead.overheadTokens),
                            ratio:
                              data.cliOverhead.estimatedTokens > 0
                                ? `, ~${(
                                    data.cliOverhead.chatMeasuredTokens /
                                    data.cliOverhead.estimatedTokens
                                  ).toFixed(1)}×`
                                : '',
                            calls: data.cliOverhead.chatCalls,
                          })}
                          {/* Also surface the reference-based FLOOR next to the measured value;
                      the gap (measured − floor) is accumulated warm context. */}
                          {data.cliOverhead.predictedOverhead > 0 && (
                            <>
                              {' · '}
                              {t('context.expectedFloor', {
                                tokens: count(
                                  data.cliOverhead.estimatedTokens +
                                    data.cliOverhead.predictedOverhead,
                                ),
                              })}
                            </>
                          )}
                        </span>
                      ) : data.cliOverhead.predictedOverhead > 0 ? (
                        <span className="text-[var(--color-text-dim)]">
                          {t('context.cliPredicted', {
                            estimated: count(data.cliOverhead.estimatedTokens),
                            baseline: count(
                              data.cliOverhead.estimatedTokens + data.cliOverhead.predictedOverhead,
                            ),
                            overhead: count(data.cliOverhead.predictedOverhead),
                          })}
                        </span>
                      ) : (
                        <span className="text-[var(--color-text-dim)]">
                          {t('context.notMeasured')}
                        </span>
                      )}
                      {data.cliOverhead.workerMeasuredTokens > 0 && (
                        <span className="text-[var(--color-text-dim)]">
                          {t('context.workerMeasured', {
                            kind: data.cliOverhead.workerKind,
                            tokens: count(data.cliOverhead.workerMeasuredTokens),
                            calls: data.cliOverhead.workerCalls,
                          })}
                        </span>
                      )}
                    </div>
                  </div>
                )}
              </>
            )}
          </div>
        )}

        {/* Sample "next" message + compaction-simulation toggle */}
        <div className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] px-5 py-2">
          <input
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && load(message, simulate)}
            placeholder={t('context.samplePlaceholder')}
            className="min-w-0 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-xs outline-none focus:border-[var(--color-accent)]"
          />
          {/* Expand/collapse-all (icon-only), sitting next to the Compaction toggle. */}
          {data && <BulkButtons onExpand={expandAll} onCollapse={collapseAll} />}
          <button
            onClick={() => setSimulate((s) => !s)}
            disabled={loading}
            title={t('context.compactionTitle')}
            className={`flex shrink-0 items-center gap-1 rounded-md border px-2.5 py-1.5 text-xs ${
              simulate
                ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'
            }`}
          >
            <FoldVertical size={13} />{' '}
            {simulate ? t('context.compactionOn') : t('context.compactionSimulate')}
          </button>
          <button
            onClick={() => setAccurate((a) => !a)}
            disabled={loading}
            title={t('context.accurateTitle')}
            className={`flex shrink-0 items-center gap-1 rounded-md border px-2.5 py-1.5 text-xs ${
              accurate
                ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'
            }`}
          >
            {t('context.accurateCount')}
          </button>
          <Button
            onClick={() => load(message, simulate, accurate)}
            disabled={loading}
            className="shrink-0"
          >
            {loading ? '…' : t('context.preview')}
          </Button>
        </div>

        {/* Body */}
        <div className="min-h-0 flex-1 overflow-y-auto p-5">
          {err && <p className="text-sm text-[var(--color-danger)]">{err}</p>}
          {!err && !data && (
            <p className="text-sm text-[var(--color-text-dim)]">{t('common.loading')}</p>
          )}
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
                      title={t('context.sections.messages', { count: freshMsgs.length })}
                      bulk={bulk}
                    >
                      {data.compactionSimulated ? (
                        <HintNote>
                          <Trans
                            ns="sessions"
                            i18nKey="context.compaction.simulated"
                            components={{ strong: <strong /> }}
                          />{' '}
                          {data.foldedCount > 0
                            ? t('context.compaction.wouldFold', { count: data.foldedCount })
                            : t('context.compaction.fits')}
                        </HintNote>
                      ) : (
                        data.droppedMessages.length === 0 && (
                          <HintNote>
                            <Trans
                              ns="sessions"
                              i18nKey="context.compaction.previewHint"
                              components={{ strong: <strong /> }}
                            />
                          </HintNote>
                        )
                      )}
                      {freshMsgs.length === 0 ? (
                        <Dim>
                          {cachedCount > 0 ? t('context.noFreshMessages') : t('context.noMessages')}
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
                        title={t('context.sections.summary')}
                        cached={data.cache.summaryCached}
                        bulk={bulk}
                      >
                        {data.cache.summaryCached && (
                          <HintNote>
                            <Trans
                              ns="sessions"
                              i18nKey="context.summaryCachedHint"
                              components={{ strong: <strong /> }}
                            />
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
                            {t('context.sections.dropped', {
                              count: data.droppedMessages.length,
                            })}
                          </span>
                        }
                        right={<DroppedTag />}
                        defaultOpen={false}
                        bulk={bulk}
                      >
                        <HintNote>
                          <Trans
                            ns="sessions"
                            i18nKey="context.droppedHint"
                            components={{
                              strong: <strong />,
                              code: <code className="rounded bg-[var(--color-surface-2)] px-1" />,
                            }}
                          />
                        </HintNote>
                        <div className="space-y-2">
                          {data.droppedMessages.map((m, i) => (
                            <MessageCard key={i} m={m} cached={false} dropped />
                          ))}
                        </div>
                      </CollapsibleSection>
                    )}

                    <Section
                      title={t('context.sections.dynamic')}
                      cached={data.cache.dynamicCached}
                      bulk={bulk}
                    >
                      {data.cliOverhead && (
                        <HintNote>
                          <Trans
                            ns="sessions"
                            i18nKey="context.dynamicCliHint"
                            components={{ strong: <strong /> }}
                          />
                        </HintNote>
                      )}
                      {data.dynamic ? (
                        <Markdown>{data.dynamic}</Markdown>
                      ) : (
                        <Dim>{t('common.empty')}</Dim>
                      )}
                    </Section>

                    <Section
                      title={t('context.sections.systemPrompt')}
                      cached={data.cache.systemCached}
                      bulk={bulk}
                    >
                      <Markdown>{data.system || t('common.empty')}</Markdown>
                    </Section>
                    {data.skills && (
                      <Section title="Skills" cached={data.cache.systemCached} bulk={bulk}>
                        <Markdown>{data.skills}</Markdown>
                      </Section>
                    )}

                    {cachedCount > 0 && (
                      <CollapsibleSection
                        title={t('context.sections.cachedMessages', { count: cachedMsgs.length })}
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
                title={t('context.sections.eagerTools', { count: data.tools.length })}
                right={<CacheTag cached={data.cache.toolsCached} />}
                bulk={bulk}
              >
                {data.cliOverhead ? (
                  <HintNote>
                    <Trans
                      ns="sessions"
                      i18nKey="context.tools.cliHint"
                      components={{ strong: <strong /> }}
                    />
                  </HintNote>
                ) : (
                  <p className="mb-1.5 text-[11px] text-[var(--color-text-dim)]">
                    {t('context.tools.eagerHelp')}
                  </p>
                )}
                {data.tools.length === 0 ? (
                  <Dim>{t('context.tools.noEager')}</Dim>
                ) : (
                  <ul className="space-y-1">
                    {data.tools.map((tool) => (
                      <li
                        key={tool.name}
                        className="overflow-hidden rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)]"
                      >
                        <details>
                          <summary className="cursor-pointer select-none px-2.5 py-1.5 text-xs">
                            <code
                              className={`font-medium ${
                                data.cache.toolsCached ? CACHED : 'text-[var(--color-text)]'
                              }`}
                            >
                              {tool.name}
                            </code>
                          </summary>
                          <div className="border-t border-[var(--color-border)] px-2.5 py-2">
                            {tool.description && (
                              <p className="mb-2 whitespace-pre-wrap text-[11px] leading-relaxed text-[var(--color-text-dim)]">
                                {tool.description}
                              </p>
                            )}
                            {tool.inputSchema != null && (
                              <pre className="overflow-x-auto rounded bg-[var(--color-surface)] p-2 text-[10px] leading-relaxed text-[var(--color-text-dim)]">
                                {JSON.stringify(tool.inputSchema, null, 2)}
                              </pre>
                            )}
                            {!tool.description && tool.inputSchema == null && (
                              <p className="text-[11px] text-[var(--color-text-dim)]">
                                {t('context.tools.noSchema')}
                              </p>
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
                  title={t('context.sections.lazyTools', { count: data.lazyTools.length })}
                  bulk={bulk}
                >
                  <p className="mb-1.5 text-[11px] text-[var(--color-text-dim)]">
                    <Trans
                      ns="sessions"
                      i18nKey="context.tools.lazyHelp"
                      components={{
                        code: <code className="rounded bg-[var(--color-surface-2)] px-1" />,
                      }}
                    />{' '}
                    {data.cliOverhead ? (
                      <Trans
                        ns="sessions"
                        i18nKey="context.tools.lazyCli"
                        components={{
                          code: <code className="rounded bg-[var(--color-surface-2)] px-1" />,
                        }}
                      />
                    ) : (
                      <Trans
                        ns="sessions"
                        i18nKey="context.tools.lazyNative"
                        components={{
                          code: <code className="rounded bg-[var(--color-surface-2)] px-1" />,
                        }}
                      />
                    )}
                  </p>
                  <ul className="space-y-1">
                    {data.lazyTools.map((tool) => (
                      <li
                        key={tool.name}
                        className="rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2.5 py-1.5 opacity-75"
                      >
                        <div className="flex items-center gap-1.5">
                          <code className="text-xs font-medium text-[var(--color-text-dim)]">
                            {tool.name}
                          </code>
                          {tool.visibility && LAZY_VIS_KEY[tool.visibility] && (
                            <span className="rounded bg-[var(--color-surface-2)] px-1 py-0.5 text-[9px] font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
                              {i18next.t(`context.visibility.${LAZY_VIS_KEY[tool.visibility]}`, {
                                ns: 'sessions',
                              })}
                            </span>
                          )}
                        </div>
                        {tool.description && (
                          <p className="mt-0.5 text-[11px] text-[var(--color-text-dim)]">
                            {tool.description}
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
  const { t } = useTranslation('sessions')
  const cls =
    'flex h-[30px] w-[30px] shrink-0 items-center justify-center rounded-md border border-[var(--color-border)] text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'
  return (
    <div className="flex shrink-0 items-center gap-1">
      <button
        onClick={onExpand}
        title={t('actions.expandAll')}
        aria-label={t('actions.expandAll')}
        className={cls}
      >
        <ChevronsUpDown size={14} />
      </button>
      <button
        onClick={onCollapse}
        title={t('actions.collapseAll')}
        aria-label={t('actions.collapseAll')}
        className={cls}
      >
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
  const { t } = useTranslation('sessions')
  const role = MESSAGE_ROLE_KEYS[m.role] ? t(MESSAGE_ROLE_KEYS[m.role]) : m.role
  const border = dropped
    ? 'border-[color-mix(in_srgb,var(--color-warning)_35%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_6%,transparent)]'
    : cached
      ? 'border-[color-mix(in_srgb,var(--color-success)_30%,transparent)] bg-[var(--color-bg)]'
      : 'border-[var(--color-border)] bg-[var(--color-bg)]'
  return (
    <div className={`rounded-lg border px-3 py-2 ${border}`}>
      <div className="mb-1 flex items-center gap-1.5">
        <span className="text-[10px] font-semibold uppercase tracking-wide text-[var(--color-accent)]">
          {role}
        </span>
        {m.author && (
          <span
            title={
              m.role === 'user'
                ? t('context.targetAgent', { agent: m.author })
                : t('context.authorAgent', { agent: m.author })
            }
            className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] font-medium text-[var(--color-text-dim)]"
          >
            {m.role === 'user' ? `→ ${m.author}` : m.author}
            {m.self && m.role !== 'user' && ` (${t('context.you')})`}
          </span>
        )}
        {dropped ? <DroppedTag /> : <CacheTag cached={cached} />}
      </div>
      <pre
        className={`overflow-x-auto whitespace-pre-wrap break-words text-xs ${
          dropped ? 'text-[var(--color-text-dim)]' : cached ? CACHED : 'text-[var(--color-text)]'
        }`}
      >
        {m.text || t('common.empty')}
      </pre>
    </div>
  )
}

// DroppedTag marks a message folded into the rolling summary (no longer sent).
function DroppedTag() {
  const { t } = useTranslation('sessions')
  return (
    <span className="rounded bg-[color-mix(in_srgb,var(--color-warning)_18%,transparent)] px-1.5 py-0.5 text-[9px] font-medium normal-case text-[var(--color-warning)]">
      {t('context.droppedTag')}
    </span>
  )
}

// CacheTag is the per-segment pill: green "cache'li" (served warm) or a neutral
// "cache dışı" (sent fresh).
function CacheTag({ cached }: { cached: boolean }) {
  const { t } = useTranslation('sessions')
  return cached ? (
    <span className="rounded bg-[color-mix(in_srgb,var(--color-success)_15%,transparent)] px-1.5 py-0.5 text-[9px] font-medium normal-case text-[var(--color-success)]">
      {t('context.cached')}
    </span>
  ) : (
    <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[9px] font-medium normal-case text-[var(--color-text-dim)]">
      {t('context.uncached')}
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
  const { t } = useTranslation('sessions')
  const cls = dropped
    ? 'bg-[color-mix(in_srgb,var(--color-warning)_14%,transparent)] text-[var(--color-warning)]'
    : accent
      ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
      : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
  return (
    <span
      className={`rounded-md px-2 py-1 ${cls} ${dim ? 'opacity-60' : ''}`}
      title={dropped ? t('context.stat.foldedHint') : dim ? t('context.stat.lazyHint') : undefined}
    >
      {dim ? (
        label
      ) : (
        <>
          {label}: <strong>{count(value)}</strong>
        </>
      )}
    </span>
  )
}
