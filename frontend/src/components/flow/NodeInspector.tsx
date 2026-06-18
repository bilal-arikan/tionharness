import type { Agent, FlowNode, FlowNodeType } from '../../types'

interface Props {
  node: FlowNode
  agents: Agent[]
  isStart: boolean
  onPatch: (patch: Partial<FlowNode>) => void
  onMakeStart: () => void
  onDelete: () => void
}

const TYPES: { value: FlowNodeType; label: string }[] = [
  { value: 'agent', label: 'Ajan' },
  { value: 'branch', label: 'Dallanma' },
  { value: 'parallel', label: 'Paralel' },
]

const input =
  'w-full rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs outline-none'

// NodeInspector edits the currently selected node. Routing targets are managed
// by drawing edges on the canvas; here we edit a node's intrinsic fields
// (type, title, agent, prompt, branch conditions). Edge re-targeting on the
// canvas stays the single source of truth for `next`/`parallel`/`joinNext`.
export function NodeInspector({ node, agents, isStart, onPatch, onMakeStart, onDelete }: Props) {
  return (
    <div className="space-y-3 text-sm">
      <div className="flex items-center justify-between">
        <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs">
          {node.id}
          {isStart && ' ▶'}
        </span>
        <div className="flex gap-2">
          {!isStart && (
            <button onClick={onMakeStart} className="text-xs text-[var(--color-accent)]">
              Başlangıç yap
            </button>
          )}
          <button onClick={onDelete} className="text-xs text-[var(--color-danger)]">
            Sil
          </button>
        </div>
      </div>

      <label className="block">
        <span className="mb-1 block text-xs text-[var(--color-text-dim)]">Tür</span>
        <select
          value={node.type}
          onChange={(e) => onPatch({ type: e.target.value as FlowNodeType })}
          className={input}
        >
          {TYPES.map((t) => (
            <option key={t.value} value={t.value}>
              {t.label}
            </option>
          ))}
        </select>
      </label>

      <label className="block">
        <span className="mb-1 block text-xs text-[var(--color-text-dim)]">Başlık</span>
        <input
          value={node.title ?? ''}
          onChange={(e) => onPatch({ title: e.target.value })}
          placeholder="başlık"
          className={input}
        />
      </label>

      {node.type === 'agent' && (
        <>
          <label className="block">
            <span className="mb-1 block text-xs text-[var(--color-text-dim)]">Ajan</span>
            <select
              value={node.agentId ?? ''}
              onChange={(e) => onPatch({ agentId: e.target.value })}
              className={input}
            >
              <option value="">— ajan seç —</option>
              {agents.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
          </label>
          <label className="block">
            <span className="mb-1 block text-xs text-[var(--color-text-dim)]">Prompt</span>
            <textarea
              value={node.prompt ?? ''}
              onChange={(e) => onPatch({ prompt: e.target.value })}
              placeholder="{{input}}, {{last}}, {{node.<id>}}"
              rows={4}
              className={input}
            />
          </label>
        </>
      )}

      {node.type === 'branch' && (
        <div className="space-y-2">
          <span className="block text-xs text-[var(--color-text-dim)]">
            Dallar (koşul; hedef için kenar çiz)
          </span>
          {(node.branches ?? []).map((b, bi) => (
            <div key={bi} className="flex items-center gap-2">
              <input
                value={b.contains}
                onChange={(e) => {
                  const branches = [...(node.branches ?? [])]
                  branches[bi] = { ...b, contains: e.target.value }
                  onPatch({ branches })
                }}
                placeholder="içeriyorsa… (boş = varsayılan)"
                className={input}
              />
              <button
                onClick={() => {
                  const branches = (node.branches ?? []).filter((_, i) => i !== bi)
                  onPatch({ branches })
                }}
                className="text-xs text-[var(--color-danger)]"
              >
                ✕
              </button>
            </div>
          ))}
          <button
            onClick={() =>
              onPatch({ branches: [...(node.branches ?? []), { contains: '', next: '' }] })
            }
            className="text-xs text-[var(--color-accent)]"
          >
            + dal ekle
          </button>
        </div>
      )}

      {node.type === 'parallel' && (
        <p className="text-xs text-[var(--color-text-dim)]">
          Eşzamanlı ajan node'larını alttaki <b>fan</b> tutamağından, join hedefini sağdaki{' '}
          <b>join</b> tutamağından kenar çizerek bağla.
        </p>
      )}
    </div>
  )
}
