// Hooks settings panel (Phase P4). Lists workspace PreToolUse/PostToolUse hooks
// and lets the user add/edit/toggle/delete them. Self-contained (own load/save),
// exempt from the global Save bar — like WorkspaceFilesPanel.
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Webhook, Trash2, Pencil, Plus, Lock } from 'lucide-react'
import { api } from '@/api'
import type { Hook, HookEvent, BuiltinHook } from '@/types'
import type { HookInput } from '@/api/hooks'
import { Field, Toggle, inputCls } from './primitives'
import { Button, LoadingState, toast } from '@/shared/components'
import { InfoPopover } from '@/shared/components/InfoPopover'

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
  const { t } = useTranslation('settingsMain')
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
      onError(t('hooks.commandRequired'))
      return
    }
    setBusy(true)
    try {
      if (editing) await api.updateHook(editing, draft)
      else await api.createHook(draft)
      setEditing(null)
      await load()
      toast.success(editing ? t('hooks.updated') : t('hooks.added'))
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
    // Destructive and irreversible (the command is not stored anywhere else) —
    // same confirm pattern the other panels use before a permanent delete.
    if (!confirm(t('hooks.deleteConfirm'))) return
    try {
      await api.deleteHook(h.id)
      await load()
      toast.success(t('hooks.deleted'))
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const set = <K extends keyof HookInput>(k: K, v: HookInput[K]) =>
    setDraft((d) => ({ ...d, [k]: v }))

  return (
    <div className="space-y-4">
      <p className="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
        <Webhook size={15} /> {t('hooks.title')}
        <InfoPopover text={t('hooks.description')} />
      </p>

      <div
        role="alert"
        data-testid="hooks-codex-not-applied-notice"
        className="rounded-lg border border-[color-mix(in_srgb,var(--color-warning)_45%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_14%,var(--color-surface))] p-4 text-sm text-[var(--color-text-dim)]"
      >
        <p className="font-medium text-[var(--color-text)]">⚠️ {t('hooks.codexWarningTitle')}</p>
        <p className="mt-1">{t('hooks.codexWarningBody')}</p>
      </div>

      {loading ? (
        <LoadingState label={t('shared.loading')} />
      ) : (
        <>
          <div className="flex flex-col gap-2">
            {hooks.length === 0 && (
              <div className="text-sm text-[var(--color-text-dim)]">{t('hooks.empty')}</div>
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
                    {t('hooks.matcher')}: <code>{h.matcher || t('hooks.allTools')}</code> ·{' '}
                    {h.timeoutSec || 30}
                    {t('shared.secondsShort')}
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
                  {h.enabled ? t('shared.active') : t('shared.inactive')}
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
              <Plus size={15} /> {t('hooks.add')}
            </Button>
          ) : (
            <div className="space-y-3 rounded-lg border border-[var(--color-accent)] bg-[var(--color-surface)] p-4">
              <div className="text-sm font-semibold">
                {editing ? t('hooks.edit') : t('hooks.new')}
              </div>
              <Field label={t('hooks.event')}>
                <select
                  data-testid="hook-event-select"
                  data-hook-id={editing}
                  value={draft.event}
                  onChange={(e) => set('event', e.target.value as HookEvent)}
                  className={inputCls}
                >
                  <optgroup label={t('hooks.toolEvents')}>
                    <option value="PreToolUse">{t('hooks.options.preToolUse')}</option>
                    <option value="PostToolUse">{t('hooks.options.postToolUse')}</option>
                  </optgroup>
                  <optgroup label={t('hooks.lifecycleEvents')}>
                    <option value="UserPromptSubmit">{t('hooks.options.userPromptSubmit')}</option>
                    <option value="SessionStart">{t('hooks.options.sessionStart')}</option>
                    <option value="Stop">{t('hooks.options.stop')}</option>
                    <option value="SubagentStop">{t('hooks.options.subagentStop')}</option>
                    <option value="PreCompact">{t('hooks.options.preCompact')}</option>
                    <option value="Notification">{t('hooks.options.notification')}</option>
                    <option value="SessionEnd">{t('hooks.options.sessionEnd')}</option>
                  </optgroup>
                </select>
              </Field>
              <Field label={t('hooks.matcher')} hint={t('hooks.matcherHint')}>
                <input
                  data-testid="hook-matcher-input"
                  data-hook-id={editing}
                  value={draft.matcher}
                  onChange={(e) => set('matcher', e.target.value)}
                  className={inputCls}
                  placeholder="*"
                />
              </Field>
              <Field label={t('hooks.command')} hint={t('hooks.commandHint')}>
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
              <Field label={t('hooks.timeout')} hint={t('hooks.timeoutHint')}>
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
              <Toggle
                label={t('shared.active')}
                checked={draft.enabled}
                onChange={(v) => set('enabled', v)}
              />
              <div className="flex gap-2">
                <Button
                  data-testid="hook-save"
                  data-hook-id={editing}
                  onClick={save}
                  disabled={busy}
                >
                  {busy ? t('shared.saving') : t('shared.save')}
                </Button>
                <Button variant="secondary" onClick={cancel}>
                  {t('shared.cancel')}
                </Button>
              </div>
            </div>
          )}
        </>
      )}

      {builtins.length > 0 && (
        <div className="space-y-2">
          <p className="flex items-center gap-2 pt-2 text-sm font-medium text-[var(--color-text)]">
            <Lock size={14} /> {t('hooks.builtins.title')}
            <InfoPopover text={t('hooks.builtins.description')} />
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
                  {b.description && <InfoPopover text={b.description} />}
                  <span className="font-mono text-[10px] text-[var(--color-text-dim)]">
                    {b.event}
                  </span>
                </div>
                {b.setting && (
                  <div className="mt-0.5 text-[10px] text-[var(--color-text-dim)]">
                    {t('hooks.builtins.setting')}: <code>{b.setting}</code>
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
                {b.enabled ? t('shared.active') : t('shared.inactive')}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
