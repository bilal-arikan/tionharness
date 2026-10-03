import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, History, RotateCcw, Sparkles, X } from 'lucide-react'
import type {
  AgentPromptVersion,
  Flow,
  FlowAuthor,
  FlowPolicy,
  FlowProposal,
  FlowVersion,
} from '@/types'
import { api } from '@/api'
import { Badge, Button, InfoPopover, type BadgeTone } from '@/shared/components'
import { OptionPills } from '@/shared/components/OptionPills'
import { relativeTime, fullDateTime } from '@/shared/lib/time'

interface Props {
  flow: Flow
  versions: FlowVersion[]
  proposals: FlowProposal[]
  promptVersions: AgentPromptVersion[]
  busy: boolean
  onChanged: () => void
  onError: (msg: string) => void
  onOpenSession?: (sessionId: string) => void
}

function authorLabel(a: FlowAuthor, t: (k: string) => string): string {
  return t(`author.${a.kind}`)
}

function authorTone(a: FlowAuthor): BadgeTone {
  switch (a.kind) {
    case 'observer':
      return 'accent'
    case 'agent':
      return 'warning'
    case 'user':
      return 'success'
    default:
      return 'muted'
  }
}

function proposalTone(status: FlowProposal['status']): BadgeTone {
  switch (status) {
    case 'pending':
      return 'warning'
    case 'applied':
      return 'success'
    case 'rejected':
      return 'muted'
    default:
      return 'danger'
  }
}

function describeOps(ops: unknown[] | null | undefined): string[] {
  if (!Array.isArray(ops)) return []
  return ops.map((raw) => {
    const op = raw as {
      op?: string
      id?: string
      from?: string
      to?: string
      node?: { id?: string; type?: string }
      edge?: { from?: string; to?: string; when?: string }
    }
    switch (op.op) {
      case 'insert_between':
        return `insert ${op.node?.id ?? '?'}(${op.node?.type ?? '?'}) ${op.from}→${op.to}`
      case 'add_node':
        return `add ${op.node?.id ?? '?'}(${op.node?.type ?? '?'})`
      case 'remove_node':
        return `remove ${op.id}`
      case 'update_node':
        return `update ${op.id}`
      case 'add_edge':
        return `edge ${op.edge?.from}→${op.edge?.to}${op.edge?.when ? ` [${op.edge.when}]` : ''}`
      case 'update_edge':
        return `edge ${op.id}${op.edge?.when !== undefined ? ` [${op.edge.when || '*'}]` : ''}`
      case 'remove_edge':
        return `remove edge ${op.id}`
      default:
        return op.op ?? '?'
    }
  })
}

function Section({
  title,
  icon: Icon,
  children,
  hint,
}: {
  title: string
  icon: typeof History
  hint?: string
  children: React.ReactNode
}) {
  return (
    <section className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)]">
      <h3 className="flex items-center gap-2 border-b border-[var(--color-border)] px-3 py-2 text-xs font-semibold">
        <Icon size={14} className="text-[var(--color-accent)]" />
        {title}
        {hint && <InfoPopover text={hint} fixed />}
      </h3>
      <div className="p-3">{children}</div>
    </section>
  )
}

// EvolutionTab is where the flow's history lives: the observer policy, the
// proposals waiting for a decision, the version timeline with revert, and the
// agent's prompt versions.
export function EvolutionTab({
  flow,
  versions,
  proposals,
  promptVersions,
  busy,
  onChanged,
  onError,
  onOpenSession,
}: Props) {
  const { t } = useTranslation('flows')
  const [policy, setPolicy] = useState<FlowPolicy>(flow.policy)
  const [policyDirty, setPolicyDirty] = useState(false)
  const [optimizing, setOptimizing] = useState(false)
  const [openPrompt, setOpenPrompt] = useState<number | null>(null)
  const [note, setNote] = useState<string | null>(null)

  const setPol = (patch: Partial<FlowPolicy>) => {
    setPolicy((p) => ({ ...p, ...patch }))
    setPolicyDirty(true)
  }
  const savePolicy = () => {
    api
      .updateFlowMeta(flow.id, { policy })
      .then(() => {
        setPolicyDirty(false)
        onChanged()
      })
      .catch((e: Error) => onError(e.message))
  }
  const optimize = () => {
    setOptimizing(true)
    setNote(null)
    api
      .optimizeFlow(flow.id)
      .then((res) => {
        if (res.applied) setNote(t('evolution.optimizeApplied'))
        else if (res.held) setNote(t('evolution.optimizeHeld', { reason: res.held }))
        else if (res.proposal) setNote(t('evolution.optimizeProposed'))
        else setNote(res.skipped || t('evolution.optimizeNothing'))
        onChanged()
      })
      .catch((e: Error) => onError(e.message))
      .finally(() => setOptimizing(false))
  }
  const decide = (p: FlowProposal, apply: boolean) => {
    const call = apply ? api.applyFlowProposal(p.id) : api.rejectFlowProposal(p.id)
    call.then(() => onChanged()).catch((e: Error) => onError(e.message))
  }
  const revert = (v: FlowVersion) => {
    api
      .revertFlow(flow.id, v.version)
      .then(() => onChanged())
      .catch((e: Error) => onError(e.message))
  }
  const restorePrompt = (v: AgentPromptVersion) => {
    api
      .restorePromptVersion(flow.agentId, v.version)
      .then(() => onChanged())
      .catch((e: Error) => onError(e.message))
  }
  const runsSince = flow.stats.runs - flow.stats.runsAtOptimize
  const pending = proposals.filter((p) => p.status === 'pending')
  const resolved = proposals.filter((p) => p.status !== 'pending')

  return (
    <div className="h-full min-h-0 overflow-y-auto">
      <div className="mx-auto grid max-w-6xl gap-3 p-3 md:p-4 xl:grid-cols-2 [&>section]:min-w-0">
        <Section title={t('evolution.policy')} icon={Sparkles} hint={t('evolution.policyHint')}>
          <div className="space-y-3 text-xs">
            <OptionPills
              value={policy.mode}
              onChange={(v) => setPol({ mode: v as FlowPolicy['mode'] })}
              ariaLabel={t('evolution.policy')}
              testid="flow-policy-mode"
              options={[
                { value: 'off', label: t('policy.off'), hint: t('policy.offHint') },
                { value: 'propose', label: t('policy.propose'), hint: t('policy.proposeHint') },
                { value: 'auto', label: t('policy.auto'), hint: t('policy.autoHint') },
              ]}
            />
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
              <label className="space-y-1">
                <span className="text-[11px] text-[var(--color-text-dim)]">
                  {t('evolution.everyRuns')}
                </span>
                <input
                  type="number"
                  min={1}
                  max={100}
                  value={policy.everyRuns}
                  onChange={(e) => setPol({ everyRuns: Math.max(1, Number(e.target.value) || 1) })}
                  className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1"
                />
              </label>
              <label className="space-y-1">
                <span className="text-[11px] text-[var(--color-text-dim)]">
                  {t('evolution.minConfidence')}
                </span>
                <input
                  type="number"
                  min={0.1}
                  max={1}
                  step={0.05}
                  value={policy.minConfidence}
                  onChange={(e) =>
                    setPol({
                      minConfidence: Math.min(1, Math.max(0.1, Number(e.target.value) || 0.7)),
                    })
                  }
                  className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1"
                />
              </label>
              <label className="space-y-1">
                <span className="text-[11px] text-[var(--color-text-dim)]">
                  {t('evolution.maxNodes')}
                </span>
                <input
                  type="number"
                  min={3}
                  max={48}
                  value={policy.maxNodes}
                  onChange={(e) => setPol({ maxNodes: Math.max(3, Number(e.target.value) || 16) })}
                  className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1"
                />
              </label>
            </div>
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={policy.allowPromptChanges}
                onChange={(e) => setPol({ allowPromptChanges: e.target.checked })}
              />
              <span>{t('evolution.allowPromptChanges')}</span>
            </label>
            <div className="flex flex-wrap items-center gap-2">
              <Button
                size="sm"
                variant="secondary"
                disabled={!policyDirty || busy}
                onClick={savePolicy}
              >
                {t('evolution.savePolicy')}
              </Button>
              <Button
                size="sm"
                disabled={optimizing || busy}
                onClick={optimize}
                data-testid="flow-optimize-now"
              >
                <Sparkles size={12} />{' '}
                {optimizing ? t('evolution.optimizing') : t('evolution.optimizeNow')}
              </Button>
              <span className="text-[11px] text-[var(--color-text-dim)]">
                {t('evolution.runsSince', { n: runsSince, every: policy.everyRuns })}
                {(flow.stats.graded ?? 0) > 0 && (
                  <>
                    {' · '}
                    {t('evolution.avgGrade', {
                      avg: ((flow.stats.gradeSum ?? 0) / (flow.stats.graded ?? 1)).toFixed(1),
                      n: flow.stats.graded,
                    })}
                  </>
                )}
              </span>
            </div>
            {note && (
              <p className="rounded bg-[var(--color-surface-2)] px-2 py-1 text-[11px]">{note}</p>
            )}
          </div>
        </Section>

        <Section title={t('evolution.proposals')} icon={Check} hint={t('evolution.proposalsHint')}>
          {proposals.length === 0 ? (
            <p className="text-xs text-[var(--color-text-dim)]">{t('evolution.noProposals')}</p>
          ) : (
            <ul className="space-y-2 text-xs" data-testid="flow-proposals">
              {[...pending, ...resolved].map((p) => (
                <li key={p.id} className="rounded-md border border-[var(--color-border)] p-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge tone={proposalTone(p.status)}>{t(`proposal.${p.status}`)}</Badge>
                    <Badge tone={authorTone(p.author)}>{authorLabel(p.author, t)}</Badge>
                    <span className="text-[var(--color-text-dim)]">
                      {t('evolution.confidenceBase', {
                        pct: Math.round(p.confidence * 100),
                        v: p.baseVersion,
                      })}
                    </span>
                    <span
                      className="ml-auto text-[10px] text-[var(--color-text-dim)]"
                      title={fullDateTime(p.createdAt)}
                    >
                      {relativeTime(p.createdAt)}
                    </span>
                  </div>
                  <p className="mt-1 break-words">{p.reason}</p>
                  {p.expected && (
                    <p className="mt-0.5 break-words text-[var(--color-text-dim)]">{p.expected}</p>
                  )}
                  {describeOps(p.ops).length > 0 && (
                    <ul className="mt-1 list-inside list-disc font-mono text-[10px] text-[var(--color-text-dim)]">
                      {describeOps(p.ops).map((line, i) => (
                        <li key={i}>{line}</li>
                      ))}
                    </ul>
                  )}
                  {p.prompt && (p.prompt.soul || p.prompt.identity) && (
                    <p className="mt-1 text-[10px] text-[var(--color-text-dim)]">
                      {t('evolution.promptChange')}
                    </p>
                  )}
                  {p.error && (
                    <p className="mt-1 text-[10px] text-[var(--color-danger)]">{p.error}</p>
                  )}
                  {p.status === 'applied' && p.appliedVersion ? (
                    <p className="mt-1 text-[10px] text-[var(--color-text-dim)]">
                      {t('evolution.appliedAs', { v: p.appliedVersion })}
                    </p>
                  ) : null}
                  {p.status === 'pending' && (
                    <div className="mt-2 flex gap-2">
                      <Button
                        size="sm"
                        onClick={() => decide(p, true)}
                        data-testid="flow-proposal-apply"
                      >
                        <Check size={12} /> {t('evolution.apply')}
                      </Button>
                      <Button size="sm" variant="secondary" onClick={() => decide(p, false)}>
                        <X size={12} /> {t('evolution.reject')}
                      </Button>
                    </div>
                  )}
                </li>
              ))}
            </ul>
          )}
        </Section>

        <Section title={t('evolution.versions')} icon={History} hint={t('evolution.versionsHint')}>
          <ol className="space-y-1 text-xs" data-testid="flow-versions">
            {versions.map((v) => (
              <li
                key={v.version}
                className="flex flex-wrap items-center gap-2 rounded-md border border-[var(--color-border)] px-2 py-1.5"
              >
                <span
                  className={`font-mono ${v.version === flow.version ? 'text-[var(--color-accent)]' : ''}`}
                >
                  {t('version', { v: v.version })}
                </span>
                <Badge tone={authorTone(v.author)}>{authorLabel(v.author, t)}</Badge>
                <span className="min-w-0 flex-1 truncate" title={v.reason}>
                  {v.reason || t('evolution.noReason')}
                </span>
                {v.diff && (
                  <span className="font-mono text-[10px] text-[var(--color-text-dim)]">
                    {v.diff}
                  </span>
                )}
                <span
                  className="text-[10px] text-[var(--color-text-dim)]"
                  title={fullDateTime(v.createdAt)}
                >
                  {relativeTime(v.createdAt)}
                </span>
                {v.version !== flow.version && (
                  <button
                    type="button"
                    onClick={() => revert(v)}
                    className="flex items-center gap-1 rounded border border-[var(--color-border)] px-1.5 py-0.5 text-[10px] hover:bg-[var(--color-surface-2)]"
                    title={t('evolution.revertTo', { v: v.version })}
                  >
                    <RotateCcw size={10} /> {t('evolution.revert')}
                  </button>
                )}
              </li>
            ))}
          </ol>
        </Section>

        <Section
          title={t('evolution.promptVersions')}
          icon={History}
          hint={t('evolution.promptVersionsHint')}
        >
          {promptVersions.length === 0 ? (
            <p className="text-xs text-[var(--color-text-dim)]">
              {t('evolution.noPromptVersions')}
            </p>
          ) : (
            <ol className="space-y-1 text-xs">
              {promptVersions.map((v) => (
                <li
                  key={v.version}
                  className="rounded-md border border-[var(--color-border)] px-2 py-1.5"
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono">{t('version', { v: v.version })}</span>
                    <Badge tone={authorTone(v.author)}>{authorLabel(v.author, t)}</Badge>
                    <span className="min-w-0 flex-1 truncate">
                      {v.reason || t('evolution.noReason')}
                    </span>
                    <span
                      className="text-[10px] text-[var(--color-text-dim)]"
                      title={fullDateTime(v.createdAt)}
                    >
                      {relativeTime(v.createdAt)}
                    </span>
                    <button
                      type="button"
                      onClick={() => setOpenPrompt(openPrompt === v.version ? null : v.version)}
                      className="rounded border border-[var(--color-border)] px-1.5 py-0.5 text-[10px] hover:bg-[var(--color-surface-2)]"
                    >
                      {openPrompt === v.version ? t('evolution.hide') : t('evolution.show')}
                    </button>
                    {v.version !== promptVersions[0]?.version && (
                      <button
                        type="button"
                        onClick={() => restorePrompt(v)}
                        className="flex items-center gap-1 rounded border border-[var(--color-border)] px-1.5 py-0.5 text-[10px] hover:bg-[var(--color-surface-2)]"
                      >
                        <RotateCcw size={10} /> {t('evolution.restore')}
                      </button>
                    )}
                  </div>
                  {openPrompt === v.version && (
                    <div className="mt-2 grid gap-2 text-[11px] md:grid-cols-2">
                      <div>
                        <div className="mb-0.5 text-[var(--color-text-dim)]">
                          {t('evolution.soul')}
                        </div>
                        <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded bg-[var(--color-bg)] p-2">
                          {v.soul}
                        </pre>
                      </div>
                      <div>
                        <div className="mb-0.5 text-[var(--color-text-dim)]">
                          {t('evolution.identity')}
                        </div>
                        <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded bg-[var(--color-bg)] p-2">
                          {v.identity}
                        </pre>
                      </div>
                    </div>
                  )}
                </li>
              ))}
            </ol>
          )}
          {onOpenSession && flow.stats.lastOptimizeAt ? (
            <p className="mt-2 text-[10px] text-[var(--color-text-dim)]">
              {t('evolution.lastObserved', { when: relativeTime(flow.stats.lastOptimizeAt) })}
            </p>
          ) : null}
        </Section>
      </div>
    </div>
  )
}
