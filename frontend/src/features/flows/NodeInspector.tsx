import type { Agent, BranchMatchMode, Flow, FlowNode } from '@/types'
import { AgentPicker } from '@/shared/components/agents/AgentPicker'
import { PromptEditor } from '@/shared/components'
import { InfoPopover } from '@/shared/components/InfoPopover'
import {
  CoordinatorWorkflowPicker,
  WORKFLOW_HELP,
} from '@/shared/components/CoordinatorWorkflowPicker'
import { Workflow } from 'lucide-react'
import { NumberField } from '@/features/settings/primitives'
import { chromeFor, nodeHeaderForeground } from './nodeStyles'
import { FlowVarsButton } from './FlowVarsButton'

interface Props {
  node: FlowNode
  agents: Agent[]
  isStart: boolean
  // All nodes in the flow (for the {{node.<id>}} variable helper). Excludes nothing;
  // the helper filters out the current node itself.
  allNodes: FlowNode[]
  // Other flows in the workspace, for the subflow/spawn flow pickers. The current
  // flow is excluded by the caller to avoid trivial self-reference in the picker.
  flows: Flow[]
  onPatch: (patch: Partial<FlowNode>) => void
  onDuplicate: () => void
  onDelete: () => void
}

const input = 'w-full rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs outline-none'

// NodeInspector edits the currently selected node. A node's TYPE is fixed once
// created (it is chosen from the palette and never changes here) — the inspector
// only edits its intrinsic fields (title, agent, prompt, branch conditions).
// Routing (next/parallel/joinNext) is managed by drawing edges on the canvas.
export function NodeInspector({
  node,
  agents,
  isStart,
  allNodes,
  flows,
  onPatch,
  onDuplicate,
  onDelete,
}: Props) {
  const chrome = chromeFor(node.type)
  // Spawn nodes in this flow, for the join node's "which spawn to await" picker.
  const spawnNodes = allNodes.filter((n) => n.type === 'spawn')
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
          <button
            onClick={onDuplicate}
            className="text-xs text-[var(--color-accent)]"
            title="Çoğalt"
          >
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
          style={{ background: chrome.accent, color: nodeHeaderForeground(chrome.accent) }}
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
          <label className="flex items-center gap-2 text-xs">
            <input
              type="checkbox"
              checked={node.fresh ?? false}
              onChange={(e) => onPatch({ fresh: e.target.checked })}
            />
            <span>
              Taze bağlam{' '}
              <span className="text-[var(--color-text-dim)]">
                (birikmiş konuşmayı görmez — yalnız "Bağlamı biriktir" açıkken etkili)
              </span>
            </span>
          </label>
        </>
      )}

      {node.type === 'coordinator' && (
        <>
          <div className="block">
            <span className="mb-1 block text-xs text-[var(--color-text-dim)]">
              Koordinatör ajan
            </span>
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
              <span>Görev (koordinatöre verilen hedef)</span>
              <FlowVarsButton
                nodeRefs={nodeRefs}
                onInsert={(t) => onPatch({ prompt: (node.prompt ?? '') + t })}
              />
            </div>
            <PromptEditor
              value={node.prompt ?? ''}
              onChange={(v) => onPatch({ prompt: v })}
              placeholder="{{input}}, {{last}}, {{node.<id>}}"
              rows={8}
              mono
              textareaClassName="min-h-32 text-xs"
            />
          </div>
          {/* The same recipe list the session info panel offers, so a pattern
              picked there is selectable here (shared component). */}
          <div className="block">
            <div className="mb-1 flex flex-wrap items-center gap-1.5 text-xs text-[var(--color-text-dim)]">
              <Workflow size={12} />
              <span>Koordinasyon türü (workflow)</span>
              <InfoPopover text={WORKFLOW_HELP} label="Workflow nedir?" fixed />
            </div>
            <CoordinatorWorkflowPicker
              value={node.workflow}
              onChange={(slug) => onPatch({ workflow: slug })}
              groupName={`node-wf-${node.id}`}
            />
          </div>
          <NumberField
            label="En çok koordinatör turu"
            hint="0 = reçetenin kendi sınırı, yoksa workspace varsayılanı"
            min={0}
            step={1}
            value={node.maxTurns ?? 0}
            onChange={(v) => onPatch({ maxTurns: Math.round(v) })}
          />
          <NumberField
            label="Zaman aşımı (saniye)"
            hint="0 = 30 dk varsayılan"
            min={0}
            step={1}
            value={node.timeoutSec ?? 0}
            onChange={(v) => onPatch({ timeoutSec: Math.round(v) })}
          />
          <p className="text-[11px] text-[var(--color-text-dim)]">
            Ajan kendi koordinatör oturumunda çalışır ve{' '}
            <b>kaç worker açacağına anlık karar verir</b>. Düğüm, tüm workerlar bitip koordinatör
            susana kadar bloklar; son yanıtı çıktı olur. Zaman aşımında çalışan workerlar durdurulur
            ve akış hata verir.
          </p>
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

      {node.type === 'subflow' && (
        <div className="space-y-2">
          <label className="block">
            <span className="mb-1 block text-xs text-[var(--color-text-dim)]">Alt-akış</span>
            <select
              value={node.flowRef ?? ''}
              onChange={(e) => onPatch({ flowRef: e.target.value })}
              className={input}
            >
              <option value="">— akış seç —</option>
              {flows.map((f) => (
                <option key={f.id} value={f.id}>
                  {f.emoji ? `${f.emoji} ` : ''}
                  {f.name} ({f.id})
                </option>
              ))}
            </select>
          </label>
          <div className="block">
            <div className="mb-1 flex flex-wrap items-center gap-1 text-xs text-[var(--color-text-dim)]">
              <span>Girdi şablonu</span>
              <FlowVarsButton
                nodeRefs={nodeRefs}
                onInsert={(t) => onPatch({ template: (node.template ?? '') + t })}
              />
            </div>
            <PromptEditor
              value={node.template ?? ''}
              onChange={(v) => onPatch({ template: v })}
              placeholder="boş = {{last}} · alt-akışa geçilecek girdi"
              rows={3}
              mono
              textareaClassName="min-h-12 text-xs"
            />
          </div>
          <p className="text-[11px] text-[var(--color-text-dim)]">
            Alt-akış <b>await-input</b>'a düşerse bu akış da askıya alınır; girdi verilince alt-akış
            devam eder (propagasyon).
          </p>
        </div>
      )}

      {node.type === 'spawn' && (
        <div className="space-y-2">
          <div className="block">
            <span className="mb-1 block text-xs text-[var(--color-text-dim)]">
              Async akışlar (birden çok seç)
            </span>
            <div className="max-h-40 space-y-1 overflow-auto rounded bg-[var(--color-surface-2)] p-1.5">
              {flows.length === 0 && (
                <div className="text-[11px] text-[var(--color-text-dim)]">başka akış yok</div>
              )}
              {flows.map((f) => {
                const selected = (node.spawnFlows ?? []).includes(f.id)
                return (
                  <label
                    key={f.id}
                    className="flex cursor-pointer items-center gap-1.5 text-[11px]"
                  >
                    <input
                      type="checkbox"
                      checked={selected}
                      onChange={(e) => {
                        const cur = node.spawnFlows ?? []
                        onPatch({
                          spawnFlows: e.target.checked
                            ? [...cur, f.id]
                            : cur.filter((x) => x !== f.id),
                        })
                      }}
                    />
                    <span className="truncate">
                      {f.emoji ? `${f.emoji} ` : ''}
                      {f.name}
                    </span>
                  </label>
                )
              })}
            </div>
          </div>
          <div className="block">
            <div className="mb-1 flex flex-wrap items-center gap-1 text-xs text-[var(--color-text-dim)]">
              <span>Girdi şablonu (her çocuğa)</span>
              <FlowVarsButton
                nodeRefs={nodeRefs}
                onInsert={(t) => onPatch({ template: (node.template ?? '') + t })}
              />
            </div>
            <PromptEditor
              value={node.template ?? ''}
              onChange={(v) => onPatch({ template: v })}
              placeholder="boş = {{last}}"
              rows={2}
              mono
              textareaClassName="min-h-10 text-xs"
            />
          </div>
          <p className="text-[11px] text-[var(--color-text-dim)]">
            Akışları <b>bloklamadan</b> başlatır; sonuçları bir <b>join</b> toplar. Async çocuklar
            interaktif olmamalı (await-input'a düşen çocuk join'i başarısız yapar).
          </p>
        </div>
      )}

      {node.type === 'join' && (
        <div className="space-y-2">
          <label className="block">
            <span className="mb-1 block text-xs text-[var(--color-text-dim)]">
              Beklenecek spawn node
            </span>
            <select
              value={node.spawnRef ?? ''}
              onChange={(e) => onPatch({ spawnRef: e.target.value })}
              className={input}
            >
              <option value="">tümü (tüm bekleyen spawn'lar)</option>
              {spawnNodes.map((n) => (
                <option key={n.id} value={n.id}>
                  {n.title || n.id} ({n.id})
                </option>
              ))}
            </select>
          </label>
          <NumberField
            label="Zaman aşımı (saniye)"
            hint="0 = süresiz"
            min={0}
            step={1}
            value={node.joinTimeoutSec ?? 0}
            onChange={(v) => onPatch({ joinTimeoutSec: Math.round(v) })}
          />
          <label className="flex items-center gap-2 text-xs text-[var(--color-text-dim)]">
            <input
              type="checkbox"
              checked={!!node.joinPartial}
              onChange={(e) => onPatch({ joinPartial: e.target.checked })}
            />
            Kısmi mod (başarısız/bekleyen/zaman aşımına uğrayan çocuğu düşür, join'i düşürme)
          </label>
          <span className="block text-[11px] text-[var(--color-text-dim)]">
            Spawn edilen child run'ları bekler, çıktılarını birleştirip {'{{last}}'}'e koyar.
          </span>
        </div>
      )}

      {node.type === 'delay' && (
        <NumberField
          label="Bekleme (saniye)"
          hint="Bekledikten sonra sonraki node'a geçer (en çok 5 dk)."
          min={0}
          max={300}
          step={0.5}
          value={(node.delayMs ?? 0) / 1000}
          onChange={(v) => onPatch({ delayMs: Math.round(v * 1000) })}
        />
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

      {node.type === 'loop' && (
        <div className="space-y-2">
          <p className="text-xs text-[var(--color-text-dim)]">
            Gövdeyi (alttaki <b>gövde</b> tutamağı) yinele; bitince <b>çıkış</b> tutamağındaki
            node'a geç. Gövde node'ları <code>{'{{iteration}}'}</code> (0-tabanlı) kullanabilir.
          </p>
          <NumberField
            label="En çok iterasyon"
            hint={'0 = yalnız "çıkış koşulu"na göre biter (biri gerekli).'}
            min={0}
            step={1}
            value={node.maxIters ?? 0}
            onChange={(v) => onPatch({ maxIters: Math.round(v) })}
          />
          <label className="block">
            <span className="mb-1 block text-xs text-[var(--color-text-dim)]">
              Çıkış koşulu (eşleşince biter)
            </span>
            <input
              value={node.until ?? ''}
              onChange={(e) => onPatch({ until: e.target.value })}
              placeholder="boş = yalnız iterasyon sınırı"
              className={input}
            />
          </label>
          <label className="block">
            <span className="mb-1 block text-xs text-[var(--color-text-dim)]">Eşleşme</span>
            <select
              value={node.untilMode ?? 'contains'}
              onChange={(e) => onPatch({ untilMode: e.target.value as BranchMatchMode })}
              className={input}
            >
              <option value="contains">İçerir (substring)</option>
              <option value="equals">Eşittir (tam)</option>
              <option value="regex">Regex</option>
            </select>
          </label>
        </div>
      )}
    </div>
  )
}
