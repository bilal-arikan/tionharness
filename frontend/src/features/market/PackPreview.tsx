import { Sparkles, Users, Clock, Zap } from 'lucide-react'
import type { Pack, WorkspacePayload } from '@/types'
import type { PriceTable } from '@/api/providers'
import { Markdown } from '@/shared/components/markdown/Markdown'
import { resolveModelLabel } from '@/shared/lib/modelLabel'
import { useCatalog } from '@/shared/lib/catalog'
import { cacheLabel, skillMeta, stripFrontmatter } from './marketHelpers'
import {
  CapBadge,
  ColumnsPreview,
  MiniChip,
  ModelList,
  PreviewSection,
  Row,
  StatChip,
} from './previewParts'
import { useTranslation } from 'react-i18next'

// PackPreview renders a kind-appropriate preview of the selected pack's payload.
export function PackPreview({ pack, prices }: { pack: Pack; prices: PriceTable }) {
  const { t } = useTranslation('market')
  const catalog = useCatalog()
  const p = pack.payload
  if (pack.kind === 'skill' && p?.skill?.body) {
    return <Markdown>{stripFrontmatter(p.skill.body)}</Markdown>
  }
  if (pack.kind === 'agent' && p?.agent) {
    const a = p.agent
    return (
      <div className="space-y-3">
        {a.soul && <p className="text-xs leading-relaxed text-[var(--color-text)]">{a.soul}</p>}
        <div className="space-y-1">
          <Row k={t('preview.field.provider')} v={a.provider} />
          <Row
            k={t('preview.field.model')}
            v={resolveModelLabel(catalog, a.provider ?? '', a.model ?? '')}
          />
          <Row k={t('preview.field.thinking')} v={a.thinkingLevel} />
          <Row k={t('preview.field.permissionMode')} v={a.permissionMode} />
          <Row k={t('preview.field.skills')} v={a.skills?.join(', ')} />
        </div>
      </div>
    )
  }
  if (pack.kind === 'provider' && p?.provider) {
    const pr = p.provider
    return (
      <div className="space-y-1">
        <Row k={t('preview.field.type')} v={pr.kind} />
        <Row k={t('preview.field.baseUrl')} v={pr.baseUrl} />
        <Row k={t('preview.field.providerModel')} v={pr.defaultModel} />
        <div className="flex flex-wrap gap-1.5 pt-1.5">
          <CapBadge
            label={cacheLabel(pr.promptCache)}
            on={pr.promptCache === 'native' || pr.promptCache === 'auto'}
          />
          <CapBadge
            label={pr.reasoning ? t('preview.reasoningSupported') : t('preview.reasoningNone')}
            on={!!pr.reasoning}
          />
        </div>
        <ModelList
          models={pr.models}
          providerId={pack.id.replace(/^provider\./, '')}
          prices={prices}
        />
      </div>
    )
  }
  if (pack.kind === 'workspace' && p?.workspace) {
    return <WorkspacePackPreview wsp={p.workspace} catalog={catalog} />
  }
  if (pack.kind === 'mcp' && p?.mcp) {
    const m = p.mcp
    const scope = m.scope === 'scoped' ? t('preview.scope.scoped') : t('preview.scope.shared')
    return (
      <div className="space-y-1">
        <Row k={t('preview.field.name')} v={m.name} />
        <Row k={t('preview.field.description')} v={m.description} />
        <Row k={t('preview.field.transport')} v={m.transport} />
        <Row k={t('preview.field.scope')} v={scope} />
        <Row k={t('preview.field.command')} v={m.command} />
        <Row k={t('preview.field.arguments')} v={m.args} />
        <Row k="URL" v={m.url} />
        <Row k={t('preview.field.headers')} v={m.headersConfig} />
        <p className="pt-2 text-[11px] text-[var(--color-text-dim)]">
          {t('preview.mcpInstallHint')}
        </p>
      </div>
    )
  }
  if (pack.kind === 'hook' && p?.hook) {
    const h = p.hook
    return (
      <div className="space-y-1">
        <Row k={t('preview.field.event')} v={h.event} />
        <Row k={t('preview.field.matcher')} v={h.matcher || t('preview.allTools')} />
        <Row
          k={t('preview.field.timeout')}
          v={h.timeoutSec ? t('preview.seconds', { count: h.timeoutSec }) : undefined}
        />
        <div className="pt-1">
          <span className="text-xs text-[var(--color-text-dim)]">{t('preview.field.command')}</span>
          <pre className="mt-1 overflow-x-auto rounded bg-[var(--color-surface-2)] p-2 text-[11px]">
            {h.command}
          </pre>
        </div>
        <p className="pt-2 text-[11px] text-[var(--color-text-dim)]">
          {t('preview.hookInstallHint')}
        </p>
      </div>
    )
  }
  return <p className="text-xs text-[var(--color-text-dim)]">{t('preview.none')}</p>
}

// WorkspacePackPreview renders the full starter ecosystem of a workspace-template
// pack: a stat strip plus the agent team (with config), schedules, automations, embedded skills, instructions and board layout.
function WorkspacePackPreview({
  wsp,
  catalog,
}: {
  wsp: WorkspacePayload
  catalog: ReturnType<typeof useCatalog>
}) {
  const { t } = useTranslation('market')
  const agents = wsp.agents ?? []
  const schedules = wsp.schedules ?? []
  const automations = wsp.automations ?? []
  const skills = wsp.skills ?? []
  const promptKeys = Object.keys(wsp.prompts ?? {})
  // An agent key → display name map, so an automation's target reads as the agent
  // the installer will actually see rather than the template's internal key.
  const agentName = (key: string) => agents.find((a) => a.key === key)?.name || key
  return (
    <div className="space-y-4">
      {/* Stat strip. Automations only take a slot when the pack ships some — an
          always-visible "0" would imply every template has them. */}
      <div className={`grid gap-2 ${automations.length > 0 ? 'grid-cols-4' : 'grid-cols-3'}`}>
        <StatChip icon={Users} label={t('preview.workspace.agent')} value={agents.length} />
        <StatChip icon={Clock} label={t('preview.workspace.schedule')} value={schedules.length} />
        {automations.length > 0 && (
          <StatChip
            icon={Zap}
            label={t('preview.workspace.automation')}
            value={automations.length}
          />
        )}
        <StatChip icon={Sparkles} label={t('preview.workspace.skill')} value={skills.length} />
      </div>

      {wsp.instructions && (
        <PreviewSection title={t('preview.workspace.instructions')}>
          <p className="whitespace-pre-wrap rounded bg-[var(--color-surface-2)] p-2 text-[11px] leading-relaxed">
            {wsp.instructions}
          </p>
        </PreviewSection>
      )}

      {agents.length > 0 && (
        <PreviewSection title={t('preview.workspace.agents', { count: agents.length })}>
          <div className="space-y-1.5">
            {agents.map((a) => (
              <div
                key={a.key}
                className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2"
              >
                <div className="flex flex-wrap items-center gap-1.5">
                  <span className="text-base leading-none">{a.avatar || '🤖'}</span>
                  <span className="text-xs font-medium">{a.name}</span>
                  {a.permissionMode && <MiniChip>{a.permissionMode}</MiniChip>}
                  {a.thinkingLevel && <MiniChip>🧠 {a.thinkingLevel}</MiniChip>}
                  {a.mcpEnabled && <MiniChip>MCP</MiniChip>}
                  {/* The headline property of an orchestrating template: which agents
                      arrive able to drive workers. Invisible otherwise until install. */}
                  {a.coordinatorMode && (
                    <MiniChip title={t('preview.workspace.coordinatorTitle')}>
                      🕸 {t('preview.workspace.coordinator')}
                    </MiniChip>
                  )}
                  {a.coordinatorWorkflow && <MiniChip>📐 {a.coordinatorWorkflow}</MiniChip>}
                </div>
                {(a.provider || a.model) && (
                  <div className="mt-0.5 text-[10px] text-[var(--color-text-dim)]">
                    {a.provider || t('preview.workspace.providerUnspecified')}
                    {' · '}
                    {resolveModelLabel(catalog, a.provider ?? '', a.model ?? '')}
                  </div>
                )}
                {a.soul && (
                  <p className="mt-1 line-clamp-2 text-[11px] leading-relaxed text-[var(--color-text-dim)]">
                    {a.soul}
                  </p>
                )}
                {a.skills && a.skills.length > 0 && (
                  <div className="mt-1 flex flex-wrap gap-1">
                    {a.skills.map((s) => (
                      <MiniChip key={s}>📚 {s}</MiniChip>
                    ))}
                  </div>
                )}
                {(a.toolOverrides || a.blockedTools) && (
                  <div className="mt-1 flex flex-wrap gap-1">
                    <MiniChip>🔧 {t('preview.workspace.toolOverrides')}</MiniChip>
                  </div>
                )}
              </div>
            ))}
          </div>
        </PreviewSection>
      )}

      {schedules.length > 0 && (
        <PreviewSection
          title={t('preview.workspace.schedules', { count: schedules.length })}
          info={t('preview.workspace.schedulesHint')}
        >
          <div className="space-y-1.5">
            {schedules.map((s, i) => (
              <div
                key={i}
                className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2 text-[11px]"
              >
                <div className="flex items-center gap-2">
                  <code className="rounded bg-[var(--color-bg)] px-1.5 py-0.5 text-[10px]">
                    {s.cronExpr}
                  </code>
                  <span className="text-[var(--color-text-dim)]">
                    {'→ '}
                    {s.agentKey}
                  </span>
                </div>
                {s.prompt && (
                  <p className="mt-1 line-clamp-2 leading-relaxed text-[var(--color-text-dim)]">
                    {s.prompt}
                  </p>
                )}
              </div>
            ))}
          </div>
        </PreviewSection>
      )}

      {automations.length > 0 && (
        <PreviewSection
          title={t('preview.workspace.automations', { count: automations.length })}
          info={t('preview.workspace.automationsHint')}
        >
          <div className="space-y-1.5">
            {automations.map((a, i) => (
              <div
                key={i}
                className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2 text-[11px]"
              >
                <div className="flex flex-wrap items-center gap-1.5">
                  <span className="text-xs font-medium">{a.name}</span>
                  <MiniChip>{a.triggerKind || 'tag'}</MiniChip>
                  {a.boardToState && (
                    <MiniChip>
                      {'→ '}
                      {a.boardToState}
                    </MiniChip>
                  )}
                  {a.boardAction && <MiniChip>{a.boardAction}</MiniChip>}
                  {/* An exclusive rule silences every other rule on the same card
                      change — worth seeing BEFORE install, not after. */}
                  {a.boardExclusive && (
                    <MiniChip title={t('preview.workspace.exclusiveTitle')}>
                      {t('preview.workspace.exclusive')}
                    </MiniChip>
                  )}
                </div>
                <div className="mt-0.5 text-[10px] text-[var(--color-text-dim)]">
                  {a.agentKey
                    ? t('preview.workspace.agentTarget', { name: agentName(a.agentKey) })
                    : t('preview.workspace.noLlmCall')}
                </div>
                {a.promptTemplate && (
                  <p className="mt-1 line-clamp-2 leading-relaxed text-[var(--color-text-dim)]">
                    {a.promptTemplate}
                  </p>
                )}
              </div>
            ))}
          </div>
        </PreviewSection>
      )}

      {skills.length > 0 && (
        <PreviewSection title={t('preview.workspace.embeddedSkills', { count: skills.length })}>
          <div className="space-y-1.5">
            {skills.map((s) => {
              const meta = skillMeta(s.body)
              return (
                <div
                  key={s.slug}
                  className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2"
                >
                  <div className="flex items-center gap-1.5 text-xs font-medium">
                    📚 {meta.name || s.slug}
                    <code className="text-[10px] text-[var(--color-text-dim)]">{s.slug}</code>
                  </div>
                  {meta.description && (
                    <p className="mt-0.5 line-clamp-2 text-[11px] leading-relaxed text-[var(--color-text-dim)]">
                      {meta.description}
                    </p>
                  )}
                </div>
              )
            })}
          </div>
        </PreviewSection>
      )}

      {wsp.columns && wsp.columns.length > 0 && (
        <PreviewSection title={t('preview.workspace.boardColumns')}>
          <ColumnsPreview columns={wsp.columns} />
        </PreviewSection>
      )}

      {promptKeys.length > 0 && (
        <PreviewSection
          title={t('preview.workspace.promptOverrides', { count: promptKeys.length })}
          info={t('preview.workspace.promptOverridesHint')}
        >
          <div className="flex flex-wrap gap-1">
            {promptKeys.map((k) => (
              <MiniChip key={k}>{k}</MiniChip>
            ))}
          </div>
        </PreviewSection>
      )}

      {wsp.readme && (
        <PreviewSection title="config/README.md">
          <p className="line-clamp-6 whitespace-pre-wrap rounded bg-[var(--color-surface-2)] p-2 text-[11px] leading-relaxed">
            {wsp.readme}
          </p>
        </PreviewSection>
      )}

      <p className="pt-1 text-[11px] text-[var(--color-text-dim)]">
        {t('preview.workspace.installHint')}
      </p>
    </div>
  )
}
