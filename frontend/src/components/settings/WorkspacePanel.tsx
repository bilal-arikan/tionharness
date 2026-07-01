// Per-workspace category: identity (icon), stats, instructions,
// provider/model overrides, autonomy pause and the delete danger zone. The
// publish/export-as-template flow now lives in its own "Dışa Aktar" sub-tab.
import type { WorkspaceSettings } from '../../types'
import { Field, Toggle, inputCls, type WsSet } from './primitives'
import { ProviderModelSelect } from '../agents/ProviderModelSelect'
import { EmojiField } from '../common/EmojiField'

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
          <EmojiField value={ws.icon} onChange={(e) => setWsField('icon', e)} clearLabel="⬡" />
        </Field>
        <div className="flex items-center gap-2 pb-1 text-sm">
          <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-[var(--color-surface-2)] text-lg">{ws.icon || '⬡'}</span>
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
      <div className="space-y-1">
        <span className="text-sm font-medium">Varsayılan sağlayıcı + model (bu workspace)</span>
        <ProviderModelSelect
          allowInherit
          provider={ws.defaultProvider}
          model={ws.defaultModel}
          onChange={(p, m) => {
            setWsField('defaultProvider', p)
            setWsField('defaultModel', m)
          }}
        />
        <span className="text-xs text-[var(--color-text-dim)]">Boş = uygulama varsayılanı.</span>
      </div>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
        📁 Proje dizini (path) ve git ayarları için yan menüdeki <b>Proje</b> sekmesine bak.
      </div>
      <Toggle label="Bu workspace'te otonomiyi duraklat" hint="Yalnızca bu workspace'in zamanlama çağrılarını bloklar." checked={ws.pauseAutonomy} onChange={(v) => setWsField('pauseAutonomy', v)} />

      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        Çapraz-session farkındalığı
      </div>
      <Toggle
        label="Session bağlamı"
        hint="Bu workspace'in ajanlarına aktif + son sessionlarının kısa özetini bağlama ekler ve list_sessions aracını sunar. Mevcut başlık/özet kullanılır (yeni LLM çağrısı yok)."
        checked={ws.sessionContextEnabled}
        onChange={(v) => setWsField('sessionContextEnabled', v)}
      />
      {ws.sessionContextEnabled && (
        <>
          <Toggle
            label="Her turda ver"
            hint="Açık: özet her turda güncellenir (token maliyeti). Kapalı: yalnızca session'ın ilk turunda verilir (önerilen)."
            checked={ws.sessionContextEveryTurn}
            onChange={(v) => setWsField('sessionContextEveryTurn', v)}
          />
          <Field label="Listelenecek geçmiş session sayısı" hint="Aktif olmayan, en son güncellenen N session (1–20).">
            <input type="number" min={1} max={20} value={ws.sessionContextRecentCount} onChange={(e) => setWsField('sessionContextRecentCount', Number(e.target.value))} className={inputCls} />
          </Field>
        </>
      )}

      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        Şablon
      </div>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2.5 text-xs text-[var(--color-text-dim)]">
        📦 Bu workspace'i (ajanlar, akışlar, zamanlamalar, skill'ler, talimatlar) bir şablon paketine
        dönüştürmek ve <span className="font-medium text-[var(--color-text)]">neyin dahil edileceğini seçmek</span> için
        yan menüdeki <span className="font-medium text-[var(--color-text)]">Dışa Aktar</span> sekmesine bak.
      </div>

      {onDeleteWorkspace && (
        <div className="mt-2 flex items-center justify-between rounded-lg border border-[color-mix(in_srgb,var(--color-danger)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_6%,transparent)] px-3 py-2">
          <span className="text-xs text-[var(--color-text-dim)]">Bu workspace'i ve tüm verisini kalıcı olarak sil.</span>
          <button
            onClick={onDeleteWorkspace}
            className="rounded border border-[color-mix(in_srgb,var(--color-danger)_40%,transparent)] px-3 py-1 text-xs text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)]"
          >
            Workspace'i sil
          </button>
        </div>
      )}
    </>
  )
}
