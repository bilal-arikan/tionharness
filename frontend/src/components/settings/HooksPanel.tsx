// Hooks settings panel (Phase P4). Lists workspace PreToolUse/PostToolUse hooks
// and lets the user add/edit/toggle/delete them. Self-contained (own load/save),
// exempt from the global Save bar — like WorkspaceFilesPanel.
import { useEffect, useState } from 'react'
import { Webhook, Trash2, Pencil, Plus, ScanSearch } from 'lucide-react'
import { api } from '../../api'
import { systemApi } from '../../api/system'
import type { Hook, HookEvent, ExternalToolStatus } from '../../types'
import type { HookInput } from '../../api/hooks'
import { Field, Toggle, inputCls } from './primitives'
import { displayPath } from '../../lib/paths'

interface Props {
  onError: (msg: string) => void
}

const EMPTY: HookInput = {
  event: 'PreToolUse',
  matcher: '',
  command: '',
  timeoutSec: 30,
  enabled: true,
}

export function HooksPanel({ onError }: Props) {
  const [hooks, setHooks] = useState<Hook[]>([])
  const [loading, setLoading] = useState(true)
  // editing: null = no form open; '' = creating; otherwise the hook id being edited.
  const [editing, setEditing] = useState<string | null>(null)
  const [draft, setDraft] = useState<HookInput>(EMPTY)
  const [busy, setBusy] = useState(false)

  // External token-tool detector: checks whether optional CLI tools used by hook
  // commands (e.g. `sqz`) are present on PATH. Read-only — nothing is installed.
  const [tools, setTools] = useState<ExternalToolStatus[] | null>(null)
  const [checking, setChecking] = useState(false)
  const [toolsErr, setToolsErr] = useState<string | null>(null)

  const checkTools = async () => {
    setChecking(true)
    setToolsErr(null)
    try {
      setTools(await systemApi.externalTools())
    } catch (e) {
      setToolsErr(e instanceof Error ? e.message : String(e))
    } finally {
      setChecking(false)
    }
  }

  const load = () =>
    api
      .listHooks()
      .then(setHooks)
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoading(false))

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const startCreate = () => {
    setDraft(EMPTY)
    setEditing('')
  }
  const startEdit = (h: Hook) => {
    setDraft({ event: h.event, matcher: h.matcher, command: h.command, timeoutSec: h.timeoutSec, enabled: h.enabled })
    setEditing(h.id)
  }
  const cancel = () => setEditing(null)

  const save = async () => {
    if (!draft.command.trim()) {
      onError('Komut zorunludur')
      return
    }
    setBusy(true)
    try {
      if (editing) await api.updateHook(editing, draft)
      else await api.createHook(draft)
      setEditing(null)
      await load()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const toggle = async (h: Hook) => {
    try {
      await api.toggleHook(h.id, !h.enabled)
      await load()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const remove = async (h: Hook) => {
    try {
      await api.deleteHook(h.id)
      await load()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const set = <K extends keyof HookInput>(k: K, v: HookInput[K]) => setDraft((d) => ({ ...d, [k]: v }))

  return (
    <div className="space-y-4">
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-sm text-[var(--color-text-dim)]">
        <p className="mb-1 flex items-center gap-2 font-medium text-[var(--color-text)]">
          <Webhook size={15} /> Araç kancaları (PreToolUse / PostToolUse)
        </p>
        <p>
          Kancalar, native (anthropic/minimax) araç döngüsünde her araç çağrısının etrafında bir dış
          komut çalıştırır. <strong>PreToolUse</strong> girdiyi değiştirebilir, çağrıyı onaylayabilir
          veya engelleyebilir; <strong>PostToolUse</strong> çıktıyı dönüştürebilir (ör. sıkıştırma) ya
          da bağlam ekleyebilir. Komut, JSON'u stdin'den alır, JSON'u stdout'a döner; <code>exit 2</code>{' '}
          engelle demektir (Claude Code sözleşmesi). <em>claude-cli ajanları kendi hook'larını{' '}
          <code>~/.claude/settings.json</code>'dan okur — bunlar oraya değil SwarmGo'ya kayıtlıdır.</em>
        </p>
      </div>

      {loading ? (
        <div className="text-sm text-[var(--color-text-dim)]">Yükleniyor…</div>
      ) : (
        <>
          <div className="flex flex-col gap-2">
            {hooks.length === 0 && (
              <div className="text-sm text-[var(--color-text-dim)]">Henüz hook yok.</div>
            )}
            {hooks.map((h) => (
              <div
                key={h.id}
                className="flex items-center gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2"
              >
                <span
                  className={`rounded px-1.5 py-0.5 font-mono text-[10px] ${
                    h.event === 'PreToolUse'
                      ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                      : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
                  }`}
                >
                  {h.event}
                </span>
                <div className="min-w-0 flex-1">
                  <div className="truncate font-mono text-xs">{h.command}</div>
                  <div className="text-[11px] text-[var(--color-text-dim)]">
                    eşleşme: <code>{h.matcher || '* (tüm araçlar)'}</code> · {h.timeoutSec || 30}s
                  </div>
                </div>
                <button
                  onClick={() => toggle(h)}
                  className={`rounded px-2 py-0.5 text-xs ${
                    h.enabled
                      ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                      : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
                  }`}
                >
                  {h.enabled ? 'Aktif' : 'Pasif'}
                </button>
                <button onClick={() => startEdit(h)} className="text-[var(--color-text-dim)] hover:text-[var(--color-text)]">
                  <Pencil size={15} />
                </button>
                <button onClick={() => remove(h)} className="text-[var(--color-text-dim)] hover:text-[var(--color-danger)]">
                  <Trash2 size={15} />
                </button>
              </div>
            ))}
          </div>

          {editing === null ? (
            <button
              onClick={startCreate}
              className="flex items-center gap-1.5 rounded bg-[var(--color-accent)] px-3 py-1.5 text-sm font-medium text-white hover:opacity-90"
            >
              <Plus size={15} /> Hook ekle
            </button>
          ) : (
            <div className="space-y-3 rounded-lg border border-[var(--color-accent)] bg-[var(--color-surface)] p-4">
              <div className="text-sm font-semibold">{editing ? 'Hook düzenle' : 'Yeni hook'}</div>
              <Field label="Olay">
                <select
                  value={draft.event}
                  onChange={(e) => set('event', e.target.value as HookEvent)}
                  className={inputCls}
                >
                  <option value="PreToolUse">PreToolUse (çağrı öncesi)</option>
                  <option value="PostToolUse">PostToolUse (çağrı sonrası)</option>
                </select>
              </Field>
              <Field label="Eşleşme (araç adı glob)" hint="Boş = tüm araçlar. Örn: shell, write_file, http_*">
                <input value={draft.matcher} onChange={(e) => set('matcher', e.target.value)} className={inputCls} placeholder="*" />
              </Field>
              <Field label="Komut" hint="Shell komutu (Windows: PowerShell). JSON stdin alır, JSON stdout döner.">
                <textarea
                  value={draft.command}
                  onChange={(e) => set('command', e.target.value)}
                  rows={3}
                  className={`${inputCls} font-mono`}
                  placeholder="sqz hook"
                />
              </Field>
              <Field label="Zaman aşımı (sn)" hint="1–120 arası.">
                <input
                  type="number"
                  value={draft.timeoutSec}
                  onChange={(e) => set('timeoutSec', Number(e.target.value))}
                  className={inputCls}
                  min={1}
                  max={120}
                />
              </Field>
              <Toggle label="Aktif" checked={draft.enabled} onChange={(v) => set('enabled', v)} />
              <div className="flex gap-2">
                <button
                  onClick={save}
                  disabled={busy}
                  className="rounded bg-[var(--color-accent)] px-4 py-1.5 text-sm font-medium text-white hover:opacity-90 disabled:opacity-30"
                >
                  {busy ? 'Kaydediliyor…' : 'Kaydet'}
                </button>
                <button onClick={cancel} className="rounded border border-[var(--color-border)] px-4 py-1.5 text-sm">
                  İptal
                </button>
              </div>
            </div>
          )}
        </>
      )}

      <div className="space-y-3 border-t border-[var(--color-border)] pt-4">
        <p className="flex items-center gap-1.5 text-sm font-medium text-[var(--color-text)]">
          <ScanSearch size={14} className="text-[var(--color-accent)]" /> Harici token araçları
        </p>
        <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
          Hook komutlarında kullanılabilecek isteğe bağlı token-optimizasyon araçlarının (ör. <code>sqz</code>) bu cihazda{' '}
          <span className="font-medium text-[var(--color-text)]">kurulu olup olmadığını</span> kontrol eder.
          Yalnız PATH'te aranır — araçlar <span className="font-medium text-[var(--color-text)]">kurulmaz, çalıştırılmaz, değiştirilmez</span>.
        </div>
        <button
          type="button"
          onClick={checkTools}
          disabled={checking}
          className="inline-flex w-fit items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition hover:border-[var(--color-accent)] disabled:opacity-50"
        >
          <ScanSearch size={14} className="text-[var(--color-accent)]" />
          {checking ? 'Kontrol ediliyor…' : 'Kurulu mu kontrol et'}
        </button>
        {toolsErr && <p className="text-xs text-[var(--color-warning)]">{toolsErr}</p>}
        {tools && (
          <div className="flex flex-col gap-1.5">
            {tools.map((t) => (
              <div key={t.name} className="flex items-center justify-between gap-3 rounded-lg border border-[var(--color-border)] px-3 py-2">
                <div className="min-w-0">
                  <div className="flex items-center gap-2 text-sm">
                    <code className="rounded bg-[var(--color-surface-2)] px-1 font-medium">{t.name}</code>
                    {t.found ? (
                      <span className="text-[var(--color-success)]">✓ kurulu</span>
                    ) : (
                      <span className="text-[var(--color-text-dim)]">— bulunamadı</span>
                    )}
                  </div>
                  <div className="truncate text-xs text-[var(--color-text-dim)]">{t.found ? displayPath(t.path ?? '') : t.desc}</div>
                </div>
                <a href={t.url} target="_blank" rel="noreferrer" className="shrink-0 text-xs text-[var(--color-accent)] hover:underline">repo ↗</a>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
