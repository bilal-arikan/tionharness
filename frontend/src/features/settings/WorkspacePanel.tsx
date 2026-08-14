// Per-workspace category: identity (icon), stats, instructions,
// provider/model overrides, autonomy pause and the delete danger zone. The
// publish/export-as-template flow now lives in its own "Dışa Aktar" sub-tab.
import type { WorkspaceSettings } from '@/types'
import { Field, Toggle, inputCls, type WsSet } from './primitives'
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
        Bu ayarlar yalnızca <span className="font-medium text-[var(--color-text)]">{ws.name}</span>{' '}
        workspace'ine özeldir. Her ajanın sağlayıcı ve modeli kendi ayarında net olarak belirtilir;
        yeni ajanlar mevcut ilk ajanın kurulumunu miras alır.
      </div>

      {/* Stats */}
      <div className="grid grid-cols-4 gap-2">
        {[
          { label: 'Ajan', value: ws.agentCount },
          { label: 'Oturum', value: ws.sessionCount },
          { label: 'Görev', value: ws.taskCount },
          { label: 'Oluşturma', value: new Date(ws.createdAt * 1000).toLocaleDateString('tr-TR') },
        ].map((s) => (
          <div
            key={s.label}
            className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-2 text-center"
          >
            <div className="text-sm font-semibold">{s.value}</div>
            <div className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
              {s.label}
            </div>
          </div>
        ))}
      </div>

      <div className="flex items-end gap-3">
        <Field label="Workspace adı">
          <input
            value={ws.name}
            onChange={(e) => setWsField('name', e.target.value)}
            className={inputCls}
          />
        </Field>
        <Field label="İkon (emoji)">
          <EmojiField value={ws.icon} onChange={(e) => setWsField('icon', e)} clearLabel="⬡" />
        </Field>
      </div>

      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        Yanıt stili
      </div>
      <Toggle
        label="Terse mod (caveman)"
        hint="Açıkken bu workspace'teki her ajanın statik system promptuna sıkıştırılmış-yanıt talimatı eklenir: dolgu sözcükler, nezaket kalıpları ve hedging düşer; kod, komut, dosya yolu ve hata metinleri harfi harfine korunur. Güvenlik uyarıları ve geri alınamaz işlem onayları bilerek uzun yazılır. Metin düzenlenebilir bir workspace promptudur (Promptlar & Dosyalar ▸ 'Terse (caveman) yanıt stili'), yani kuralları kendine göre değiştirebilirsin. Statik prefix'te olduğu için prompt-cache penceresi başına bir kez ödenir. Değişiklik açık oturumlara /refresh-context veya yeni oturumla yansır."
        checked={ws.terseMode}
        onChange={(v) => setWsField('terseMode', v)}
      />

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
        Shell çıktısı sıkıştırma (sqz)
      </div>
      <Field
        label="Büyük shell çıktısını sqz ile sıkıştır"
        hint="Ajanın shell (Bash/PowerShell) komut çıktısı, modele dönmeden önce yerel 'sqz compress' ile in-process kısaltılır (kayıpsız n-gram; canlı UI ham kalır, yalnız modele giden sonuç küçülür). sqz'in PreToolUse hook'u yalnız native 'Bash' adını tanıdığı ve TionSwarm shell'i bridged araçla koşturduğu için hook yolu çalışmaz — bu ayar onun yerine geçer. Otomatik = sqz hook bağlıysa açık; Açık = hook olmasa da açık (sqz binary gerekir); Kapalı = devre dışı."
      >
        <select
          value={ws.shellOutputCompression || ''}
          onChange={(e) =>
            setWsField('shellOutputCompression', e.target.value as '' | 'on' | 'off')
          }
          className={inputCls}
        >
          <option value="">Otomatik (sqz hook varsa)</option>
          <option value="on">Açık (zorla)</option>
          <option value="off">Kapalı</option>
        </select>
      </Field>

      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        Shell komutu yeniden yazma (rtk)
      </div>
      <Field
        label="Test/build komutlarını rtk ile çalıştır"
        hint="sqz'nin tamamlayıcısı, karşı uçta: sqz çıktıyı sonradan sıkıştırır, rtk komutu ÖNCEDEN değiştirip daha az çıktı üretmesini sağlar. Yalnız ölçülmüş kazanç veren aileler (go/cargo/npm/pytest/jest… test-build-lint koşucuları + git status/log) yeniden yazılır; git diff ve cat kapsam DIŞI (ölçümde sqz daha iyi, rtk read ham çıktıdan büyük). rtk ÖZET döndürür — geçen testler düşer, hatalar dosya:satır ile korunur. Komut başarısız olursa ajana 'bu bir özet' notu eklenir. Otomatik = rtk hook bağlıysa açık; Açık = hook olmasa da açık (rtk binary gerekir); Kapalı = devre dışı."
      >
        <select
          value={ws.shellCommandRewrite || ''}
          onChange={(e) => setWsField('shellCommandRewrite', e.target.value as '' | 'on' | 'off')}
          className={inputCls}
        >
          <option value="">Otomatik (rtk hook varsa)</option>
          <option value="on">Açık (zorla)</option>
          <option value="off">Kapalı</option>
        </select>
      </Field>

      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        Sil
      </div>

      {onDeleteWorkspace && (
        <div className="mt-2 flex items-center justify-between rounded-lg border border-[color-mix(in_srgb,var(--color-danger)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_6%,transparent)] px-3 py-2">
          <span className="text-xs text-[var(--color-text-dim)]">
            Bu workspace'i ve tüm verisini kalıcı olarak sil.
          </span>
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
