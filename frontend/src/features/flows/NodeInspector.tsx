import { PanelRightClose, Trash2, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type {
  Agent,
  Automation,
  FlowEdge,
  FlowGraph,
  FlowNode,
  FlowNodeType,
  RouteMode,
} from '@/types'
import { PromptEditor } from '@/shared/components/PromptEditor'
import { OptionPills } from '@/shared/components/OptionPills'
import { FieldHint, InfoPopover } from '@/shared/components/InfoPopover'
import { NODE_CHROME } from './nodeChrome'
import { outgoing } from './flowGraph'

interface Props {
  graph: FlowGraph
  nodeId: string | null
  edgeId: string | null
  agents: Agent[]
  automations: Automation[]
  ownerAgentId: string
  readOnly: boolean
  onChangeNode: (id: string, patch: Partial<FlowNode>) => void
  onChangeEdge: (id: string, patch: Partial<FlowEdge>) => void
  onDeleteNode: (id: string) => void
  onDeleteEdge: (id: string) => void
  onClose: () => void
  // When set, the empty state shows a button that folds the docked panel away.
  onCollapse?: () => void
}

const inputCls =
  'w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-xs text-[var(--color-text)] focus:border-[var(--color-accent)] focus:outline-none'

function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: React.ReactNode
}) {
  return (
    <label className="block space-y-1">
      <span className="flex items-center gap-1 text-[11px] font-medium text-[var(--color-text-dim)]">
        {label}
        {hint && <InfoPopover text={hint} mode="long" />}
      </span>
      {children}
      <FieldHint text={hint} className="text-[11px]" />
    </label>
  )
}

// NodeInspector edits the selected node or edge. It is docked on wide
// landscape screens and shown as a bottom sheet elsewhere (the parent decides;
// this component only renders the fields).
export function NodeInspector({
  graph,
  nodeId,
  edgeId,
  agents,
  automations,
  ownerAgentId,
  readOnly,
  onChangeNode,
  onChangeEdge,
  onDeleteNode,
  onDeleteEdge,
  onClose,
  onCollapse,
}: Props) {
  const { t } = useTranslation('flows')
  const node = nodeId ? graph.nodes.find((n) => n.id === nodeId) : undefined
  const edge = edgeId ? graph.edges.find((e) => e.id === edgeId) : undefined

  if (!node && !edge) {
    return (
      <div className="p-4 text-xs text-[var(--color-text-dim)]">
        <div className="flex items-start justify-between gap-2">
          <p className="flex items-center gap-1">
            {t('inspector.empty')}
            <InfoPopover text={t('inspector.emptyHint')} />
          </p>
          {onCollapse && (
            <button
              type="button"
              onClick={onCollapse}
              title={t('inspector.collapse')}
              aria-label={t('inspector.collapse')}
              data-testid="flow-inspector-collapse"
              className="-mr-1 -mt-1 shrink-0 rounded p-1 hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
            >
              <PanelRightClose size={14} />
            </button>
          )}
        </div>
      </div>
    )
  }

  if (edge && !node) {
    const from = graph.nodes.find((n) => n.id === edge.from)
    const isRoute = from?.type === 'route'
    return (
      <div className="space-y-3 p-4 text-xs" data-testid="flow-edge-inspector">
        <div className="flex items-center justify-between">
          <span className="font-semibold">{t('inspector.edge')}</span>
          <button
            type="button"
            onClick={onClose}
            className="rounded p-1 hover:bg-[var(--color-surface-2)]"
          >
            <X size={14} />
          </button>
        </div>
        <p className="font-mono text-[11px] text-[var(--color-text-dim)]">
          {edge.from} → {edge.to}
        </p>
        {isRoute ? (
          <Field label={t('inspector.when')} hint={t('inspector.whenHint')}>
            <input
              className={inputCls}
              value={edge.when ?? ''}
              disabled={readOnly}
              placeholder={t('inspector.whenDefault')}
              onChange={(e) => onChangeEdge(edge.id, { when: e.target.value })}
              data-testid="flow-edge-when"
            />
          </Field>
        ) : (
          <p className="text-[var(--color-text-dim)]">{t('inspector.linearEdge')}</p>
        )}
        {!readOnly && (
          <button
            type="button"
            onClick={() => onDeleteEdge(edge.id)}
            className="flex items-center gap-1 rounded border border-[var(--color-danger)]/50 px-2 py-1 text-[var(--color-danger)] hover:bg-[var(--color-danger)]/10"
          >
            <Trash2 size={12} /> {t('inspector.deleteEdge')}
          </button>
        )}
      </div>
    )
  }

  const n = node!
  const chrome = NODE_CHROME[n.type]
  const Icon = chrome.Icon
  const set = (patch: Partial<FlowNode>) => onChangeNode(n.id, patch)
  const arms = n.type === 'route' ? outgoing(graph, n.id) : []
  const protectedNode = n.type === 'input' || n.type === 'output'
  return (
    <div className="space-y-3 p-4 text-xs" data-testid="flow-node-inspector">
      <div className="flex items-center gap-2">
        <Icon size={15} style={{ color: chrome.accent }} />
        <span className="font-semibold">{t(`types.${n.type}`)}</span>
        <span className="font-mono text-[10px] text-[var(--color-text-dim)]">{n.id}</span>
        <button
          type="button"
          onClick={onClose}
          className="ml-auto rounded p-1 hover:bg-[var(--color-surface-2)]"
          title={t('inspector.close')}
        >
          <X size={14} />
        </button>
      </div>
      <Field label={t('inspector.title')}>
        <input
          className={inputCls}
          value={n.title ?? ''}
          disabled={readOnly}
          onChange={(e) => set({ title: e.target.value })}
          data-testid="flow-node-title"
        />
      </Field>
      {n.type === 'llm' && (
        <>
          <Field label={t('inspector.prompt')} hint={t('inspector.promptHint')}>
            <PromptEditor
              value={n.prompt ?? ''}
              onChange={(v) => set({ prompt: v })}
              mono
              rows={5}
              autoSizeMax={260}
              disabled={readOnly}
              placeholder="{{input}}"
            />
          </Field>
          <Field label={t('inspector.context')} hint={t('inspector.contextHint')}>
            <OptionPills
              value={n.context ?? 'thread'}
              onChange={(v) => set({ context: v as 'thread' | 'fresh' })}
              ariaLabel={t('inspector.context')}
              options={[
                { value: 'thread', label: t('context.thread') },
                { value: 'fresh', label: t('context.fresh') },
              ]}
            />
          </Field>
          <Field label={t('inspector.tools')} hint={t('inspector.toolsHint')}>
            <OptionPills
              value={n.tools ?? 'inherit'}
              onChange={(v) => set({ tools: v as 'inherit' | 'none' })}
              ariaLabel={t('inspector.tools')}
              options={[
                { value: 'inherit', label: t('tools.inherit') },
                { value: 'none', label: t('tools.none') },
              ]}
            />
          </Field>
          <Field label={t('inspector.agent')} hint={t('inspector.agentHint')}>
            <select
              className={inputCls}
              value={n.agentId ?? ''}
              disabled={readOnly}
              onChange={(e) => set({ agentId: e.target.value || undefined })}
            >
              <option value="">{t('inspector.ownerAgent')}</option>
              {agents
                .filter((a) => !a.system && !a.deleted && a.id !== ownerAgentId)
                .map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
            </select>
          </Field>
          <Field label={t('inspector.model')} hint={t('inspector.modelHint')}>
            <input
              className={inputCls}
              value={n.model ?? ''}
              disabled={readOnly}
              onChange={(e) => set({ model: e.target.value || undefined })}
            />
          </Field>
          <Field label={t('inspector.outputSchema')} hint={t('inspector.outputSchemaHint')}>
            <textarea
              className={`${inputCls} font-mono`}
              rows={3}
              value={n.outputSchema ?? ''}
              disabled={readOnly}
              onChange={(e) => set({ outputSchema: e.target.value || undefined })}
            />
          </Field>
        </>
      )}
      {n.type === 'route' && (
        <>
          <Field label={t('inspector.mode')} hint={t('inspector.modeHint')}>
            <OptionPills
              value={n.mode ?? 'contains'}
              onChange={(v) => set({ mode: v as RouteMode })}
              ariaLabel={t('inspector.mode')}
              options={(
                ['contains', 'equals', 'regex', 'json', 'judge', 'criteria'] as RouteMode[]
              ).map((m) => ({
                value: m,
                label: t(`mode.${m}`),
              }))}
            />
          </Field>
          {n.mode === 'json' && (
            <Field label={t('inspector.jsonField')}>
              <input
                className={inputCls}
                value={n.jsonField ?? ''}
                disabled={readOnly}
                onChange={(e) => set({ jsonField: e.target.value })}
              />
            </Field>
          )}
          {n.mode === 'judge' && (
            <Field label={t('inspector.question')} hint={t('inspector.questionHint')}>
              <input
                className={inputCls}
                value={n.question ?? ''}
                disabled={readOnly}
                onChange={(e) => set({ question: e.target.value })}
              />
            </Field>
          )}
          {n.mode === 'criteria' && (
            <Field label={t('inspector.criteria')} hint={t('inspector.criteriaHint')}>
              <textarea
                className={inputCls}
                rows={4}
                value={(n.criteria ?? []).join('\n')}
                disabled={readOnly}
                placeholder={t('inspector.criteriaPlaceholder')}
                onChange={(e) => set({ criteria: e.target.value.split('\n') })}
                onBlur={(e) =>
                  set({
                    criteria: e.target.value
                      .split('\n')
                      .map((c) => c.trim())
                      .filter(Boolean),
                  })
                }
                data-testid="flow-node-criteria"
              />
            </Field>
          )}
          <Field label={t('inspector.maxVisits')} hint={t('inspector.maxVisitsHint')}>
            <input
              type="number"
              min={1}
              max={20}
              className={inputCls}
              value={n.maxVisits ?? 3}
              disabled={readOnly}
              onChange={(e) => set({ maxVisits: Math.max(1, Number(e.target.value) || 1) })}
            />
          </Field>
          <div className="space-y-1">
            <span className="text-[11px] font-medium text-[var(--color-text-dim)]">
              {t('inspector.arms')}
            </span>
            {arms.length === 0 && (
              <p className="text-[var(--color-text-dim)]">{t('inspector.noArms')}</p>
            )}
            {arms.map((e) => (
              <div key={e.id} className="flex items-center gap-1">
                <input
                  className={`${inputCls} font-mono`}
                  value={e.when ?? ''}
                  disabled={readOnly}
                  placeholder={t('inspector.whenDefault')}
                  onChange={(ev) => onChangeEdge(e.id, { when: ev.target.value })}
                />
                <span className="shrink-0 font-mono text-[10px] text-[var(--color-text-dim)]">
                  → {e.to}
                </span>
              </div>
            ))}
          </div>
        </>
      )}
      {n.type === 'trigger' && (
        <>
          <Field label={t('inspector.automation')} hint={t('inspector.automationHint')}>
            <select
              className={inputCls}
              value={n.automationId ?? ''}
              disabled={readOnly}
              onChange={(e) => set({ automationId: e.target.value || undefined })}
              data-testid="flow-node-automation"
            >
              <option value="">{t('inspector.pickAutomation')}</option>
              {automations
                .filter((a) => !a.archived)
                .map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name || a.id}
                    {a.enabled ? '' : ` ${t('inspector.disabledSuffix')}`}
                  </option>
                ))}
            </select>
            {automations.length === 0 && (
              <p className="mt-1 text-[11px] text-[var(--color-text-dim)]">
                {t('inspector.noAutomations')}
              </p>
            )}
          </Field>
          <Field label={t('inspector.payload')} hint={t('inspector.payloadHint')}>
            <PromptEditor
              value={n.template ?? ''}
              onChange={(v) => set({ template: v })}
              mono
              rows={3}
              autoSizeMax={200}
              disabled={readOnly}
              placeholder="{{last}}"
            />
          </Field>
        </>
      )}
      {(n.type === 'transform' || n.type === 'output') && (
        <Field label={t('inspector.template')} hint={t('inspector.templateHint')}>
          <PromptEditor
            value={n.template ?? ''}
            onChange={(v) => set({ template: v })}
            mono
            rows={4}
            autoSizeMax={220}
            disabled={readOnly}
            placeholder="{{last}}"
          />
        </Field>
      )}
      <Field label={t('inspector.note')} hint={t('inspector.noteHint')}>
        <textarea
          className={inputCls}
          rows={2}
          value={n.note ?? ''}
          disabled={readOnly}
          onChange={(e) => set({ note: e.target.value || undefined })}
        />
      </Field>
      {!readOnly && !protectedNode && (
        <button
          type="button"
          onClick={() => onDeleteNode(n.id)}
          data-testid="flow-node-delete"
          className="flex items-center gap-1 rounded border border-[var(--color-danger)]/50 px-2 py-1 text-[var(--color-danger)] hover:bg-[var(--color-danger)]/10"
        >
          <Trash2 size={12} /> {t('inspector.deleteNode')}
        </button>
      )}
    </div>
  )
}

export type { FlowNodeType }
