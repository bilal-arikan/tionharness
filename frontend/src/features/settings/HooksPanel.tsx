// Hooks settings panel (Phase P4). Lists workspace PreToolUse/PostToolUse hooks
// and lets the user add/edit/toggle/delete them. Self-contained (own load/save),
// exempt from the global Save bar — like WorkspaceFilesPanel.
import { useEffect, useState } from 'react'
import { Webhook, Trash2, Pencil, Plus, Lock } from 'lucide-react'
import { api } from '@/api'
import type { Hook, HookEvent, BuiltinHook } from '@/types'
import type { HookInput } from '@/api/hooks'
import { Field, Toggle, inputCls } from './primitives'
import { Button, LoadingState, toast } from '@/shared/components'

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
  const [builtins, setBuiltins] = useState<BuiltinHook[]>([])
  const [loading, setLoading] = useState(true)
  // editing: null = no form open; '' = creating; otherwise the hook id being edited.
  const [editing, setEditing] = useState<string | null>(null)
  const [draft, setDraft] = useState<HookInput>(EMPTY)
  const [busy, setBusy] = useState(false)

  const load = () =>
    api
      .listHooks()
      .then(setHooks)
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoading(false))

  useEffect(() => {
    api
      .listBuiltinHooks()
      .then(setBuiltins)
      .catch(() => {})
  }, [])

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const startCreate = () => {
    setDraft(EMPTY)
    setEditing('')
  }
  const startEdit = (h: Hook) => {
    setDraft({
      event: h.event,
      matcher: h.matcher,
      command: h.command,
      timeoutSec: h.timeoutSec,
      enabled: h.enabled,
    })
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
      toast.success(editing ? 'Hook güncellendi' : 'Hook eklendi')
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
      toast.success('Hook silindi')
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const set = <K extends keyof HookInput>(k: K, v: HookInput[K]) =>
    setDraft((d) => ({ ...d, [k]: v }))

  return (
    <div className="space-y-4">
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-sm text-[var(--color-text-dim)]">
        <p className="mb-1 flex items-center gap-2 font-medium text-[var(--color-text)]">
          <Webhook size={15} /> Araç kancaları (PreToolUse / PostToolUse)
        </p>
        <p>
          Kancalar, native (anthropic/minimax) araç döngüsünde her araç çağrısının etrafında bir dış
          komut çalıştırır. <strong>PreToolUse</strong> girdiyi değiştirebilir, çağrıyı
          onaylayabilir veya engelleyebilir; <strong>PostToolUse</strong> çıktıyı dönüştürebilir
          (ör. sıkıştırma) ya da bağlam ekleyebilir. Komut, JSON'u stdin'den alır, JSON'u stdout'a
          döner; <code>exit 2</code> engelle demektir (Claude Code sözleşmesi).{' '}
          <em>
            claude-cli ajanlarında bu hook'lar, "Hook'ları claude-cli'ye geçir" ayarı açıkken{' '}
            <code>--settings</code> ile CLI'nin kendi tool döngüsüne de uygulanır; ancak CLI
            hook'ları CLI'nin kendi shell'inde koşar (Windows PowerShell uyumsuzluğuna dikkat).
          </em>
        </p>
      </div>

      <div
        role="alert"
        data-testid="hooks-codex-not-applied-notice"
        className="rounded-lg border border-[color-mix(in_srgb,var(--color-warning)_45%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_14%,var(--color-surface))] p-4 text-sm text-[var(--color-text-dim)]"
      >
        <p className="font-medium text-[var(--color-text)]">
          ⚠️ codex-cli ajanlarında hook&apos;lar çalışmaz
        </p>
        <p className="mt-1">
          codex-cli kendi araç döngüsünü ayrı bir alt süreçte koşturur ve hook aktarımı sunmaz.
          Yukarıdaki PreToolUse/PostToolUse hook&apos;ların hiçbiri bu ajanlarda tetiklenmez — ne
          codex&apos;in kendi shell/apply_patch araçları için, ne de MCP köprüsü üzerinden çağrılan
          TionSwarm araçları için. Sonuç olarak <code>sqz</code> gibi PostToolUse token-optimizer
          sıkıştırması da codex-cli ajanlarında devre dışıdır.
        </p>
      </div>

      {loading ? (
        <LoadingState label="Yükleniyor…" />
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
                  data-testid="hook-toggle"
                  data-hook-id={h.id}
                  onClick={() => toggle(h)}
                  className={`rounded px-2 py-0.5 text-xs ${
                    h.enabled
                      ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                      : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
                  }`}
                >
                  {h.enabled ? 'Aktif' : 'Pasif'}
                </button>
                <button
                  onClick={() => startEdit(h)}
                  className="text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
                >
                  <Pencil size={15} />
                </button>
                <button
                  data-testid="hook-delete"
                  data-hook-id={h.id}
                  onClick={() => remove(h)}
                  className="text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
                >
                  <Trash2 size={15} />
                </button>
              </div>
            ))}
          </div>

          {editing === null ? (
            <Button data-testid="hook-create" onClick={startCreate}>
              <Plus size={15} /> Hook ekle
            </Button>
          ) : (
            <div className="space-y-3 rounded-lg border border-[var(--color-accent)] bg-[var(--color-surface)] p-4">
              <div className="text-sm font-semibold">{editing ? 'Hook düzenle' : 'Yeni hook'}</div>
              <Field label="Olay">
                <select
                  data-testid="hook-event-select"
                  data-hook-id={editing}
                  value={draft.event}
                  onChange={(e) => set('event', e.target.value as HookEvent)}
                  className={inputCls}
                >
                  <optgroup label="Araç (yalnız native döngü)">
                    <option value="PreToolUse">PreToolUse (çağrı öncesi)</option>
                    <option value="PostToolUse">PostToolUse (çağrı sonrası)</option>
                  </optgroup>
                  <optgroup label="Yaşam döngüsü (native + claude-cli)">
                    <option value="UserPromptSubmit">
                      UserPromptSubmit (prompt öncesi — bağlam ekle/engelle)
                    </option>
                    <option value="SessionStart">SessionStart (oturum ilk turu)</option>
                    <option value="Stop">Stop (ana ajan turu bitti)</option>
                    <option value="SubagentStop">SubagentStop (alt-ajan bitti)</option>
                    <option value="PreCompact">PreCompact (özetleme öncesi)</option>
                    <option value="Notification">Notification (bildirim)</option>
                    <option value="SessionEnd">SessionEnd (oturum silindi)</option>
                  </optgroup>
                </select>
              </Field>
              <Field
                label="Eşleşme"
                hint="Araç olayları: araç adı glob'u (boş = tümü, örn: Bash, Write, http_*). SessionStart: kaynak (startup|resume). PreCompact: tetik (manual|auto). Diğer yaşam-döngüsü olayları: boş bırakın."
              >
                <input
                  data-testid="hook-matcher-input"
                  data-hook-id={editing}
                  value={draft.matcher}
                  onChange={(e) => set('matcher', e.target.value)}
                  className={inputCls}
                  placeholder="*"
                />
              </Field>
              <Field
                label="Komut"
                hint="Shell komutu (Windows: PowerShell). JSON stdin alır, JSON stdout döner."
              >
                <textarea
                  data-testid="hook-command-input"
                  data-hook-id={editing}
                  value={draft.command}
                  onChange={(e) => set('command', e.target.value)}
                  rows={3}
                  className={`${inputCls} font-mono`}
                  placeholder="sqz hook"
                />
              </Field>
              <Field label="Zaman aşımı (sn)" hint="1–120 arası.">
                <input
                  data-testid="hook-timeout-input"
                  data-hook-id={editing}
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
                <Button
                  data-testid="hook-save"
                  data-hook-id={editing}
                  onClick={save}
                  disabled={busy}
                >
                  {busy ? 'Kaydediliyor…' : 'Kaydet'}
                </Button>
                <Button variant="secondary" onClick={cancel}>
                  İptal
                </Button>
              </div>
            </div>
          )}
        </>
      )}

      {builtins.length > 0 && (
        <div className="space-y-2">
          <p className="flex items-center gap-2 pt-2 text-sm font-medium text-[var(--color-text)]">
            <Lock size={14} /> Yerleşik davranışlar (salt-okunur)
          </p>
          <p className="text-[11px] text-[var(--color-text-dim)]">
            TionSwarm'nun araç döngüsünün etrafına otomatik enjekte ettiği kancalar.
            Düzenlenemezler; bazıları Ayarlar'daki ilgili anahtarla açılıp kapatılır.
          </p>
          {builtins.map((b) => (
            <div
              key={b.name}
              className="flex items-start gap-3 rounded-lg border border-dashed border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2"
            >
              <span className="mt-0.5 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--color-text-dim)]">
                {b.scope}
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="text-xs font-semibold text-[var(--color-text)]">{b.name}</span>
                  <span className="font-mono text-[10px] text-[var(--color-text-dim)]">
                    {b.event}
                  </span>
                </div>
                <div className="text-[11px] leading-snug text-[var(--color-text-dim)]">
                  {b.description}
                </div>
                {b.setting && (
                  <div className="mt-0.5 text-[10px] text-[var(--color-text-dim)]">
                    ayar: <code>{b.setting}</code>
                  </div>
                )}
              </div>
              <span
                className={`mt-0.5 rounded px-2 py-0.5 text-xs ${
                  b.enabled
                    ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                    : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
                }`}
              >
                {b.enabled ? 'Aktif' : 'Pasif'}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
