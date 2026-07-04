import { useState } from 'react'
import { Info } from 'lucide-react'
import type { Agent, BranchMatchMode, FlowNode } from '../../types'
import { AgentPicker } from '../agents/AgentPicker'
import { PromptEditor } from '../common'
import { chromeFor } from './nodeStyles'

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

// FlowVarsButton is an ℹ️ popover listing the template placeholders usable in a
// node's prompt/template, mirroring the Automations prompt-vars helper. Static
// entries ({{input}}, {{last}}) plus one {{node.<id>}} per OTHER node in the flow.
// Clicking a row appends the placeholder to the target field.
function FlowVarsButton({
  nodeRefs,
  onInsert,
}: {
  nodeRefs: { id: string; title: string }[]
  onInsert: (text: string) => void
}) {
  const [open, setOpen] = useState(false)
  const statics: { name: string; desc: string }[] = [
    { name: '{{input}}', desc: 'Akışın girdisi (RunFlow input)' },
    { name: '{{last}}', desc: 'En son çalışan node’un çıktısı' },
  ]
  return (
    <span className="relative inline-flex">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className={`rounded p-0.5 transition hover:text-[var(--color-accent)] ${open ? 'text-[var(--color-accent)]' : 'text-[var(--color-text-dim)]'}`}
        title="Kullanılabilir değişkenler"
        aria-label="Kullanılabilir değişkenler"
      >
        <Info size={13} />
      </button>
      {open && (
        <>
          {/* Click-away backdrop closes the popover. */}
          <div className="fixed inset-0 z-10" onClick={() => setOpen(false)} />
          <div className="absolute bottom-full left-0 z-20 mb-1 w-[340px] max-w-[90vw] rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-2 shadow-lg">
            <div className="mb-1 px-1 text-[11px] font-semibold text-[var(--color-text-dim)]">
              Node’lar arası değişkenler (tıkla → ekle)
            </div>
            <div className="max-h-64 overflow-y-auto">
              {statics.map((v) => (
                <button
                  key={v.name}
                  type="button"
                  onClick={() => {
                    onInsert(v.name)
                    setOpen(false)
                  }}
                  className="flex w-full items-baseline gap-2 rounded px-1.5 py-1 text-left transition hover:bg-[var(--color-surface-2)]"
                  title="Alana ekle"
                >
                  <code className="shrink-0 rounded bg-[var(--color-accent-soft)] px-1 py-0.5 font-mono text-[11px] text-[var(--color-accent)]">
                    {v.name}
                  </code>
                  <span className="text-[11px] text-[var(--color-text-dim)]">{v.desc}</span>
                </button>
              ))}
              {nodeRefs.length > 0 && (
                <div className="mt-1 border-t border-[var(--color-border)] pt-1">
                  <div className="mb-0.5 px-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
                    Belirli node çıktısı
                  </div>
                  {nodeRefs.map((n) => {
                    const name = `{{node.${n.id}}}`
                    return (
                      <button
                        key={n.id}
                        type="button"
                        onClick={() => {
                          onInsert(name)
                          setOpen(false)
                        }}
                        className="flex w-full items-baseline gap-2 rounded px-1.5 py-1 text-left transition hover:bg-[var(--color-surface-2)]"
                        title="Alana ekle"
                      >
                        <code className="shrink-0 rounded bg-[var(--color-accent-soft)] px-1 py-0.5 font-mono text-[11px] text-[var(--color-accent)]">
                          {name}
                        </code>
                        <span className="truncate text-[11px] text-[var(--color-text-dim)]">{n.title || n.id}</span>
                      </button>
                    )
                  })}
                </div>
              )}
            </div>
          </div>
        </>
      )}
    </span>
  )
}

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
            />
          </div>
          <div className="block">
            <div className="mb-1 flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
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
          <div className="mb-1 flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
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
