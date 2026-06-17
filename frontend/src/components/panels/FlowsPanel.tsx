import { useCallback, useEffect, useState } from 'react'
import { api } from '../../api'
import type { Agent, Flow, FlowNode, FlowNodeType, FlowRun, FlowState } from '../../types'

interface Props {
  agents: Agent[]
  onError: (msg: string) => void
}

const NODE_TYPES: { value: FlowNodeType; label: string }[] = [
  { value: 'agent', label: '🤖 Ajan' },
  { value: 'branch', label: '🔀 Dallanma' },
  { value: 'parallel', label: '⇉ Paralel' },
]

function newNodeId(existing: FlowNode[]): string {
  let i = 1
  while (existing.some((n) => n.id === `n${i}`)) i++
  return `n${i}`
}

// FlowsPanel is the visual protocol builder: pick a flow, edit its nodes
// (agent / branch / parallel), save, run with an input, and inspect the trace.
export function FlowsPanel({ agents, onError }: Props) {
  const [flows, setFlows] = useState<Flow[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)

  // Editor state for the selected flow.
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [start, setStart] = useState('')
  const [nodes, setNodes] = useState<FlowNode[]>([])

  // Run state.
  const [input, setInput] = useState('')
  const [running, setRunning] = useState(false)
  const [run, setRun] = useState<FlowRun | null>(null)

  const loadFlows = useCallback(() => {
    api.listFlows().then(setFlows).catch((e) => onError(e.message))
  }, [onError])

  useEffect(() => loadFlows(), [loadFlows])

  const selectFlow = useCallback(
    (f: Flow) => {
      setSelectedId(f.id)
      setName(f.name)
      setDescription(f.description)
      setRun(null)
      setInput('')
      try {
        const g = f.graph ? JSON.parse(f.graph) : { start: '', nodes: [] }
        setNodes(Array.isArray(g.nodes) ? g.nodes : [])
        setStart(g.start ?? '')
      } catch {
        setNodes([])
        setStart('')
      }
    },
    [],
  )

  const createFlow = async () => {
    const n = prompt('Akış adı:')
    if (!n) return
    try {
      const f = await api.createFlow(n)
      setFlows((prev) => [f, ...prev])
      selectFlow(f)
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const addNode = (type: FlowNodeType) => {
    const id = newNodeId(nodes)
    const node: FlowNode = { id, type, title: '' }
    if (type === 'agent') {
      node.agentId = agents[0]?.id ?? ''
      node.prompt = '{{input}}'
      node.next = ''
    } else if (type === 'branch') {
      node.branches = [{ contains: '', next: '' }]
    } else {
      node.parallel = []
      node.joinNext = ''
    }
    setNodes((prev) => [...prev, node])
    if (!start) setStart(id)
  }

  const patchNode = (idx: number, patch: Partial<FlowNode>) => {
    setNodes((prev) => prev.map((n, i) => (i === idx ? { ...n, ...patch } : n)))
  }

  const removeNode = (idx: number) => {
    setNodes((prev) => prev.filter((_, i) => i !== idx))
  }

  const saveFlow = async () => {
    if (!selectedId) return
    try {
      const f = await api.updateFlow(selectedId, name, description, { start, nodes })
      setFlows((prev) => prev.map((x) => (x.id === f.id ? f : x)))
      onError('') // clear
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const removeFlow = async (f: Flow) => {
    if (!confirm(`"${f.name}" akışı silinsin mi?`)) return
    try {
      await api.deleteFlow(f.id)
      if (selectedId === f.id) setSelectedId(null)
      loadFlows()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const doRun = async () => {
    if (!selectedId) return
    setRunning(true)
    setRun(null)
    try {
      await saveFlow() // persist edits before running
      const r = await api.runFlow(selectedId, input)
      setRun(r)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setRunning(false)
    }
  }

  const nodeOptions = (
    <>
      <option value="">(bitiş)</option>
      {nodes.map((n) => (
        <option key={n.id} value={n.id}>
          {n.id} · {n.title || n.type}
        </option>
      ))}
    </>
  )

  const trace: FlowState | null = run?.state ? safeParse(run.state) : null
  const agentNodes = nodes.filter((n) => n.type === 'agent')

  return (
    <div className="flex h-full">
      {/* Flow list */}
      <div className="w-56 flex-shrink-0 overflow-y-auto border-r border-[var(--color-border)] p-3">
        <button
          onClick={createFlow}
          className="mb-3 w-full rounded-lg bg-[var(--color-accent)] px-3 py-2 text-sm font-medium text-white hover:opacity-90"
        >
          + Yeni akış
        </button>
        <ul className="space-y-1">
          {flows.map((f) => (
            <li key={f.id}>
              <button
                onClick={() => selectFlow(f)}
                className={`flex w-full items-start justify-between rounded-lg px-3 py-2 text-left text-sm ${
                  selectedId === f.id
                    ? 'bg-[var(--color-surface-2)]'
                    : 'hover:bg-[var(--color-surface-2)]'
                }`}
              >
                <span className="min-w-0 flex-1">
                  <span className="block truncate">{f.name}</span>
                  {f.description && (
                    <span className="mt-0.5 block truncate text-xs text-[var(--color-text-dim)]">
                      {f.description}
                    </span>
                  )}
                </span>
                <span
                  onClick={(e) => {
                    e.stopPropagation()
                    removeFlow(f)
                  }}
                  className="ml-2 text-xs text-[var(--color-text-dim)] hover:text-red-400"
                >
                  ✕
                </span>
              </button>
            </li>
          ))}
          {flows.length === 0 && (
            <li className="text-sm text-[var(--color-text-dim)]">Henüz akış yok.</li>
          )}
        </ul>
      </div>

      {/* Editor + run */}
      <div className="flex-1 overflow-y-auto p-6">
        {!selectedId ? (
          <p className="text-sm text-[var(--color-text-dim)]">
            Soldan bir akış seçin veya yeni bir akış oluşturun.
          </p>
        ) : (
          <div className="mx-auto max-w-3xl space-y-5">
            {/* Meta */}
            <div className="flex items-center gap-2">
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                className="flex-1 rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm font-medium outline-none"
              />
              <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
                Başlangıç:
                <select
                  value={start}
                  onChange={(e) => setStart(e.target.value)}
                  className="rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs outline-none"
                >
                  <option value="">—</option>
                  {nodes.map((n) => (
                    <option key={n.id} value={n.id}>
                      {n.id}
                    </option>
                  ))}
                </select>
              </label>
              <button
                onClick={saveFlow}
                className="rounded-lg bg-[var(--color-accent)] px-3 py-2 text-sm font-medium text-white hover:opacity-90"
              >
                Kaydet
              </button>
            </div>

            {/* Description */}
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Açıklama — bu akış ne yapar? (isteğe bağlı)"
              rows={2}
              className="w-full rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm text-[var(--color-text-dim)] outline-none"
            />

            {/* Nodes */}
            <div className="space-y-3">
              {nodes.map((node, idx) => (
                <div
                  key={node.id}
                  className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
                >
                  <div className="mb-2 flex items-center gap-2">
                    <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs">
                      {node.id}
                      {start === node.id && ' ▶'}
                    </span>
                    <select
                      value={node.type}
                      onChange={(e) => patchNode(idx, { type: e.target.value as FlowNodeType })}
                      className="rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs outline-none"
                    >
                      {NODE_TYPES.map((t) => (
                        <option key={t.value} value={t.value}>
                          {t.label}
                        </option>
                      ))}
                    </select>
                    <input
                      value={node.title ?? ''}
                      onChange={(e) => patchNode(idx, { title: e.target.value })}
                      placeholder="başlık"
                      className="flex-1 rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs outline-none"
                    />
                    <button
                      onClick={() => removeNode(idx)}
                      className="px-1 text-xs text-red-400 hover:opacity-80"
                    >
                      Sil
                    </button>
                  </div>

                  {node.type === 'agent' && (
                    <div className="space-y-2">
                      <select
                        value={node.agentId ?? ''}
                        onChange={(e) => patchNode(idx, { agentId: e.target.value })}
                        className="w-full rounded bg-[var(--color-surface-2)] px-2 py-1 text-sm outline-none"
                      >
                        <option value="">— ajan seç —</option>
                        {agents.map((a) => (
                          <option key={a.id} value={a.id}>
                            {a.name}
                          </option>
                        ))}
                      </select>
                      <textarea
                        value={node.prompt ?? ''}
                        onChange={(e) => patchNode(idx, { prompt: e.target.value })}
                        placeholder="Prompt — {{input}}, {{last}}, {{node.<id>}} kullanılabilir"
                        rows={2}
                        className="w-full rounded bg-[var(--color-surface-2)] px-2 py-1 text-sm outline-none"
                      />
                      <label className="flex items-center gap-2 text-xs text-[var(--color-text-dim)]">
                        Sonraki:
                        <select
                          value={node.next ?? ''}
                          onChange={(e) => patchNode(idx, { next: e.target.value })}
                          className="rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs outline-none"
                        >
                          {nodeOptions}
                        </select>
                      </label>
                    </div>
                  )}

                  {node.type === 'branch' && (
                    <div className="space-y-2">
                      {(node.branches ?? []).map((b, bi) => (
                        <div key={bi} className="flex items-center gap-2">
                          <input
                            value={b.contains}
                            onChange={(e) => {
                              const branches = [...(node.branches ?? [])]
                              branches[bi] = { ...b, contains: e.target.value }
                              patchNode(idx, { branches })
                            }}
                            placeholder="içeriyorsa… (boş = varsayılan)"
                            className="flex-1 rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs outline-none"
                          />
                          <span className="text-xs text-[var(--color-text-dim)]">→</span>
                          <select
                            value={b.next}
                            onChange={(e) => {
                              const branches = [...(node.branches ?? [])]
                              branches[bi] = { ...b, next: e.target.value }
                              patchNode(idx, { branches })
                            }}
                            className="rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs outline-none"
                          >
                            {nodeOptions}
                          </select>
                          <button
                            onClick={() => {
                              const branches = (node.branches ?? []).filter((_, i) => i !== bi)
                              patchNode(idx, { branches })
                            }}
                            className="text-xs text-red-400"
                          >
                            ✕
                          </button>
                        </div>
                      ))}
                      <button
                        onClick={() =>
                          patchNode(idx, { branches: [...(node.branches ?? []), { contains: '', next: '' }] })
                        }
                        className="text-xs text-[var(--color-accent)]"
                      >
                        + dal ekle
                      </button>
                    </div>
                  )}

                  {node.type === 'parallel' && (
                    <div className="space-y-2">
                      <p className="text-xs text-[var(--color-text-dim)]">
                        Eşzamanlı çalışacak ajan node'ları:
                      </p>
                      <div className="flex flex-wrap gap-2">
                        {agentNodes
                          .filter((an) => an.id !== node.id)
                          .map((an) => {
                            const checked = (node.parallel ?? []).includes(an.id)
                            return (
                              <label key={an.id} className="flex items-center gap-1 text-xs">
                                <input
                                  type="checkbox"
                                  checked={checked}
                                  onChange={(e) => {
                                    const set = new Set(node.parallel ?? [])
                                    if (e.target.checked) set.add(an.id)
                                    else set.delete(an.id)
                                    patchNode(idx, { parallel: [...set] })
                                  }}
                                />
                                {an.id}
                              </label>
                            )
                          })}
                        {agentNodes.filter((an) => an.id !== node.id).length === 0 && (
                          <span className="text-xs text-[var(--color-text-dim)]">
                            Önce ajan node'ları ekleyin.
                          </span>
                        )}
                      </div>
                      <label className="flex items-center gap-2 text-xs text-[var(--color-text-dim)]">
                        Join sonrası:
                        <select
                          value={node.joinNext ?? ''}
                          onChange={(e) => patchNode(idx, { joinNext: e.target.value })}
                          className="rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs outline-none"
                        >
                          {nodeOptions}
                        </select>
                      </label>
                    </div>
                  )}
                </div>
              ))}
            </div>

            {/* Add node */}
            <div className="flex gap-2">
              {NODE_TYPES.map((t) => (
                <button
                  key={t.value}
                  onClick={() => addNode(t.value)}
                  className="rounded-lg bg-[var(--color-surface-2)] px-3 py-1.5 text-xs hover:opacity-90"
                >
                  + {t.label}
                </button>
              ))}
            </div>

            {/* Run */}
            <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
              <h3 className="mb-2 text-sm font-semibold">Çalıştır</h3>
              <textarea
                value={input}
                onChange={(e) => setInput(e.target.value)}
                placeholder="Girdi (akışa {{input}} olarak geçer)"
                rows={2}
                className="w-full rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
              />
              <button
                onClick={doRun}
                disabled={running}
                className="mt-2 rounded-lg bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
              >
                {running ? 'Çalışıyor…' : '▶ Çalıştır'}
              </button>

              {run && (
                <div className="mt-4 border-t border-[var(--color-border)] pt-3">
                  <div className="mb-2 text-xs">
                    Durum:{' '}
                    <span className={run.status === 'success' ? 'text-green-400' : 'text-red-400'}>
                      {run.status}
                    </span>
                    {run.error && <span className="ml-2 text-red-400">· {run.error}</span>}
                  </div>
                  <ol className="space-y-2">
                    {(trace?.trace ?? []).map((t, i) => (
                      <li key={i} className="rounded bg-[var(--color-surface-2)] p-2 text-sm">
                        <div className="mb-1 text-xs text-[var(--color-text-dim)]">
                          {i + 1}. [{t.type}] {t.title}
                        </div>
                        <div className="whitespace-pre-wrap">{t.output}</div>
                      </li>
                    ))}
                  </ol>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

function safeParse(s: string): FlowState | null {
  try {
    return JSON.parse(s) as FlowState
  } catch {
    return null
  }
}
