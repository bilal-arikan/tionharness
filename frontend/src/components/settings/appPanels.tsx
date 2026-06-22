// The simple app-global setting categories (form fields only). The stateful
// categories (providers, commands, step kinds, workspace) live in their own
// files; these are pure draft+setter forms.
import { useState, useEffect } from 'react'
import { Layers, Database, NotebookPen, LifeBuoy, Bell, Scissors, Sparkles, FlaskConical, type LucideIcon } from 'lucide-react'
import type { VersionInfo } from '../../types'
import { api } from '../../api'
import type { AppSettings } from '../../types'
import { THEME_PRESETS } from '../../lib/themePresets'
import { NOTIFY_TYPES, mutedTypes, setTypeEnabled } from '../../lib/notifyPrefs'
import { Field, Toggle, Slider, inputCls, type AppSet } from './primitives'

interface PanelProps {
  draft: AppSettings
  set: AppSet
  setDraft: React.Dispatch<React.SetStateAction<AppSettings | null>>
}

// SubHead is a small icon + label group header used to organise a settings
// panel into labelled sections (accent icon, matches the rail/header style).
function SubHead({ icon: Icon, children }: { icon: LucideIcon; children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-1.5 pt-1 text-sm font-medium text-[var(--color-text)]">
      <Icon size={14} className="text-[var(--color-accent)]" />
      {children}
    </div>
  )
}

export function ProfilePanel({ draft, set }: PanelProps) {
  return (
    <>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
        Bu bilgiler ajanların yanıtlarını sana göre kişiselleştirmesi için sohbet bağlamına eklenir.
      </div>
      <Field label="Ad" hint="Ajan sana nasıl hitap etsin."><input value={draft.userName} onChange={(e) => set('userName', e.target.value)} placeholder="örn. Ada" className={inputCls} /></Field>
      <Field label="Saat dilimi" hint="'yarın', 'gelecek hafta' gibi göreli tarihler için."><input value={draft.userTimezone} onChange={(e) => set('userTimezone', e.target.value)} placeholder="örn. Europe/Istanbul" className={inputCls} /></Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Şehir"><input value={draft.userCity} onChange={(e) => set('userCity', e.target.value)} placeholder="örn. İstanbul" className={inputCls} /></Field>
        <Field label="Ülke"><input value={draft.userCountry} onChange={(e) => set('userCountry', e.target.value)} placeholder="örn. Türkiye" className={inputCls} /></Field>
      </div>
      <Field label="Notlar" hint="Tercihlerini anlatan serbest metin (talimatlar, çalışma şekli…)."><textarea value={draft.userNotes} onChange={(e) => set('userNotes', e.target.value)} rows={5} className={`${inputCls} resize-none`} placeholder="Ajanların bilmesi gereken tercihlerin…" /></Field>
    </>
  )
}

export function NotificationsPanel({ draft, set }: PanelProps) {
  // Per-type toast preferences are device-local (localStorage), so they apply
  // instantly — independent of the backend-persisted master toggle / Save.
  const [muted, setMuted] = useState<Set<string>>(() => mutedTypes())
  const toggleType = (type: string, on: boolean) => {
    setTypeEnabled(type, on)
    setMuted((prev) => {
      const next = new Set(prev)
      if (on) next.delete(type)
      else next.add(type)
      return next
    })
  }
  return (
    <>
      <Toggle label="Masaüstü bildirimleri" hint="Pencere arkadayken olay gerçekleşince tarayıcı bildirimi gösterir (izin ister)." checked={draft.desktopNotifications} onChange={(v) => set('desktopNotifications', v)} />
      <Toggle label="Ekranı açık tut" hint="Uygulama açıkken ekran uyku moduna geçmez (Wake Lock)." checked={draft.keepAwake} onChange={(v) => set('keepAwake', v)} />

      <SubHead icon={Bell}>Bildirim türleri</SubHead>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">
        Bir türü kapatınca o olay için masaüstü bildirimi gösterilmez. Bu ayarlar bu cihaza özeldir ve anında uygulanır.
      </p>
      {NOTIFY_TYPES.map((t) => (
        <Toggle key={t.type} label={t.label} hint={t.hint} checked={!muted.has(t.type)} onChange={(v) => toggleType(t.type, v)} />
      ))}
    </>
  )
}

export function AppearancePanel({ draft, set, setDraft }: PanelProps) {
  return (
    <>
      <Field label="Tema paleti" hint="Hazır bir palet seç; tüm arayüz yeniden renklenir. Vurgu rengini aşağıdan ince ayarlayabilirsin.">
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
          {THEME_PRESETS.map((p) => {
            const sel = draft.themePreset === p.id
            return (
              <button
                key={p.id}
                type="button"
                onClick={() => setDraft((d) => (d ? { ...d, themePreset: p.id, accent: p.tokens.accent } : d))}
                className={`flex items-center gap-2.5 rounded-lg border px-2.5 py-2 text-left transition ${
                  sel
                    ? 'border-[var(--color-accent)] ring-1 ring-[var(--color-accent)]'
                    : 'border-[var(--color-border)] hover:border-[var(--color-accent)]'
                }`}
              >
                <span
                  className="flex h-7 w-7 shrink-0 items-center justify-center overflow-hidden rounded-md border"
                  style={{ background: p.tokens.bg, borderColor: p.tokens.border }}
                >
                  <span className="h-3.5 w-3.5 rounded-full" style={{ background: p.tokens.accent }} />
                </span>
                <span className="min-w-0">
                  <span className="block truncate text-xs font-medium text-[var(--color-text)]">{p.label}</span>
                  <span className="block text-[10px] text-[var(--color-text-dim)]">{p.dark ? 'Koyu' : 'Açık'}</span>
                </span>
              </button>
            )
          })}
        </div>
      </Field>
      <Field label="Temel mod" hint="Yalnızca özel palet kullanılmadığında (sistem otomatik açık/koyu) etkilidir.">
        <select value={draft.theme} onChange={(e) => set('theme', e.target.value as AppSettings['theme'])} className={inputCls}>
          <option value="dark">Koyu</option>
          <option value="light">Açık</option>
          <option value="system">Sistem</option>
        </select>
      </Field>
      <Field label="Vurgu rengi (accent)">
        <div className="flex items-center gap-2">
          <input type="color" value={draft.accent} onChange={(e) => set('accent', e.target.value)} className="h-9 w-12 cursor-pointer rounded border border-[var(--color-border)] bg-[var(--color-bg)]" />
          <input value={draft.accent} onChange={(e) => set('accent', e.target.value)} className={`${inputCls} w-32`} />
        </div>
      </Field>
      <Field label="Dil" hint="UI dili tercihi (tam çeviri kademeli ekleniyor).">
        <select value={draft.language} onChange={(e) => set('language', e.target.value as AppSettings['language'])} className={inputCls}>
          <option value="tr">Türkçe</option>
          <option value="en">English</option>
        </select>
      </Field>
    </>
  )
}

export function ContextPanel({ draft, set }: PanelProps) {
  return (
    <>
      <SubHead icon={Layers}>Bağlam penceresi</SubHead>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Maks. bağlam token" hint="Aşılınca eski turlar özetlenir."><input type="number" value={draft.maxContextTokens} onChange={(e) => set('maxContextTokens', Number(e.target.value))} className={inputCls} /></Field>
        <Field label="Korunan son mesaj" hint="Her zaman aynen gönderilir."><input type="number" value={draft.keepRecentMsgs} onChange={(e) => set('keepRecentMsgs', Number(e.target.value))} className={inputCls} /></Field>
      </div>

      <SubHead icon={FlaskConical}>Anthropic beta</SubHead>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">Yalnız anthropic sağlayıcıda etkili; claude-cli'da etkisizdir.</p>
      <Toggle label="1 milyon token bağlam" hint="Anthropic 1M context penceresi beta'sı (anthropic-beta başlığı). claude-cli'da etkisizdir." checked={draft.oneMillionContext} onChange={(v) => set('oneMillionContext', v)} />
      <Toggle label="Uzatılmış prompt cache (1 saat)" hint="Sistem promptunu 1 saatlik cache_control ile önbelleğe alır — tekrar eden büyük persona/bağlam ucuzlar." checked={draft.extendedPromptCache} onChange={(v) => set('extendedPromptCache', v)} />

      <SubHead icon={Database}>Hafıza geri çağırma (recall)</SubHead>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Recall sonuç sayısı (top-N)"><input type="number" value={draft.recallTopN} onChange={(e) => set('recallTopN', Number(e.target.value))} className={inputCls} /></Field>
        <Field label="Recall min skor" hint="0–1 arası benzerlik eşiği."><input type="number" step="0.01" value={draft.recallMinScore} onChange={(e) => set('recallMinScore', Number(e.target.value))} className={inputCls} /></Field>
      </div>

      <SubHead icon={NotebookPen}>Günlük & yansıma</SubHead>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Günlük kayıt limiti" hint="Ajan başına saklanan en yeni journal sayısı; eskiler budanır."><input type="number" value={draft.journalCap} onChange={(e) => set('journalCap', Number(e.target.value))} className={inputCls} /></Field>
        <Field label="Günlük kayıt uzunluğu" hint="Tek bir journal kaydı için maks. karakter."><input type="number" value={draft.journalMaxLen} onChange={(e) => set('journalMaxLen', Number(e.target.value))} className={inputCls} /></Field>
        <Field label="Otomatik yansıma eşiği" hint="Journal sayısı bunu aşınca dream cycle kendiliğinden tetiklenir."><input type="number" value={draft.autoReflectThreshold} onChange={(e) => set('autoReflectThreshold', Number(e.target.value))} className={inputCls} /></Field>
      </div>
      <Toggle label="Otomatik yansıma (dream cycle)" hint="Journal eşiği aşılınca ajan kendi günlüğünü arka planda özetler ve özetlenen kayıtları siler. Otonom çağrı sayılır: duraklatma ve günlük bütçeye saygı gösterir." checked={draft.autoReflect} onChange={(v) => set('autoReflect', v)} />

      <SubHead icon={Database}>Çekirdek bellek (MemGPT)</SubHead>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        Letta/MemGPT tarzı kendi-düzenleyen bellek: bağlam dolmaya yaklaşınca ajana "önemliyi şimdi yaz" uyarısı gösterilir; ajan ayrıca her turda sabit kalan,
        kendi düzenlediği bir <span className="font-medium text-[var(--color-text)]">çekirdek bellek</span> bloğunu <code>core_memory_replace</code>/<code>core_memory_append</code> ile yönetir.
      </div>
      <Slider
        label="Bağlam basıncı uyarı eşiği"
        min={0}
        max={1}
        step={0.05}
        value={draft.memoryPressureWarn}
        onChange={(v) => set('memoryPressureWarn', v)}
        badge={draft.memoryPressureWarn <= 0 ? 'Kapalı' : `%${Math.round(draft.memoryPressureWarn * 100)}`}
        hint="Bağlam doluluğu bu oranı aşınca tura 'belleğe yaz' uyarısı eklenir. Sola dayayınca (0) kapanır."
        sub={
          draft.memoryPressureWarn > 0
            ? `≈ ${Math.round(draft.maxContextTokens * draft.memoryPressureWarn).toLocaleString('tr-TR')} token dolunca tetiklenir (maks. ${draft.maxContextTokens.toLocaleString('tr-TR')} token üzerinden).`
            : 'Uyarı kapalı — ajan bağlam dolduğunda sessizce sıkıştırılır.'
        }
      />
      <Toggle label="Çekirdek bellek araçları" hint="core_memory_replace/append araçlarını ajana sun (kapatınca minimal araç yüzeyi)." checked={draft.coreMemoryTools} onChange={(v) => set('coreMemoryTools', v)} />
      {/* Live status card (B): the net effect of the two controls at a glance. */}
      <div className="flex flex-wrap items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-xs">
        <span className="text-[var(--color-text-dim)]">Durum:</span>
        <span className={`rounded px-1.5 py-0.5 font-medium ${draft.coreMemoryTools ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]' : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'}`}>
          Çekirdek araçları {draft.coreMemoryTools ? 'açık' : 'kapalı'}
        </span>
        <span className={`rounded px-1.5 py-0.5 font-medium ${draft.memoryPressureWarn > 0 ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]' : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'}`}>
          {draft.memoryPressureWarn > 0 ? `Uyarı %${Math.round(draft.memoryPressureWarn * 100)}'te` : 'Uyarı kapalı'}
        </span>
      </div>

      <SubHead icon={LifeBuoy}>Tur kurtarma & sıkıştırma</SubHead>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        <span className="font-medium text-[var(--color-text)]">Tur kurtarma (A1).</span> Ajanın araç döngüsü "mutlu yol" dışına çıktığında turu yapısal olarak kurtarır: modelin
        cevabı çıktı-token limitine takılırsa kaldığı yerden <em>sürdürür</em> (parçalar tek cevapta birleştirilir), bağlam penceresi taşarsa eski mesajları
        özetleyip turu <em>yeniden dener</em>. Her kurtarma tek-atımlıktır, sonsuz döngü olmaz. Yalnız native (Anthropic/MiniMax) yolunda etkilidir; claude-cli kendi döngüsünü sürdürür.
      </div>
      <Toggle label="Reaktif sıkıştırma" hint="Bağlam taşması hatasında eski tur geçmişi özetlenip tur yeniden denenir. Kapalıyken taşma turu sonlandırır." checked={draft.reactiveCompact} onChange={(v) => set('reactiveCompact', v)} />
      <div className="grid grid-cols-2 gap-3">
        <Field label="Maks. token resume denemesi" hint="Çıktı limiti aşılınca tur kaç kez sürdürülür (0 = kapalı; kısmi cevap olduğu gibi gösterilir)."><input type="number" value={draft.maxTokenRetries} onChange={(e) => set('maxTokenRetries', Number(e.target.value))} className={inputCls} /></Field>
        <Field label="Sıkıştırmada korunan mesaj" hint="Reaktif sıkıştırmada aynen tutulan en yeni mesaj sayısı (≥2)."><input type="number" value={draft.reactiveKeepRecent} onChange={(e) => set('reactiveKeepRecent', Number(e.target.value))} className={inputCls} /></Field>
      </div>

      <SubHead icon={Scissors}>Araç çıktısı sıkıştırma — Sistem A (deterministik)</SubHead>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        Araç çıktıları (shell, dosya, MCP) modele dönmeden önce <span className="font-medium text-[var(--color-text)]">ücretsiz, kural tabanlı</span> kısaltılır:
        ardışık tekrar satırları birleştirilir, boş satır blokları sadeleşir, çok uzun çıktının ortası kırpılıp baş/son korunur. Model çağrısı yapmaz; her çıktıda çalışır.
      </div>
      <Toggle label="Deterministik sıkıştırma (Sistem A)" hint="Token kazanımı için araç çıktılarını yerel olarak kısaltır. Hata çıktıları aynen korunur." checked={draft.compactToolOutput} onChange={(v) => set('compactToolOutput', v)} />
      <div className="grid grid-cols-2 gap-3">
        <Field label="Maks. satır" hint="Bu sayıyı aşan çıktının ortası atlanır (baş+son korunur)."><input type="number" value={draft.compactMaxLines} onChange={(e) => set('compactMaxLines', Number(e.target.value))} className={inputCls} /></Field>
        <Field label="Maks. bayt" hint="Satır işleminden sonra uygulanan sert bayt sınırı."><input type="number" value={draft.compactMaxBytes} onChange={(e) => set('compactMaxBytes', Number(e.target.value))} className={inputCls} /></Field>
      </div>

      <SubHead icon={Sparkles}>Araç çıktısı özeti — Sistem B (LLM)</SubHead>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        Sistem A'dan sonra çıktı hâlâ eşik üstündeyse, ucuz bir model (başlık modeli → yoksa ajanın modeli) çıktıyı <span className="font-medium text-[var(--color-text)]">niyet-farkında</span> özetler.
        Sistem A'dan bağımsızdır; ikisi de açıkken ardışık çalışır. <span className="text-[var(--color-warning)]">Ek bir model çağrısı maliyeti</span> getirir, bu yüzden varsayılan kapalıdır.
      </div>
      <Toggle label="LLM intent-aware özet (Sistem B)" hint="Eşik üstü araç çıktılarını ucuz modelle özetler. Maliyetlidir; kullanım 'compact' türünde sayaca işlenir." checked={draft.compactLlmSummary} onChange={(v) => set('compactLlmSummary', v)} />
      <div className="grid grid-cols-2 gap-3">
        <Field label="Özet eşiği (bayt)" hint="Sistem A sonrası bu boyutu aşan çıktılar özetlenir."><input type="number" value={draft.compactLlmThreshold} onChange={(e) => set('compactLlmThreshold', Number(e.target.value))} className={inputCls} /></Field>
        <Field label="Özet modeli" hint="Sistem B'nin kullanacağı model. Boşsa: Başlık modeli → o da boşsa ajanın kendi modeli. Sağlayıcı her zaman ajanın sağlayıcısıdır. Ucuz bir model (ör. claude-haiku-4-5) önerilir."><input type="text" value={draft.compactModel} placeholder="boş = başlık modeli / ajan modeli" onChange={(e) => set('compactModel', e.target.value)} className={inputCls} /></Field>
      </div>
    </>
  )
}

export function BudgetPanel({ draft, set }: PanelProps) {
  return (
    <div className="grid grid-cols-2 gap-3">
      <Field label="Günlük çağrı limiti" hint="0 = sınırsız"><input type="number" value={draft.defaultDailyCallLimit} onChange={(e) => set('defaultDailyCallLimit', Number(e.target.value))} className={inputCls} /></Field>
      <Field label="Günlük token limiti" hint="0 = sınırsız"><input type="number" value={draft.defaultDailyTokenLimit} onChange={(e) => set('defaultDailyTokenLimit', Number(e.target.value))} className={inputCls} /></Field>
      <p className="col-span-2 text-xs text-[var(--color-text-dim)]">Bu varsayılanlar yalnızca yeni oluşturulan ajanlara uygulanır.</p>
    </div>
  )
}

export function AutonomyPanel({ draft, set }: PanelProps) {
  return (
    <>
      <Toggle label="Tüm otonomiyi duraklat (uygulama geneli)" hint="Zamanlanmış çağrılar modele gitmeden bloklanır. Manuel sohbet etkilenmez." checked={draft.pauseAutonomy} onChange={(v) => set('pauseAutonomy', v)} />
    </>
  )
}

export function AutoTitlePanel({ draft, set }: PanelProps) {
  return (
    <>
      <Toggle label="Otomatik başlık üretimi" hint="Sohbet ilk mesajında ve görev oluşturmada başlık otomatik üretilir." checked={draft.autoTitleEnabled} onChange={(v) => set('autoTitleEnabled', v)} />
      <Field label="Başlık modeli" hint="Boş = ajanın kendi modeli. Ucuz bir model seçebilirsin."><input value={draft.titleModel} onChange={(e) => set('titleModel', e.target.value)} placeholder="örn. haiku" className={inputCls} /></Field>
    </>
  )
}

export function McpPanel({ draft, set }: PanelProps) {
  return (
    <Field label="MCP Gateway URL" hint="Araç entegrasyonları için ağ geçidi adresi."><input value={draft.mcpGatewayUrl} onChange={(e) => set('mcpGatewayUrl', e.target.value)} placeholder="http://localhost:9091/mcp" className={inputCls} /></Field>
  )
}

export function ToolsPanel({ draft, set }: PanelProps) {
  return (
    <>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
        Bu yetenekler varsayılan olarak <b>kapalıdır</b>: her biri ajanların gücünü ve
        token maliyetini artırır. Değişiklikler tüm workspace'lere canlı uygulanır.
      </div>
      <Toggle
        label="Kabuk (shell) aracı"
        hint="Built-in `shell`: ajan workspace sandbox'ında komut çalıştırır (Windows'ta PowerShell). claude-cli ajanlarında bu araç köprülenir ve CLI'nin native `Bash`'i bastırılır — tüm komutlar SwarmGo shell'inden geçer. Yüksek risk."
        checked={draft.enableShell}
        onChange={(v) => set('enableShell', v)}
      />
      <Toggle
        label="Öz-yönetim araç paketi"
        hint="Ajanların ajan/akış/zamanlama/artifact oluşturup düzenlemesine izin verir. Araç kataloğunu kabaca iki katına çıkarır."
        checked={draft.enableSelfManage}
        onChange={(v) => set('enableSelfManage', v)}
      />
      <Toggle
        label="Hook'ları claude-cli'ye geçir"
        hint="Açıkken workspace PreToolUse/PostToolUse hook'ları claude-cli ajanlarına da `--settings` ile uygulanır (yalnız native değil). Uyarı: CLI hook'ları CLI'nin kendi shell'inde koşar; SwarmGo shell'i (PowerShell) için yazılmış bir hook uyumsuz olabilir — sorun çıkarsa kapatın."
        checked={draft.enableCliHooks}
        onChange={(v) => set('enableCliHooks', v)}
      />
      <Toggle
        label="Ajan→ajan delegasyon (run_subagent)"
        hint="Bir ajan, izole bir alt-ajana (yerleşik profil ya da mevcut bir ajan) alt-görev devredip cevabını bekleyebilir; tek turda paralel de çağrılabilir. Her çağrı tam bir alt-ajan turu koşar (token maliyeti). Yalnızca native/anthropic tool yolunda."
        checked={draft.enableDelegation}
        onChange={(v) => set('enableDelegation', v)}
      />
      {draft.enableDelegation && (
        <div className="grid grid-cols-2 gap-3">
          <Field label="Maks. delegasyon derinliği" hint="Zincirin kaç kat iç içe gidebileceği (1–10). Döngü koruması.">
            <input type="number" min={1} max={10} value={draft.delegationMaxDepth} onChange={(e) => set('delegationMaxDepth', Number(e.target.value))} className={inputCls} />
          </Field>
          <Field label="Tur başına maks. delegasyon" hint="Tek kullanıcı turunda toplam run_subagent çağrısı (1–100). Bütçe koruması.">
            <input type="number" min={1} max={100} value={draft.delegationMaxCalls} onChange={(e) => set('delegationMaxCalls', Number(e.target.value))} className={inputCls} />
          </Field>
        </div>
      )}

      <SubHead icon={Sparkles}>Spawn (arka plan) limitleri</SubHead>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">
        Ayrık arka plan yüzeyi için sınırlar: <code>run_subagent</code> (async) ve köprülenen <code>spawn_session</code> + UI spawn düğmesi.
      </p>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Maks. eşzamanlı spawn" hint="Aynı anda çalışabilen spawn edilmiş oturum sayısı (1–128).">
          <input type="number" min={1} max={128} value={draft.spawnMaxConcurrent} onChange={(e) => set('spawnMaxConcurrent', Number(e.target.value))} className={inputCls} />
        </Field>
        <Field label="Tur başına maks. spawn" hint="Tek ajan turunda başlatılabilecek spawn sayısı (1–64).">
          <input type="number" min={1} max={64} value={draft.spawnMaxPerTurn} onChange={(e) => set('spawnMaxPerTurn', Number(e.target.value))} className={inputCls} />
        </Field>
      </div>

      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
        <b>Çalışma dizini güvenliği.</b> Dosya/kabuk araçları artık workspace'e kilitli
        değil (her yola erişebilir). Aşağıdaki frenler bu gücü <b>otonom</b>
        (zamanlama/spawn/flow — insan döngüde değil) turlarda dengeler. İnteraktif
        sohbet etkilenmez.
      </div>
      <Toggle
        label="Otonom turları çalışma dizinine kilitle"
        hint="Açıkken zamanlama/spawn/flow ile çalışan ajanların dosya araçları yalnızca oturumun çalışma dizininde kalır (mutlak yol + `..` kaçışı reddedilir) ve `git push` engellenir. Önerilen: AÇIK."
        checked={draft.autonomousConfine}
        onChange={(v) => set('autonomousConfine', v)}
      />
      <Toggle
        label="Git worktree izolasyonu (otonom)"
        hint="Açıkken çalışma dizini bir git deposuysa, otonom oturumlar depoyu doğrudan değiştirmek yerine oturuma özel bir git worktree + dal alır. Paralel ajanların birbirinin dosyalarını ezmesini önler. Git gerektirir; oturum silinince worktree temizlenir."
        checked={draft.gitWorktreeIsolation}
        onChange={(v) => set('gitWorktreeIsolation', v)}
      />
    </>
  )
}

export function DiagnosticsPanel({ draft, set }: PanelProps) {
  return (
    <Field label="Log seviyesi" hint="Yeniden başlatınca uygulanır.">
      <select value={draft.logLevel} onChange={(e) => set('logLevel', e.target.value)} className={inputCls}>
        <option value="debug">debug</option>
        <option value="info">info</option>
        <option value="warn">warn</option>
        <option value="error">error</option>
      </select>
    </Field>
  )
}

export function AboutPanel() {
  const [info, setInfo] = useState<VersionInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api.getVersion()
      .then(setInfo)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  const isDev = !info || info.version === 'dev'

  return (
    <div className="space-y-5 text-sm">
      {/* App identity */}
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-[var(--color-accent)] text-xl font-bold text-white select-none">
          S
        </div>
        <div>
          <div className="text-base font-semibold text-[var(--color-text)]">SwarmGo</div>
          <div className="text-[var(--color-text-dim)]">Çok-ajanlı AI runtime</div>
        </div>
      </div>

      {/* Version table */}
      <div className="rounded-lg border border-[var(--color-border)] divide-y divide-[var(--color-border)]">
        <AboutRow label="Sürüm">
          {loading ? (
            <span className="text-[var(--color-text-dim)]">yükleniyor…</span>
          ) : error ? (
            <span className="text-[var(--color-danger)]">Alınamadı</span>
          ) : isDev ? (
            <span className="rounded bg-[var(--color-warning)]/15 px-1.5 py-0.5 text-[var(--color-warning)] font-mono text-xs">dev build</span>
          ) : (
            <span className="font-mono">{info!.version}</span>
          )}
        </AboutRow>

        {info && info.commit !== 'dev' && (
          <AboutRow label="Commit">
            <span className="font-mono text-xs">{info.commit}</span>
          </AboutRow>
        )}

        {info && info.buildDate !== 'dev' && (
          <AboutRow label="Build tarihi">
            <span>{new Date(info.buildDate).toLocaleDateString('tr-TR', { dateStyle: 'medium' })}</span>
          </AboutRow>
        )}

        <AboutRow label="Go sürümü">
          {loading ? (
            <span className="text-[var(--color-text-dim)]">—</span>
          ) : (
            <span className="font-mono text-xs">{info?.goVersion ?? '—'}</span>
          )}
        </AboutRow>
      </div>

      {/* Güncelleme notu */}
      <div className="rounded-lg border border-[var(--color-border)] px-3 py-2.5 text-[var(--color-text-dim)]">
        <span className="mr-1.5 text-[var(--color-warning)]">⚠</span>
        Otomatik güncelleme kontrolü henüz desteklenmiyor. Yeni sürümler için
        projeyi manuel olarak kontrol edin.
      </div>

      {/* Depolama açıklaması */}
      <p className="text-[var(--color-text-dim)] leading-relaxed">
        Uygulama ayarları{' '}
        <code className="rounded bg-[var(--color-surface-2)] px-1">settings.json</code>,
        workspace ayarları her workspace&apos;in{' '}
        <code className="rounded bg-[var(--color-surface-2)] px-1">ws-settings.json</code>{' '}
        dosyasında saklanır. Veritabanı kullanılmaz; tüm veriler düz dosyalardır.
      </p>
    </div>
  )
}

function AboutRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-4 px-3 py-2">
      <span className="w-28 shrink-0 text-[var(--color-text-dim)]">{label}</span>
      <span className="text-[var(--color-text)]">{children}</span>
    </div>
  )
}
