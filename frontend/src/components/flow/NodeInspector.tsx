import type { Agent, BranchMatchMode, FlowNode } from '../../types'
import { AgentPicker } from '../agents/AgentPicker'
import { PromptEditor } from '../common'
import { chromeFor } from './nodeStyles'
import { FlowVarsButton } from './FlowVarsButton'

interface Props {
  node: FlowNode
  agents: Agent[]
  isStart: boolean
  // All nodes in the flow (for the {{node.<id>}} variable helper). Excludes nothing;
  // the helper filters out the current node itself.
  allNodes: FlowNode[]
  onPatch: (patch: Partial<FlowNode>) => void
  onMakeStart: () => void
  onDuplicate: () => void
  onDelete: () => void
}

const input =
  'w-full rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs outline-none'

// NodeInspector edits the currently selected node. A node's TYPE is fixed once
// created (it is chosen from the palette and never changes here) — the inspector
// only edits its intrinsic fields (title, agent, prompt, branch conditions).
// Routing (next/parallel/joinNext) is managed by drawing edges on the canvas.
export function NodeInspector({ node, agents, isStart, allNodes, onPatch, onMakeStart, onDuplicate, onDelete }: Props) {
  const chrome = chromeFor(node.type)
  // Other nodes, for the {{node.<id>}} variable helper (a node can't reference itself).
  const nodeRefs = allNodes
    .filter((n) => n.id !== node.id)
    .map((n) => ({ id: n.id, title: n.title ?? '' }))
  return (
    <div className="space-y-3 text-sm">
      <div className="flex items-center justify-between gap-2">
        <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs">
          {node.id}
          {isStart && ' ▶'}
        </span>
        <div className="flex flex-wrap gap-2">
          {!isStart && (
            <button onClick={onMakeStart} className="text-xs text-[var(--color-accent)]" title="Başlangıç yap">
              ▶ Başlangıç
            </button>
          )}
          <button onClick={onDuplicate} className="text-xs text-[var(--color-accent)]" title="Çoğalt">
            ⧉ Çoğalt
          </button>
          <button onClick={onDelete} className="text-xs text-[var(--color-danger)]" title="Sil">
            ✕ Sil
          </button>
        </div>
      </div>

      {/* Node type is fixed after creation — shown read-only with its monochrome
          type glyph (no type <select>; the type is chosen once from the palette). */}
      <div className="flex items-center gap-2">
        <span
          className="flex h-6 w-6 shrink-0 items-center justify-center rounded"
          style={{ background: chrome.accent, color: '#fff' }}
        >
          <chrome.Icon size={14} />
        </span>
        <span className="text-sm font-medium">{chrome.label}</span>
        <span className="text-[11px] text-[var(--color-text-dim)]">(tür sabit)</span>
      </div>

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
          <div className="block">
            <span className="mb-1 block text-xs text-[var(--color-text-dim)]">Ajan</span>
            <AgentPicker
              agents={agents}
              value={node.agentId ?? ''}
              onChange={(id) => onPatch({ agentId: id })}
              placeholder="— ajan seç —"
              clearable
            />
          </div>
          <div className="block">
            <div className="mb-1 flex flex-wrap items-center gap-1 text-xs text-[var(--color-text-dim)]">
              <span>Prompt</span>
              <FlowVarsButton
                nodeRefs={nodeRefs}
                onInsert={(t) => onPatch({ prompt: (node.prompt ?? '') + t })}
              />
            </div>
            <PromptEditor
              value={node.prompt ?? ''}
              onChange={(v) => onPatch({ prompt: v })}
              placeholder="{{input}}, {{last}}, {{node.<id>}}"
              rows={10}
              mono
              textareaClassName="min-h-48 text-xs"
            />
          </div>
        </>
      )}

      {node.type === 'branch' && (
        <div className="space-y-2">
          <label className="block">
            <span className="mb-1 block text-xs text-[var(--color-text-dim)]">Eşleşme</span>
            <select
              value={node.matchMode ?? 'contains'}
              onChange={(e) => onPatch({ matchMode: e.target.value as BranchMatchMode })}
              className={input}
            >
              <option value="contains">İçerir (substring)</option>
              <option value="equals">Eşittir (tam)</option>
              <option value="regex">Regex</option>
            </select>
          </label>
          <span className="block text-xs text-[var(--color-text-dim)]">
            Dallar (hedef için kenar çiz)
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
                placeholder={
                  (node.matchMode ?? 'contains') === 'equals'
                    ? 'eşittir… (boş = varsayılan)'
                    : (node.matchMode ?? 'contains') === 'regex'
                      ? 'regex… (boş = varsayılan)'
                      : 'içeriyorsa… (boş = varsayılan)'
                }
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

      {node.type === 'delay' && (
        <label className="block">
          <span className="mb-1 block text-xs text-[var(--color-text-dim)]">Bekleme (saniye)</span>
          <input
            type="number"
            min={0}
            step={0.5}
            value={(node.delayMs ?? 0) / 1000}
            onChange={(e) =>
              onPatch({ delayMs: Math.max(0, Math.round((Number(e.target.value) || 0) * 1000)) })
            }
            className={input}
          />
          <span className="mt-1 block text-[11px] text-[var(--color-text-dim)]">
            Bekledikten sonra sonraki node'a geçer (en çok 5 dk).
          </span>
        </label>
      )}

      {node.type === 'transform' && (
        <div className="block">
          <div className="mb-1 flex flex-wrap items-center gap-1 text-xs text-[var(--color-text-dim)]">
            <span>Şablon (çıktı)</span>
            <FlowVarsButton
              nodeRefs={nodeRefs}
              onInsert={(t) => onPatch({ template: (node.template ?? '') + t })}
            />
          </div>
          <PromptEditor
            value={node.template ?? ''}
            onChange={(v) => onPatch({ template: v })}
            placeholder="{{input}}, {{last}}, {{node.<id>}} — LLM çağırmadan çıktı üretir"
            rows={8}
            mono
            textareaClassName="min-h-32 text-xs"
          />
        </div>
      )}
    </div>
  )
}
