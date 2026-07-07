import { Sparkles, Users, GitBranch, Clock } from 'lucide-react'
import type { Pack, WorkspacePayload } from '@/types'
import type { PriceTable } from '@/api/providers'
import { Markdown } from '@/shared/components/markdown/Markdown'
import {
  cacheLabel,
  flowNodeSummary,
  flowSummary,
  NODE_ICON,
  skillMeta,
  stripFrontmatter,
} from './marketHelpers'
import {
  CapBadge,
  ColumnsPreview,
  MiniChip,
  ModelList,
  PreviewSection,
  Row,
  StatChip,
} from './previewParts'

// PackPreview renders a kind-appropriate preview of the selected pack's payload.
export function PackPreview({ pack, prices }: { pack: Pack; prices: PriceTable }) {
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
          <Row k="Sağlayıcı" v={a.provider} />
          <Row k="Model" v={a.model || '(varsayılan)'} />
          <Row k="Düşünme" v={a.thinkingLevel} />
          <Row k="İzin modu" v={a.permissionMode} />
          <Row k="Skills" v={a.skills?.join(', ')} />
        </div>
      </div>
    )
  }
  if (pack.kind === 'provider' && p?.provider) {
    const pr = p.provider
    return (
      <div className="space-y-1">
        <Row k="Tür" v={pr.kind} />
        <Row k="Base URL" v={pr.baseUrl} />
        <Row k="Varsayılan model" v={pr.defaultModel} />
        <div className="flex flex-wrap gap-1.5 pt-1.5">
          <CapBadge label={cacheLabel(pr.promptCache)} on={pr.promptCache === 'native' || pr.promptCache === 'auto'} />
          <CapBadge label={pr.reasoning ? 'Düşünme: ✅ destekli' : 'Düşünme: —'} on={!!pr.reasoning} />
        </div>
        <ModelList models={pr.models} providerId={pack.id.replace(/^provider\./, '')} prices={prices} />
      </div>
    )
  }
  if (pack.kind === 'flow' && p?.flow) {
    const nodes = flowNodeSummary(p.flow.graph)
    return (
      <div className="space-y-2">
        <p className="text-xs text-[var(--color-text-dim)]">{nodes.length} düğüm:</p>
        <ul className="space-y-1">
          {nodes.map((n, i) => (
            <li key={i} className="flex gap-2 text-xs">
              <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] uppercase text-[var(--color-text-dim)]">{n.type}</span>
              <span>{n.title || n.id}</span>
            </li>
          ))}
        </ul>
        <p className="pt-1 text-[11px] text-[var(--color-text-dim)]">
          İçe aktarınca boş ajan slotları ilk ajana atanır; Akışlar ekranından düzenleyebilirsin.
        </p>
      </div>
    )
  }
  if (pack.kind === 'workspace' && p?.workspace) {
    return <WorkspacePackPreview wsp={p.workspace} />
  }
  if (pack.kind === 'mcp' && p?.mcp) {
    const m = p.mcp
    return (
      <div className="space-y-1">
        <Row k="Ad" v={m.name} />
        <Row k="Transport" v={m.transport} />
        <Row k="Komut" v={m.command} />
        <Row k="Argümanlar" v={m.args} />
        <Row k="URL" v={m.url} />
        <p className="pt-2 text-[11px] text-[var(--color-text-dim)]">
          Kurunca workspace'e bir MCP sunucusu eklenir; araçları sonraki turda görünür.
        </p>
      </div>
    )
  }
  return <p className="text-xs text-[var(--color-text-dim)]">Önizleme yok.</p>
}

// WorkspacePackPreview renders the full starter ecosystem of a workspace-template
// pack: a stat strip plus the agent team (with config), flows (with node types),
// schedules, embedded skills, instructions and board layout.
function WorkspacePackPreview({ wsp }: { wsp: WorkspacePayload }) {
  const agents = wsp.agents ?? []
  const flows = wsp.flows ?? []
  const schedules = wsp.schedules ?? []
  const skills = wsp.skills ?? []
  return (
    <div className="space-y-4">
      {/* Stat strip */}
      <div className="grid grid-cols-4 gap-2">
        <StatChip icon={Users} label="Ajan" value={agents.length} />
        <StatChip icon={GitBranch} label="Akış" value={flows.length} />
        <StatChip icon={Clock} label="Zamanlama" value={schedules.length} />
        <StatChip icon={Sparkles} label="Skill" value={skills.length} />
      </div>

      {wsp.instructions && (
        <PreviewSection title="Yönergeler">
          <p className="whitespace-pre-wrap rounded bg-[var(--color-surface-2)] p-2 text-[11px] leading-relaxed">{wsp.instructions}</p>
        </PreviewSection>
      )}

      {agents.length > 0 && (
        <PreviewSection title={`Ajanlar (${agents.length})`}>
          <div className="space-y-1.5">
            {agents.map((a) => (
              <div key={a.key} className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2">
                <div className="flex flex-wrap items-center gap-1.5">
                  <span className="text-base leading-none">{a.avatar || '🤖'}</span>
                  <span className="text-xs font-medium">{a.name}</span>
                  {a.permissionMode && <MiniChip>{a.permissionMode}</MiniChip>}
                  {a.thinkingLevel && <MiniChip>🧠 {a.thinkingLevel}</MiniChip>}
                  {a.mcpEnabled && <MiniChip>MCP</MiniChip>}
                </div>
                {(a.provider || a.model) && (
                  <div className="mt-0.5 text-[10px] text-[var(--color-text-dim)]">
                    {a.provider || '(varsayılan sağlayıcı)'}{a.model ? ` · ${a.model}` : ''}
                  </div>
                )}
                {a.soul && <p className="mt-1 line-clamp-2 text-[11px] leading-relaxed text-[var(--color-text-dim)]">{a.soul}</p>}
                {a.skills && a.skills.length > 0 && (
                  <div className="mt-1 flex flex-wrap gap-1">
                    {a.skills.map((s) => <MiniChip key={s}>📚 {s}</MiniChip>)}
                  </div>
                )}
              </div>
            ))}
          </div>
        </PreviewSection>
      )}

      {flows.length > 0 && (
        <PreviewSection title={`Akışlar (${flows.length})`}>
          <div className="space-y-1.5">
            {flows.map((f, i) => {
              const nodes = flowSummary(f)
              const hasBranch = nodes.some((n) => n.type === 'branch')
              const hasParallel = nodes.some((n) => n.type === 'parallel')
              return (
                <div key={i} className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2">
                  <div className="flex flex-wrap items-center gap-1.5 text-xs font-medium">
                    {f.name}
                    {hasBranch && <MiniChip>branch</MiniChip>}
                    {hasParallel && <MiniChip>parallel</MiniChip>}
                  </div>
                  <div className="mt-1.5 flex flex-wrap items-center gap-1">
                    {nodes.map((n, j) => {
                      const NIcon = NODE_ICON[n.type]
                      return (
                        <span key={j} className="flex items-center gap-1">
                          <span className="inline-flex items-center gap-1 rounded bg-[var(--color-bg)] px-1.5 py-0.5 text-[10px]" title={n.type}>
                            {NIcon ? <NIcon size={11} className="text-[var(--color-text-dim)]" /> : <span className="text-[var(--color-text-dim)]">•</span>}
                            {n.title || n.id}
                          </span>
                          {j < nodes.length - 1 && <span className="text-[10px] text-[var(--color-text-dim)]">→</span>}
                        </span>
                      )
                    })}
                  </div>
                </div>
              )
            })}
          </div>
        </PreviewSection>
      )}

      {schedules.length > 0 && (
        <PreviewSection title={`Zamanlamalar (${schedules.length})`}>
          <div className="space-y-1.5">
            {schedules.map((s, i) => (
              <div key={i} className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2 text-[11px]">
                <div className="flex items-center gap-2">
                  <code className="rounded bg-[var(--color-bg)] px-1.5 py-0.5 text-[10px]">{s.cronExpr}</code>
                  <span className="text-[var(--color-text-dim)]">→ {s.agentKey}</span>
                </div>
                {s.prompt && <p className="mt-1 line-clamp-2 leading-relaxed text-[var(--color-text-dim)]">{s.prompt}</p>}
              </div>
            ))}
          </div>
          <p className="mt-1 text-[10px] text-[var(--color-text-dim)]">Zamanlamalar pasif (disabled) kurulur — Zamanlamalar ekranından açılır.</p>
        </PreviewSection>
      )}

      {skills.length > 0 && (
        <PreviewSection title={`Gömülü Skill'ler (${skills.length})`}>
          <div className="space-y-1.5">
            {skills.map((s) => {
              const meta = skillMeta(s.body)
              return (
                <div key={s.slug} className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2">
                  <div className="flex items-center gap-1.5 text-xs font-medium">
                    📚 {meta.name || s.slug}
                    <code className="text-[10px] text-[var(--color-text-dim)]">{s.slug}</code>
                  </div>
                  {meta.description && <p className="mt-0.5 line-clamp-2 text-[11px] leading-relaxed text-[var(--color-text-dim)]">{meta.description}</p>}
                </div>
              )
            })}
          </div>
        </PreviewSection>
      )}

      {wsp.columns && wsp.columns.length > 0 && (
        <PreviewSection title="Board kolonları">
          <ColumnsPreview columns={wsp.columns} />
        </PreviewSection>
      )}

      <p className="pt-1 text-[11px] text-[var(--color-text-dim)]">Kurunca bu şablondan yeni bir workspace oluşturulur.</p>
    </div>
  )
}
