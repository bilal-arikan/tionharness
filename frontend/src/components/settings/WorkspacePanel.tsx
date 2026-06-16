// Per-workspace category: identity (icon/color), stats, instructions,
// provider/model overrides, autonomy pause and the delete danger zone.
import type { WorkspaceSettings } from '../../types'
import { Field, Toggle, inputCls, type WsSet } from './primitives'

interface Props {
  ws: WorkspaceSettings
  setWsField: WsSet
  onDeleteWorkspace?: () => void
}

export function WorkspacePanel({ ws, setWsField, onDeleteWorkspace }: Props) {
  return (
    <>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
        Bu ayarlar yalnızca <span className="font-medium text-[var(--color-text)]">{ws.name}</span> workspace'ine özeldir. Boş bırakılan sağlayıcı/model uygulama-geneli varsayılana düşer.
      </div>

      {/* Stats */}
      <div className="grid grid-cols-4 gap-2">
        {[
          { label: 'Ajan', value: ws.agentCount },
          { label: 'Oturum', value: ws.sessionCount },
          { label: 'Görev', value: ws.taskCount },
          { label: 'Oluşturma', value: new Date(ws.createdAt * 1000).toLocaleDateString('tr-TR') },
        ].map((s) => (
          <div key={s.label} className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-2 text-center">
            <div className="text-sm font-semibold">{s.value}</div>
            <div className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">{s.label}</div>
          </div>
        ))}
      </div>

      <div className="flex items-end gap-3">
        <Field label="İkon (emoji)">
          <input value={ws.icon} onChange={(e) => setWsField('icon', e.target.value)} placeholder="🧩" maxLength={4} className={`${inputCls} w-20 text-center text-lg`} />
        </Field>
        <Field label="Renk">
          <div className="flex items-center gap-2">
            <input type="color" value={ws.color || '#4f8cff'} onChange={(e) => setWsField('color', e.target.value)} className="h-9 w-12 cursor-pointer rounded border border-[var(--color-border)] bg-[var(--color-bg)]" />
            <input value={ws.color} onChange={(e) => setWsField('color', e.target.value)} placeholder="(varsayılan)" className={`${inputCls} w-28`} />
          </div>
        </Field>
        <div className="flex items-center gap-2 pb-1 text-sm">
          <span className="flex h-9 w-9 items-center justify-center rounded-lg text-lg" style={{ backgroundColor: (ws.color || '#1e3a66') + '33' }}>{ws.icon || '⬡'}</span>
          <span className="text-xs text-[var(--color-text-dim)]">önizleme</span>
        </div>
      </div>

      <Field label="Workspace adı"><input value={ws.name} onChange={(e) => setWsField('name', e.target.value)} className={inputCls} /></Field>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
        📝 Bu workspace'in <span className="font-medium text-[var(--color-text)]">talimatları</span> ve runtime
        <span className="font-medium text-[var(--color-text)]"> promptları</span> artık yan menüdeki
        <span className="font-medium text-[var(--color-text)]"> “Promptlar &amp; Dosyalar”</span> sekmesinden,
        düzenlenebilir dosyalar (<code className="rounded bg-[var(--color-bg)] px-1">config/</code>) olarak yönetilir.
      </div>
      <Field label="Varsayılan sağlayıcı (bu workspace)" hint="Boş = uygulama varsayılanı.">
        <select value={ws.defaultProvider} onChange={(e) => setWsField('defaultProvider', e.target.value)} className={inputCls}>
          <option value="">(uygulama varsayılanı)</option>
          <option value="claude-cli">claude-cli (abonelik)</option>
          <option value="anthropic">anthropic (API key)</option>
        </select>
      </Field>
      <Field label="Varsayılan model (bu workspace)" hint="Boş = uygulama varsayılanı."><input value={ws.defaultModel} onChange={(e) => setWsField('defaultModel', e.target.value)} placeholder="(uygulama varsayılanı)" className={inputCls} /></Field>
      <Toggle label="Bu workspace'te otonomiyi duraklat" hint="Yalnızca bu workspace'in heartbeat/zamanlama çağrılarını bloklar." checked={ws.pauseAutonomy} onChange={(v) => setWsField('pauseAutonomy', v)} />

      {onDeleteWorkspace && (
        <div className="mt-2 flex items-center justify-between rounded-lg border border-red-500/30 bg-red-500/5 px-3 py-2">
          <span className="text-xs text-[var(--color-text-dim)]">Bu workspace'i ve tüm verisini kalıcı olarak sil.</span>
          <button
            onClick={onDeleteWorkspace}
            className="rounded border border-red-500/40 px-3 py-1 text-xs text-red-400 hover:bg-red-500/10"
          >
            Workspace'i sil
          </button>
        </div>
      )}
    </>
  )
}
