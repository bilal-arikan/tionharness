// Per-workspace category: identity (icon), stats, instructions,
// provider/model overrides, autonomy pause and the delete danger zone. The
// publish/export-as-template flow now lives in its own "Dışa Aktar" sub-tab.
import type { WorkspaceSettings } from '@/types'
import { Field, Toggle, inputCls, type WsSet } from './primitives'
import { ProviderModelSelect } from '@/shared/components/agents/ProviderModelSelect'
import { EmojiField } from '@/shared/components/EmojiField'

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
        <Field label="Workspace adı"><input value={ws.name} onChange={(e) => setWsField('name', e.target.value)} className={inputCls} /></Field>
        <Field label="İkon (emoji)">
          <EmojiField value={ws.icon} onChange={(e) => setWsField('icon', e)} clearLabel="⬡" />
        </Field>
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

      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        Çapraz-session farkındalığı
      </div>
      <Toggle
        label="Session bağlamı"
        hint="Bu workspace'in ajanlarına son (geçmiş) sessionlarının kısa özetini bağlama ekler ve list_sessions aracını sunar. Aktif (canlı) sessionlar otomatik gönderilmez — ajan onları list_sessions ile kendi çeker. Mevcut başlık/özet kullanılır (yeni LLM çağrısı yok)."
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
        Kod bilgi-grafiği (codebase-memory)
      </div>
      <Toggle
        label="codebase-memory yeteneği"
        hint="Bu workspace'te bir codebase-memory MCP sunucusu varsa: ajanın bağlamına 'kod bilgi-grafiği mevcut' ipucu eklenir, sunucu workspace'e özel izole bir indeks store'a yönlendirilir (indeksler karışmaz), session çalışma dizini otomatik indekslenir ve codebase_workspace_search (workspace-geneli arama) aracı sunulur. Kapalı = tamamen devre dışı (sunucu kendi varsayılan store'unu kullanır)."
        checked={ws.codebaseMemoryEnabled}
        onChange={(v) => setWsField('codebaseMemoryEnabled', v)}
      />

      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        Prompt cache — donmuş bağlam (prompt epoch)
      </div>
      <Toggle
        label="Prompt epoch (donmuş bağlam snapshot'ı)"
        hint="Açıkken bir oturumun statik sistem promptu + araç şemaları oturum başında dondurulur; oturum ortası değişiklikler (skill kurulumu, ayar/talimat düzenlemesi, MCP araç listesi değişimi) prompt cache'i kırmaz — compaction, uzun boşluk, model değişimi veya /refresh-context anında devreye girer. Ajan bu arada 'snapshot eski' notu görür; kapatılan araçlar yürütmede zaten anında engellenir. Kapalı = her tur canlı derlenir (her değişiklik cache'i kırar)."
        checked={ws.promptEpochEnabled}
        onChange={(v) => setWsField('promptEpochEnabled', v)}
      />

      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        Sil
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
