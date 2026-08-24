import { Layers, LifeBuoy, FlaskConical, RotateCcw, ListChecks, Bug, Tags } from 'lucide-react'
import { Field, Toggle, inputCls } from './primitives'
import { SubHead } from './settingsPanelShared'
import type { PanelProps } from './settingsPanelShared'

// Package defaults mirrored from the backend so the live preview matches the
// engine exactly: internal/conversation.defaultMaxTokens / defaultBudgetAutoCeil.
const DEFAULT_MAX_TOKENS = 12000
const DEFAULT_CEIL = 262144

// effectiveBudget mirrors internal/conversation.EffectiveBudget (budget.go):
// clamp(window × fraction, floor, ceil). It also reports which bound is ACTIVE so
// the preview can explain WHY a value lands where it does — the whole point of the
// fix, since "maks. bağlam token" is a FLOOR (it only lifts) and the confusion is
// that lowering it below the ceil-capped derived value changes nothing. autoFraction
// mirrors providers.AdaptiveBudgetFraction for the representative model family.
function effectiveBudget(
  window: number,
  floor: number,
  fraction: number,
  ceil: number,
  autoFraction: number,
): { value: number; bound: 'ceil' | 'floor' | 'fraction' } {
  const cfg = floor > 0 ? floor : DEFAULT_MAX_TOKENS
  const cl = ceil > 0 ? ceil : DEFAULT_CEIL
  const f = fraction > 0 ? fraction : autoFraction
  let derived = Math.floor(window * f)
  let bound: 'ceil' | 'floor' | 'fraction' = 'fraction'
  if (derived > cl) {
    derived = cl
    bound = 'ceil'
  }
  if (derived < cfg) {
    derived = cfg
    bound = 'floor'
  }
  return { value: derived, bound }
}

const fmtK = (n: number) => `${(n / 1000).toFixed(1)}K`
const boundLabel: Record<'ceil' | 'floor' | 'fraction', string> = {
  ceil: 'tavana kırpıldı',
  floor: 'tabana yükseltildi',
  fraction: 'pencere × oran',
}

export function ContextPanel({ draft, set }: PanelProps) {
  // Two representative windows so the clamp is tangible: a 1M model (Opus/Sonnet/
  // Fable, auto-fraction 0.45) and a 200K model (Haiku, 0.40). Computed live from
  // the current draft values.
  const previews = [
    {
      label: '1M model (Opus/Sonnet/Fable)',
      ...effectiveBudget(
        1_000_000,
        draft.maxContextTokens,
        draft.contextBudgetFraction,
        draft.contextBudgetCeil,
        0.45,
      ),
    },
    {
      label: '200K model (Haiku)',
      ...effectiveBudget(
        200_000,
        draft.maxContextTokens,
        draft.contextBudgetFraction,
        draft.contextBudgetCeil,
        0.4,
      ),
    },
  ]
  return (
    <>
      <SubHead icon={Layers}>Bağlam penceresi</SubHead>
      <div className="grid grid-cols-2 gap-3">
        <Field
          label="Maks. bağlam token — TABAN"
          hint="Bütçenin ALT sınırı: bütçeyi yalnızca YÜKSELTİR, asla düşürmez. Türetilen değer (pencere × oran, tavana kırpılı) bunun üstündeyse etkisizdir. Pencereyi KÜÇÜLTMEK için bunu değil, aşağıdaki 'Bütçe tavanı'nı düşür."
        >
          <input
            type="number"
            value={draft.maxContextTokens}
            onChange={(e) => set('maxContextTokens', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field label="Korunan son mesaj" hint="Her zaman aynen gönderilir.">
          <input
            type="number"
            value={draft.keepRecentMsgs}
            onChange={(e) => set('keepRecentMsgs', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Bütçe tavanı (token) — ÜST SINIR"
          hint="Bütçenin ÜST sınırı ve pencereyi KÜÇÜLTMEK için değiştireceğin ayar BUDUR. Varsayılan 262144 (256K) — ham pencereyi gradyanın yüksek-hassasiyet bölgesinde tutar (context-rot). Düşürürsen etkin bütçe buraya kırpılır; yükseltmek daha çok ham geçmiş tutar ama recall hassasiyetiyle takas eder."
        >
          <input
            type="number"
            value={draft.contextBudgetCeil}
            onChange={(e) => set('contextBudgetCeil', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Pencere oranı"
          hint="Transkripte ayrılan pay (0–1). 0 = otomatik (model-ailesine göre adaptif, önerilen). Pozitif değer sabit pay sabitler (ör. 0.45 → 1M model 450K, tavana kırpılır)."
        >
          <input
            type="number"
            step="0.05"
            value={draft.contextBudgetFraction}
            onChange={(e) => set('contextBudgetFraction', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
      </div>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">
        Etkin bütçe = clamp(pencere × oran, <span className="font-medium">taban</span>,{' '}
        <span className="font-medium">tavan</span>). Oran 0 = otomatik adaptif; bilinmeyen pencere →
        taban kullanılır.
      </p>

      {/* Live preview: shows the effective budget these three knobs resolve to, per
          representative window, plus WHICH bound is active — so it is obvious why
          lowering the floor below a ceil-capped value changes nothing. */}
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2">
        <div className="mb-1 text-[11px] font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
          Etkin bütçe önizlemesi (girdiğin değerlerle)
        </div>
        {previews.map((p) => (
          <div key={p.label} className="flex items-center justify-between gap-2 py-0.5 text-xs">
            <span className="min-w-0 truncate text-[var(--color-text)]">{p.label}</span>
            <span className="shrink-0 font-mono text-[var(--color-text-dim)]">
              <span className="font-semibold text-[var(--color-text)]">{fmtK(p.value)}</span>
              <span
                className={`ml-1.5 ${p.bound === 'ceil' ? 'text-[var(--color-warning)]' : p.bound === 'floor' ? 'text-[var(--color-accent)]' : ''}`}
              >
                · {boundLabel[p.bound]}
              </span>
            </span>
          </div>
        ))}
      </div>

      <SubHead icon={FlaskConical}>Anthropic beta</SubHead>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">
        Yalnız anthropic sağlayıcıda etkili; claude-cli'da etkisizdir. (1M bağlam artık GA — ayar
        gerekmez.)
      </p>
      <Toggle
        label="Uzatılmış prompt cache (1 saat)"
        hint="Sistem promptunu 1 saatlik cache_control ile önbelleğe alır — tekrar eden büyük persona/bağlam ucuzlar."
        checked={draft.extendedPromptCache}
        onChange={(v) => set('extendedPromptCache', v)}
      />
      <Toggle
        label="API-native bağlam düzenleme (clear_tool_uses)"
        hint="Sunucu, cache'li önekteki eski tool sonuçlarını yerinde budar (microcompact muadili) — önek soğumadan küçülür. İstemci-tarafı compaction'ı tamamlar, değiştirmez."
        checked={draft.anthropicContextEditing}
        onChange={(v) => set('anthropicContextEditing', v)}
      />
      <Toggle
        label="API-native araç arama (tool search)"
        hint="Tüm araç kataloğu defer_loading ile gönderilir + sunucu-tarafı regex arama aracı eklenir: model, activate_tools tur-gidiş-dönüşü olmadan araç keşfeder; bulunan şemalar cache'i BOZMADAN eklenir. Yalnız birinci-parti anthropic sağlayıcı; mevcut activate_tools/tool_search akışı yanında çalışmaya devam eder."
        checked={draft.anthropicNativeToolSearch}
        onChange={(v) => set('anthropicNativeToolSearch', v)}
      />
      <Toggle
        label="Programatik araç çağrısı (code execution)"
        hint="Model, Anthropic'in sunucu container'ında Python yazarak araçları KOD İÇİNDEN çağırır (allowed_callers): ara sonuçlar bağlama hiç girmez — çok adımlı araç zincirlerinde token+tur tasarrufu. Yerel run_code'dan farkı: yerel Python gerekmez. MCP ve etkileşimli araçlar hariç; izin kapısı her çağrıda yine çalışır. Yalnız birinci-parti anthropic + Claude 4.5+ modeller."
        checked={draft.anthropicProgrammaticTools}
        onChange={(v) => set('anthropicProgrammaticTools', v)}
      />
      <Toggle
        label="Sunucu-tarafı web arama + sayfa çekme"
        hint="web_search/web_fetch sunucu araçları isteğe eklenir: aramayı Anthropic kendi altyapısında yürütür, alıntılı (citation) sonuçlar aynı yanıtta döner — makinede tarayıcı/servis çalışmaz. 4.6+ modellerde dinamik filtreli sürüm (sonuçlar bağlama girmeden süzülür), eskilerde temel sürüm. Arama başına ücretlendirilir; tur başına kullanım tavanı uygulanır (arama 8, çekme 12). Yalnız birinci-parti anthropic + araçları açık ajanlar."
        checked={draft.anthropicWebTools}
        onChange={(v) => set('anthropicWebTools', v)}
      />
      <Toggle
        label="API-native compaction (sunucu özetleme)"
        hint="Prompt ~150K token eşiğine yaklaşınca geçmişi SUNUCU özetler (compaction blokları); tur içinde bloklar aynen geri gönderilir. İstemci-tarafı compaction turlar-arası transkripti yönetmeye devam eder — bu, uzun TEK turların (araç döngüleri) taşma sigortasıdır ve compaction LLM çağrısının maliyetini sunucuya taşır. Beta; yalnız anthropic sağlayıcı."
        checked={draft.anthropicServerCompaction}
        onChange={(v) => set('anthropicServerCompaction', v)}
      />
      <Toggle
        label="Fable 5 red-fallback (Opus 4.8)"
        hint="Fable 5'in güvenlik sınıflandırıcıları bir isteği reddederse (masum güvenlik/biyoloji-bitişiği işlerde yanlış-pozitif olabilir) istek AYNI çağrı içinde Opus 4.8 tarafından yanıtlanır — tur boş düşmez. Red öncesi kısım faturalanmaz; kurtarma Opus fiyatından. Yalnız Fable/Mythos isteklerine eklenir; Anthropic'in önerisiyle varsayılan açık. Not: Fable 5 ayrıca 30 günlük veri saklama gerektirir (ZDR organizasyonlarda her istek 400 döner)."
        checked={draft.anthropicRefusalFallback}
        onChange={(v) => set('anthropicRefusalFallback', v)}
      />
      <Field
        label="Otonom görev bütçesi (token)"
        hint="0 = kapalı. Pozitifken her OTONOM tura API-native task_budget bildirilir: model tüm araç döngüsü için geri sayımı görür ve kendini ona göre ayarlar (kesilmek yerine düzgün toparlar). API minimumu 20000'dir — altı otomatik yükseltilir. Yalnız adaptive-sınıf anthropic modeller (Opus 4.7/4.8, Sonnet 5, Fable 5)."
      >
        <input
          type="number"
          min={0}
          step={1000}
          value={draft.autonomousTaskBudgetTokens}
          onChange={(e) => set('autonomousTaskBudgetTokens', Number(e.target.value))}
          className={inputCls}
        />
      </Field>

      <SubHead icon={RotateCcw}>Context reset (handoff)</SubHead>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        Anthropic "harness design" deseni: uzun otonom görevlerde yerinde sıkıştırma tek başına
        "context anxiety"yi (modelin limite yaklaşınca işi erken toparlaması) çözmez. Açıkken,
        bağlam sınırına çarpan otonom bir tur özetlenmek yerine bir{' '}
        <span className="font-medium text-[var(--color-text)]">handoff dosyası</span> yazar ve işi{' '}
        <span className="font-medium text-[var(--color-text)]">temiz bir pencerede</span> sürdürmek
        için yeni bir oturum başlatır. Manuel <code>/handoff</code> komutu ve{' '}
        <code>handoff_session</code> aracı bu ayardan bağımsız her zaman çalışır.
      </div>
      <Toggle
        label="Otomatik context reset"
        hint="Bağlam sınırına çarpan (reactive compaction tetikleyen) otonom tur, handoff yazıp temiz oturumda devam eder. Yalnız otonom turlar; manuel sohbet etkilenmez."
        checked={draft.handoffAuto}
        onChange={(v) => set('handoffAuto', v)}
      />
      <Field
        label="Maks. reset zinciri"
        hint="Art arda kaç context reset'e izin verilir; aşılınca normal sıkıştırmaya düşer (sonsuz zincir freni)."
      >
        <input
          type="number"
          min={1}
          max={100}
          className={inputCls}
          value={draft.handoffMaxChain || 20}
          onChange={(e) => set('handoffMaxChain', Number(e.target.value))}
        />
      </Field>
      <Toggle
        label="Handoff'u dosyaya da yaz"
        hint="Artifact'ın yanı sıra çalışma dizinine <workdir>/.tionharness/handoff.md olarak yazar (disk üstü progress dosyası deseni)."
        checked={draft.handoffWriteFile}
        onChange={(v) => set('handoffWriteFile', v)}
      />

      <SubHead icon={ListChecks}>Kalıcı ilerleme (progress)</SubHead>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        Anthropic'in <span className="font-medium text-[var(--color-text)]">claude-progress</span>{' '}
        konvansiyonu: <code>todo_write</code> kontrol listesi proje çalışma dizinine
        <code> &lt;cwd&gt;/.tionharness/progress.json</code> olarak yazılır (cwd yoksa ajan-başına
        depo dosyasına). Böylece liste oturumlar arası kaybolmaz; yeni bir oturum açıldığında
        kaldığı yerden devralınır. Dosya git-commit'lenebilir ve ajan dosya araçlarıyla okunabilir.
      </div>
      <Toggle
        label="İlerlemeyi diske yaz"
        hint="todo_write çağrıldığında kontrol listesi proje progress dosyasına kalıcılaşır. Kapalıyken bugünkü (oturum-içi efemeral) davranışa dönülür."
        checked={draft.progressPersist}
        onChange={(v) => set('progressPersist', v)}
      />
      <Toggle
        label="Yeni oturumda geri yükle"
        hint="Kendi listesi olmayan yeni bir oturuma, önceki oturumun progress dosyasındaki tamamlanmamış liste bağlam olarak enjekte edilir."
        checked={draft.progressResume}
        onChange={(v) => set('progressResume', v)}
      />

      <SubHead icon={Tags}>Otomatik etiketleme (olay → etiket)</SubHead>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        Tur olaylarına ve oturum durumuna göre oturuma well-known etiketler otomatik atanır:
        <code> tool-error</code> (gerçek araç hatası; claude-cli izin-verilmeyen araç reddi hariç),
        <code> error</code> (tur hatası), <code> goal</code>/<code>goal-done</code>/
        <code>archived</code>. Etiketler ekleme-yönlü kalır (bir onarıcı silene kadar) — bir
        otomasyonla hataları tarayıp otomatik onarmak için idealdir.
      </div>
      <Toggle
        label="Olaylara göre otomatik etiketle"
        hint="tool-error / error / goal / goal-done / archived etiketlerini turlarda ve arşivlemede otomatik atar. Kapalıyken hiçbir otomatik etiket yazılmaz (elle + ajan etiketleme çalışmaya devam eder)."
        checked={draft.autoTagSessions}
        onChange={(v) => set('autoTagSessions', v)}
      />

      <SubHead icon={Bug}>Debug günlüğü (gözlemlenebilirlik)</SubHead>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        Her oturum için <code>session.jsonl</code>'in yanına paralel bir <code>debug.jsonl</code>{' '}
        akışı yazılır: tur süreleri, çağrı-başına token tüketimi, araç gecikme/boyut/hataları, hook
        kararları, sıkıştırma ve kurtarma olayları. Ajan bunu <code>read_session_debug</code>{' '}
        aracıyla okuyup kendini optimize edebilir; UI'da oturum detayında "Debug" kartı gösterir.
      </div>
      <Toggle
        label="Debug günlüğünü yaz"
        hint="Yapılandırılmış gözlemlenebilirlik olaylarını oturum-başına debug.jsonl'e ekler. Kapalıyken hiçbir debug olayı yazılmaz."
        checked={draft.debugJournalEnabled}
        onChange={(v) => set('debugJournalEnabled', v)}
      />
      <Field
        label="Olay limiti"
        hint="Oturum başına saklanan en yeni debug olayı sayısı; aşıldığında en eskiler budanır (0 = varsayılan 5000)."
      >
        <input
          type="number"
          min={0}
          value={draft.debugJournalCap}
          onChange={(e) => set('debugJournalCap', Number(e.target.value))}
          className="w-28 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm text-[var(--color-text)]"
        />
      </Field>

      <SubHead icon={LifeBuoy}>Tur kurtarma & sıkıştırma</SubHead>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        <span className="font-medium text-[var(--color-text)]">Tur kurtarma (A1).</span> Ajanın araç
        döngüsü "mutlu yol" dışına çıktığında turu yapısal olarak kurtarır: modelin cevabı
        çıktı-token limitine takılırsa kaldığı yerden <em>sürdürür</em> (parçalar tek cevapta
        birleştirilir), bağlam penceresi taşarsa eski mesajları özetleyip turu{' '}
        <em>yeniden dener</em>. Her kurtarma tek-atımlıktır, sonsuz döngü olmaz. Yalnız native
        (Anthropic/MiniMax) yolunda etkilidir; claude-cli kendi döngüsünü sürdürür.
      </div>
      <Toggle
        label="Reaktif sıkıştırma"
        hint="Bağlam taşması hatasında eski tur geçmişi özetlenip tur yeniden denenir. Kapalıyken taşma turu sonlandırır."
        checked={draft.reactiveCompact}
        onChange={(v) => set('reactiveCompact', v)}
      />
      <div className="grid grid-cols-2 gap-3">
        <Field
          label="Maks. token resume denemesi"
          hint="Çıktı limiti aşılınca tur kaç kez sürdürülür (0 = kapalı; kısmi cevap olduğu gibi gösterilir)."
        >
          <input
            type="number"
            value={draft.maxTokenRetries}
            onChange={(e) => set('maxTokenRetries', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Sıkıştırmada korunan mesaj"
          hint="Reaktif sıkıştırmada aynen tutulan en yeni mesaj sayısı (≥2)."
        >
          <input
            type="number"
            value={draft.reactiveKeepRecent}
            onChange={(e) => set('reactiveKeepRecent', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Çıktı token tavanı"
          hint="Tur başına maks. çıktı tokeni (max_tokens). 0 = otomatik: modele göre aile-bazlı (opus/sonnet/fable+minimax 32K, haiku 16K, deepseek/gemini 8K). Pozitif değer tüm modeller için sabit tavanı zorlar. Düşük tavan resume döngüsünü daha sık tetikler."
        >
          <input
            type="number"
            value={draft.maxOutputTokens}
            onChange={(e) => set('maxOutputTokens', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Sağlayıcı retry bütçesi"
          hint="Geçici sağlayıcı hatasında (429 / 5xx / zaman aşımı) tur içinde kaç kez jitter'lı backoff'la yeniden denenir (0 = kapalı, maks 5). Kalıcı hatalar (auth/kota) asla yeniden denenmez."
        >
          <input
            type="number"
            min={0}
            max={5}
            value={draft.maxProviderRetries}
            onChange={(e) => set('maxProviderRetries', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
      </div>

      {/* Self-healing (döngü koruması & ders çıkarma) moved to İçgörü ▸ Dersler. */}
    </>
  )
}
