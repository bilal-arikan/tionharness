import { useEffect, useState } from 'react'
import { Repeat, RotateCcw, Trash2, Info, Pencil, X } from 'lucide-react'
import { api } from '../../api'
import type { Agent, Automation } from '../../types'
import { AgentPicker } from '../agents/AgentPicker'
import { AgentAvatar } from '../agents/AgentAvatar'
import { Button, TagEditor } from '../common'

// datetime-local <-> unix-seconds helpers (mirrors Schedules.tsx).
function unixToLocalInput(unix?: number): string {
  if (!unix) return ''
  const d = new Date(unix * 1000)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
function localInputToUnix(s: string): number {
  if (!s) return 0
  const ms = new Date(s).getTime()
  return isNaN(ms) ? 0 : Math.floor(ms / 1000)
}

// PromptTemplate placeholders (kept in sync with agent/automation.go turnVars).
const PROMPT_VARS: { name: string; desc: string }[] = [
  { name: '{{result}}', desc: 'Biten oturumun son yanıtı' },
  { name: '{{title}}', desc: 'Biten oturumun başlığı' },
  { name: '{{tag}}', desc: 'Tetikleyici etiket' },
  { name: '{{sessionId}}', desc: 'Biten oturumun ID’si' },
  { name: '{{iteration}}', desc: 'Bu ateşlemenin sıra no’su (1-tabanlı)' },
  { name: '{{maxIterations}}', desc: 'Üst sınır (0 → ∞)' },
  { name: '{{agent}}', desc: 'Sonucu üreten ajanın adı ({{agentName}} eşdeğer)' },
  { name: '{{prevPrompt}}', desc: 'Bir önceki turu tetikleyen kullanıcı promptu' },
  { name: '{{automation}}', desc: 'Otomasyonun adı' },
  { name: '{{date}}', desc: 'Geçerli tarih (2026-07-03)' },
  { name: '{{time}}', desc: 'Geçerli saat (03:00)' },
  { name: '{{datetime}}', desc: 'Tarih + saat' },
]

interface Props {
  agents: Agent[]
  onError: (msg: string) => void
}

function fmtTime(unix?: number): string {
  if (!unix) return '—'
  return new Date(unix * 1000).toLocaleString('tr-TR')
}

// Automations — tag-triggered loop rules. When a session carrying triggerTag
// finishes a turn, its final reply is rendered into promptTemplate and a new
// session is spawned for the target agent (which, tagged with triggerTag by
// default, re-triggers the rule → a bounded loop). Rendered as a distinct
// section inside the Schedules screen.
export function Automations({ agents, onError }: Props) {
  const [items, setItems] = useState<Automation[]>([])

  // Create form.
  const [showVars, setShowVars] = useState(false)
  const [name, setName] = useState('')
  const [triggerTag, setTriggerTag] = useState('')
  const [targetAgentId, setTargetAgentId] = useState('')
  const [promptTemplate, setPromptTemplate] = useState('Devam et. Önceki sonuç:\n{{result}}')
  const [maxIterations, setMaxIterations] = useState('50')
  const [cooldownSec, setCooldownSec] = useState('0')
  const [expiresAt, setExpiresAt] = useState('')

  // Inline edit state (one automation edited at a time).
  const [editId, setEditId] = useState<string | null>(null)
  const [edit, setEdit] = useState({
    name: '', triggerTag: '', targetAgentId: '', promptTemplate: '',
    maxIterations: '50', cooldownSec: '0', expiresAt: '',
  })
  const [showEditVars, setShowEditVars] = useState(false)

  const reload = () =>
    api.listAutomations().then(setItems).catch((e) => onError((e as Error).message))

  useEffect(() => {
    reload()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const agentName = (id: string) => agents.find((a) => a.id === id)?.name ?? '—'

  const create = async () => {
    if (!triggerTag.trim() || !targetAgentId || !promptTemplate.trim()) {
      onError('Tetikleyici etiket, hedef ajan ve prompt şablonu zorunlu')
      return
    }
    const expUnix = localInputToUnix(expiresAt)
    if (expUnix && expUnix <= Math.floor(Date.now() / 1000)) {
      onError('Son tarih gelecekte olmalı')
      return
    }
    try {
      const a = await api.createAutomation({
        name: name.trim(),
        triggerTag: triggerTag.trim(),
        targetAgentId,
        promptTemplate: promptTemplate.trim(),
        maxIterations: Number(maxIterations) || 0,
        cooldownSec: Number(cooldownSec) || 0,
        expiresAt: expUnix,
        enabled: true,
      })
      setItems((prev) => [a, ...prev])
      setName('')
      setTriggerTag('')
      setExpiresAt('')
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const startEdit = (a: Automation) => {
    setEditId(a.id)
    setShowEditVars(false)
    setEdit({
      name: a.name ?? '',
      triggerTag: a.triggerTag,
      targetAgentId: a.targetAgentId,
      promptTemplate: a.promptTemplate,
      maxIterations: String(a.maxIterations),
      cooldownSec: String(a.cooldownSec),
      expiresAt: unixToLocalInput(a.expiresAt),
    })
  }

  const cancelEdit = () => setEditId(null)

  const saveEdit = async (a: Automation) => {
    if (!edit.triggerTag.trim() || !edit.targetAgentId || !edit.promptTemplate.trim()) {
      onError('Tetikleyici etiket, hedef ajan ve prompt şablonu zorunlu')
      return
    }
    const expUnix = localInputToUnix(edit.expiresAt)
    if (expUnix && expUnix <= Math.floor(Date.now() / 1000)) {
      onError('Son tarih gelecekte olmalı')
      return
    }
    try {
      await api.updateAutomation(a.id, {
        name: edit.name.trim(),
        triggerTag: edit.triggerTag.trim(),
        targetAgentId: edit.targetAgentId,
        promptTemplate: edit.promptTemplate.trim(),
        maxIterations: Number(edit.maxIterations) || 0,
        cooldownSec: Number(edit.cooldownSec) || 0,
        expiresAt: expUnix,
      })
      setEditId(null)
      reload()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const toggle = async (a: Automation) => {
    setItems((prev) => prev.map((x) => (x.id === a.id ? { ...x, enabled: !x.enabled } : x)))
    try {
      await api.toggleAutomation(a.id, !a.enabled)
      reload() // toggling on resets the counter server-side; resync
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }

  const reset = async (a: Automation) => {
    try {
      await api.resetAutomation(a.id)
      reload()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const remove = async (a: Automation) => {
    if (!confirm('Otomasyon silinsin mi?')) return
    setItems((prev) => prev.filter((x) => x.id !== a.id))
    try {
      await api.deleteAutomation(a.id)
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }

  const setSpawnTags = async (a: Automation, spawnTags: string[]) => {
    setItems((prev) => prev.map((x) => (x.id === a.id ? { ...x, spawnTags } : x)))
    try {
      await api.updateAutomation(a.id, { spawnTags })
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }

  return (
    <div className="mt-6">
      <div className="mb-2 flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
        <Repeat size={15} className="text-violet-500" />
        Otomasyonlar (etiket tetikleyicili döngüler)
      </div>
      <p className="mb-3 text-xs text-[var(--color-text-dim)]">
        Bir <span className="font-mono">tetikleyici etiket</span> taşıyan oturum bir turu bitirdiğinde,
        son yanıtı prompt şablonuna işlenir ve hedef ajan için yeni bir oturum başlatılır. Yeni oturum
        aynı etiketi taşıdığından döngü kendiliğinden sürer (maks. iterasyon / bekleme / aç-kapa ile sınırlı).
      </p>

      {/* Create form */}
      <div className="mb-4 space-y-2 rounded-lg border border-l-4 border-[var(--color-border)] border-l-violet-500 bg-[var(--color-surface)] p-3">
        <div className="flex flex-wrap items-center gap-2">
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Ad (ops.)"
            className="w-40 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
          />
          <input
            value={triggerTag}
            onChange={(e) => setTriggerTag(e.target.value)}
            placeholder="tetikleyici etiket (ör. loop)"
            className="w-52 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 font-mono text-sm outline-none focus:border-[var(--color-accent)]"
          />
          <AgentPicker agents={agents} value={targetAgentId} onChange={setTargetAgentId} />
          <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
            Maks. iter.
            <input
              type="number"
              min={0}
              value={maxIterations}
              onChange={(e) => setMaxIterations(e.target.value)}
              className="w-16 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none"
              title="0 = sınırsız (dikkat: sonsuz döngü)"
            />
          </label>
          <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
            Bekleme (sn)
            <input
              type="number"
              min={0}
              value={cooldownSec}
              onChange={(e) => setCooldownSec(e.target.value)}
              className="w-16 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none"
              title="İki tetik arası minimum saniye"
            />
          </label>
          <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
            Son tarih (ops.)
            <input
              type="datetime-local"
              value={expiresAt}
              onChange={(e) => setExpiresAt(e.target.value)}
              className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
              title="Bu tarihten sonra otomasyon tetiklenmez (opsiyonel)"
            />
            {expiresAt && (
              <button
                type="button"
                onClick={() => setExpiresAt('')}
                className="text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
                title="Son tarihi temizle"
              >
                <X size={13} />
              </button>
            )}
          </label>
        </div>
        <div className="flex items-end gap-2">
          <div className="relative flex-1">
            <div className="mb-1 flex items-center gap-1 text-[11px] text-[var(--color-text-dim)]">
              <span>Prompt şablonu</span>
              <button
                type="button"
                onClick={() => setShowVars((v) => !v)}
                className={`rounded p-0.5 transition hover:text-[var(--color-accent)] ${showVars ? 'text-[var(--color-accent)]' : ''}`}
                title="Kullanılabilir değişkenler"
                aria-label="Kullanılabilir değişkenler"
              >
                <Info size={13} />
              </button>
            </div>
            {showVars && (
              <>
                {/* Click-away backdrop closes the popover. */}
                <div className="fixed inset-0 z-10" onClick={() => setShowVars(false)} />
                <div className="absolute bottom-full left-0 z-20 mb-1 w-[360px] max-w-[90vw] rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-2 shadow-lg">
                  <div className="mb-1 px-1 text-[11px] font-semibold text-[var(--color-text-dim)]">
                    Şablonda kullanılabilir değişkenler (tıkla → ekle)
                  </div>
                  <div className="max-h-64 overflow-y-auto">
                    {PROMPT_VARS.map((v) => (
                      <button
                        key={v.name}
                        type="button"
                        onClick={() => {
                          setPromptTemplate((p) => p + v.name)
                          setShowVars(false)
                        }}
                        className="flex w-full items-baseline gap-2 rounded px-1.5 py-1 text-left transition hover:bg-[var(--color-surface-2)]"
                        title="Şablona ekle"
                      >
                        <code className="shrink-0 rounded bg-[var(--color-accent-soft)] px-1 py-0.5 font-mono text-[11px] text-[var(--color-accent)]">
                          {v.name}
                        </code>
                        <span className="text-[11px] text-[var(--color-text-dim)]">{v.desc}</span>
                      </button>
                    ))}
                  </div>
                </div>
              </>
            )}
            <textarea
              value={promptTemplate}
              onChange={(e) => setPromptTemplate(e.target.value)}
              rows={2}
              placeholder="Prompt şablonu — ℹ️ ile değişkenleri gör. Örn: Devam et. Önceki sonuç:\n{{result}}"
              className="w-full resize-y rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
            />
          </div>
          <Button onClick={create}>+ Otomasyon</Button>
        </div>
      </div>

      {/* List */}
      <div className="space-y-2">
        {items.length === 0 && (
          <p className="text-sm text-[var(--color-text-dim)]">Henüz otomasyon yok.</p>
        )}
        {items.map((a) => {
          const maxed = a.maxIterations > 0 && a.iterationCount >= a.maxIterations
          const expired = a.expiresAt && a.expiresAt <= Math.floor(Date.now() / 1000)

          if (editId === a.id) {
            return (
              <div
                key={a.id}
                className="space-y-2 rounded-lg border border-l-4 border-[var(--color-accent)] border-l-violet-500 bg-[var(--color-surface)] p-3 text-sm"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <input
                    value={edit.name}
                    onChange={(e) => setEdit((s) => ({ ...s, name: e.target.value }))}
                    placeholder="Ad (ops.)"
                    className="w-40 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
                  />
                  <input
                    value={edit.triggerTag}
                    onChange={(e) => setEdit((s) => ({ ...s, triggerTag: e.target.value }))}
                    placeholder="tetikleyici etiket"
                    className="w-52 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 font-mono text-sm outline-none focus:border-[var(--color-accent)]"
                  />
                  <AgentPicker agents={agents} value={edit.targetAgentId} onChange={(v) => setEdit((s) => ({ ...s, targetAgentId: v }))} />
                  <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
                    Maks. iter.
                    <input type="number" min={0} value={edit.maxIterations}
                      onChange={(e) => setEdit((s) => ({ ...s, maxIterations: e.target.value }))}
                      className="w-16 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none" />
                  </label>
                  <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
                    Bekleme (sn)
                    <input type="number" min={0} value={edit.cooldownSec}
                      onChange={(e) => setEdit((s) => ({ ...s, cooldownSec: e.target.value }))}
                      className="w-16 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none" />
                  </label>
                  <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
                    Son tarih (ops.)
                    <input type="datetime-local" value={edit.expiresAt}
                      onChange={(e) => setEdit((s) => ({ ...s, expiresAt: e.target.value }))}
                      className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]" />
                    {edit.expiresAt && (
                      <button type="button" onClick={() => setEdit((s) => ({ ...s, expiresAt: '' }))}
                        className="text-[var(--color-text-dim)] hover:text-[var(--color-danger)]" title="Son tarihi temizle">
                        <X size={13} />
                      </button>
                    )}
                  </label>
                </div>
                <div className="relative">
                  <div className="mb-1 flex items-center gap-1 text-[11px] text-[var(--color-text-dim)]">
                    <span>Prompt şablonu</span>
                    <button type="button" onClick={() => setShowEditVars((v) => !v)}
                      className={`rounded p-0.5 transition hover:text-[var(--color-accent)] ${showEditVars ? 'text-[var(--color-accent)]' : ''}`}
                      title="Kullanılabilir değişkenler" aria-label="Kullanılabilir değişkenler">
                      <Info size={13} />
                    </button>
                  </div>
                  {showEditVars && (
                    <>
                      <div className="fixed inset-0 z-10" onClick={() => setShowEditVars(false)} />
                      <div className="absolute bottom-full left-0 z-20 mb-1 w-[360px] max-w-[90vw] rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-2 shadow-lg">
                        <div className="mb-1 px-1 text-[11px] font-semibold text-[var(--color-text-dim)]">Şablonda kullanılabilir değişkenler (tıkla → ekle)</div>
                        <div className="max-h-64 overflow-y-auto">
                          {PROMPT_VARS.map((v) => (
                            <button key={v.name} type="button"
                              onClick={() => { setEdit((s) => ({ ...s, promptTemplate: s.promptTemplate + v.name })); setShowEditVars(false) }}
                              className="flex w-full items-baseline gap-2 rounded px-1.5 py-1 text-left transition hover:bg-[var(--color-surface-2)]" title="Şablona ekle">
                              <code className="shrink-0 rounded bg-[var(--color-accent-soft)] px-1 py-0.5 font-mono text-[11px] text-[var(--color-accent)]">{v.name}</code>
                              <span className="text-[11px] text-[var(--color-text-dim)]">{v.desc}</span>
                            </button>
                          ))}
                        </div>
                      </div>
                    </>
                  )}
                  <textarea value={edit.promptTemplate}
                    onChange={(e) => setEdit((s) => ({ ...s, promptTemplate: e.target.value }))}
                    rows={2}
                    className="w-full resize-y rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]" />
                </div>
                <div className="flex items-center gap-2">
                  <Button onClick={() => saveEdit(a)}>Kaydet</Button>
                  <Button variant="secondary" onClick={cancelEdit}>İptal</Button>
                </div>
              </div>
            )
          }

          return (
            <div
              key={a.id}
              className="flex items-start gap-3 rounded-lg border border-l-4 border-[var(--color-border)] border-l-violet-500 bg-[var(--color-surface)] px-3 py-2 text-sm"
            >
              <button
                onClick={() => toggle(a)}
                className={`mt-1 h-4 w-8 flex-shrink-0 rounded-full transition ${
                  a.enabled ? 'bg-[var(--color-accent)]' : 'bg-[var(--color-border)]'
                }`}
                title={a.enabled ? 'Etkin' : 'Pasif'}
              >
                <span className={`block h-4 w-4 rounded-full bg-white transition ${a.enabled ? 'translate-x-4' : ''}`} />
              </button>
              {(() => {
                const owner = agents.find((x) => x.id === a.targetAgentId)
                return owner ? (
                  <AgentAvatar agent={owner} size={28} />
                ) : (
                  <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[var(--color-surface-2)] text-[10px] text-[var(--color-text-dim)]">?</span>
                )
              })()}
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium text-[var(--color-text)]">{a.name || '(adsız)'}</span>
                  <span className="rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--color-accent)]">
                    #{a.triggerTag}
                  </span>
                  <span className="text-xs text-[var(--color-text-dim)]">→ {agentName(a.targetAgentId)}</span>
                  <span className="ml-auto font-mono text-[10px] text-[var(--color-text-dim)] opacity-60">{a.id}</span>
                </div>
                <div className="mt-1 truncate text-xs text-[var(--color-text-dim)]" title={a.promptTemplate}>
                  {a.promptTemplate}
                </div>
                <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--color-text-dim)]">
                  <span className={maxed ? 'text-[var(--color-danger)]' : ''}>
                    İterasyon: {a.iterationCount}
                    {a.maxIterations > 0 ? ` / ${a.maxIterations}` : ' / ∞'}
                    {maxed && ' (limit doldu)'}
                  </span>
                  <span>Bekleme: {a.cooldownSec}s</span>
                  <span>Son çalışma: {fmtTime(a.lastFiredAt)}</span>
                  {a.expiresAt ? (
                    <span className={expired ? 'text-[var(--color-danger)]' : ''}>
                      Son tarih: {fmtTime(a.expiresAt)}{expired && ' (süresi doldu)'}
                    </span>
                  ) : null}
                  {a.lastError && <span className="text-[var(--color-danger)]">Hata: {a.lastError}</span>}
                </div>
                <div className="mt-1.5 flex items-center gap-2">
                  <span className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
                    Spawn etiketleri
                  </span>
                  <TagEditor
                    tags={a.spawnTags ?? [a.triggerTag]}
                    onChange={(tags) => setSpawnTags(a, tags)}
                    placeholder="loop kırmak için boş bırak"
                    className="flex-1 py-1"
                  />
                </div>
              </div>
              <div className="flex flex-col items-center gap-2">
                <button
                  onClick={() => startEdit(a)}
                  className="text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
                  title="Düzenle"
                >
                  <Pencil size={15} />
                </button>
                {maxed && (
                  <button
                    onClick={() => reset(a)}
                    className="text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
                    title="Sayacı sıfırla"
                  >
                    <RotateCcw size={15} />
                  </button>
                )}
                <button
                  onClick={() => remove(a)}
                  className="text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
                  title="Sil"
                >
                  <Trash2 size={15} />
                </button>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
