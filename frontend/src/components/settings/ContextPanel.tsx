import { Layers, Database, NotebookPen, LifeBuoy, Scissors, Sparkles, FlaskConical, RotateCcw, ListChecks, Bug } from 'lucide-react'
import { Field, Toggle, Slider, inputCls } from './primitives'
import { SubHead } from './settingsPanelShared'
import type { PanelProps } from './settingsPanelShared'

export function ContextPanel({ draft, set }: PanelProps) {
  return (
    <>
      <SubHead icon={Layers}>Bağlam penceresi</SubHead>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Maks. bağlam token" hint="Sıkıştırma için taban değer. Aşılınca eski turlar özetlenir."><input type="number" value={draft.maxContextTokens} onChange={(e) => set('maxContextTokens', Number(e.target.value))} className={inputCls} /></Field>
        <Field label="Korunan son mesaj" hint="Her zaman aynen gönderilir."><input type="number" value={draft.keepRecentMsgs} onChange={(e) => set('keepRecentMsgs', Number(e.target.value))} className={inputCls} /></Field>
        <Field label="Bütçe tavanı (token)" hint="Büyük pencereli (1M) modeller için üst sınır. Varsayılan 262144 (256K) — ham pencereyi gradyanın yüksek-hassasiyet bölgesinde tutar (context-rot). Yükseltmek daha çok ham geçmiş tutar ama recall hassasiyetiyle takas eder."><input type="number" value={draft.contextBudgetCeil} onChange={(e) => set('contextBudgetCeil', Number(e.target.value))} className={inputCls} /></Field>
        <Field label="Pencere oranı" hint="Transkripte ayrılan pay (0–1). 0 = otomatik (model-ailesine göre adaptif, önerilen). Pozitif değer sabit pay sabitler (ör. 0.45 → 1M model 450K, tavana kırpılır)."><input type="number" step="0.05" value={draft.contextBudgetFraction} onChange={(e) => set('contextBudgetFraction', Number(e.target.value))} className={inputCls} /></Field>
      </div>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">Etkin bütçe = clamp(pencere × oran, maks. token, tavan). Oran 0 = otomatik adaptif; bilinmeyen pencere → maks. token kullanılır.</p>

      <SubHead icon={FlaskConical}>Anthropic beta</SubHead>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">Yalnız anthropic sağlayıcıda etkili; claude-cli'da etkisizdir. (1M bağlam artık GA — ayar gerekmez.)</p>
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
        <Field label="Günlük min. uzunluk" hint="Bu karakter sayısından kısa turlar (ör. tek kelime/sayı cevaplar) journal'a yazılmaz, recall'ı kirletmez; 0 = kapalı."><input type="number" value={draft.journalMinLen} onChange={(e) => set('journalMinLen', Number(e.target.value))} className={inputCls} /></Field>
        <Field label="Yansıma limiti" hint="Ajan başına saklanan en yeni reflection sayısı; her dream cycle'da eskiler budanır (sınırsız birikmeyi önler)."><input type="number" value={draft.reflectionCap} onChange={(e) => set('reflectionCap', Number(e.target.value))} className={inputCls} /></Field>
        <Field label="Otomatik yansıma eşiği" hint="Journal sayısı bunu aşınca dream cycle kendiliğinden tetiklenir."><input type="number" value={draft.autoReflectThreshold} onChange={(e) => set('autoReflectThreshold', Number(e.target.value))} className={inputCls} /></Field>
      </div>
      <Toggle label="Otomatik yansıma (dream cycle)" hint="Journal eşiği aşılınca ajan kendi günlüğünü arka planda özetler ve özetlenen kayıtları siler. Otonom çağrı sayılır: duraklatma ve günlük bütçeye saygı gösterir." checked={draft.autoReflect} onChange={(v) => set('autoReflect', v)} />
      <Toggle label="Otomatik kullanıcı modelleme (HA-1)" hint="Dream cycle sırasında ajan, journal'dan kullanıcı hakkındaki kalıcı bilgileri çıkarıp 'human' çekirdek belleğini günceller. Yansıma açıkken çalışır." checked={draft.autoUserModel} onChange={(v) => set('autoUserModel', v)} />

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

      <SubHead icon={RotateCcw}>Context reset (handoff)</SubHead>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        Anthropic "harness design" deseni: uzun otonom görevlerde yerinde sıkıştırma tek başına "context anxiety"yi (modelin limite yaklaşınca işi erken
        toparlaması) çözmez. Açıkken, bağlam sınırına çarpan otonom bir tur özetlenmek yerine bir <span className="font-medium text-[var(--color-text)]">handoff
        dosyası</span> yazar ve işi <span className="font-medium text-[var(--color-text)]">temiz bir pencerede</span> sürdürmek için yeni bir oturum başlatır.
        Manuel <code>/handoff</code> komutu ve <code>handoff_session</code> aracı bu ayardan bağımsız her zaman çalışır.
      </div>
      <Toggle
        label="Otomatik context reset"
        hint="Bağlam sınırına çarpan (reactive compaction tetikleyen) otonom tur, handoff yazıp temiz oturumda devam eder. Yalnız otonom turlar; manuel sohbet etkilenmez."
        checked={draft.handoffAuto}
        onChange={(v) => set('handoffAuto', v)}
      />
      <Slider
        label="Otomatik reset basınç eşiği"
        min={0.5}
        max={0.99}
        step={0.01}
        value={draft.handoffPressure || 0.9}
        onChange={(v) => set('handoffPressure', v)}
        badge={`%${Math.round((draft.handoffPressure || 0.9) * 100)}`}
        hint="Otomatik reset yalnız bağlam doluluğu bu oranın üstündeyken yapılır (bellek-basıncı uyarısının üstünde tutun)."
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
        hint="Artifact'ın yanı sıra çalışma dizinine <workdir>/.swarmgo/handoff.md olarak yazar (disk üstü progress dosyası deseni)."
        checked={draft.handoffWriteFile}
        onChange={(v) => set('handoffWriteFile', v)}
      />

      <SubHead icon={ListChecks}>Kalıcı ilerleme (progress)</SubHead>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        Anthropic'in <span className="font-medium text-[var(--color-text)]">claude-progress</span> konvansiyonu: <code>todo_write</code> kontrol listesi proje çalışma dizinine
        <code> &lt;cwd&gt;/.swarmgo/progress.json</code> olarak yazılır (cwd yoksa ajan-başına depo dosyasına). Böylece liste oturumlar arası kaybolmaz; yeni bir oturum
        açıldığında kaldığı yerden devralınır. Dosya git-commit'lenebilir ve ajan dosya araçlarıyla okunabilir.
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

      <SubHead icon={Bug}>Debug günlüğü (gözlemlenebilirlik)</SubHead>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        Her oturum için <code>session.jsonl</code>'in yanına paralel bir <code>debug.jsonl</code> akışı yazılır: tur süreleri, çağrı-başına token tüketimi, araç gecikme/boyut/hataları,
        hook kararları, sıkıştırma ve kurtarma olayları. Ajan bunu <code>read_session_debug</code> aracıyla okuyup kendini optimize edebilir; UI'da oturum detayında "Debug" kartı gösterir.
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
        <span className="font-medium text-[var(--color-text)]">Tur kurtarma (A1).</span> Ajanın araç döngüsü "mutlu yol" dışına çıktığında turu yapısal olarak kurtarır: modelin
        cevabı çıktı-token limitine takılırsa kaldığı yerden <em>sürdürür</em> (parçalar tek cevapta birleştirilir), bağlam penceresi taşarsa eski mesajları
        özetleyip turu <em>yeniden dener</em>. Her kurtarma tek-atımlıktır, sonsuz döngü olmaz. Yalnız native (Anthropic/MiniMax) yolunda etkilidir; claude-cli kendi döngüsünü sürdürür.
      </div>
      <Toggle label="Reaktif sıkıştırma" hint="Bağlam taşması hatasında eski tur geçmişi özetlenip tur yeniden denenir. Kapalıyken taşma turu sonlandırır." checked={draft.reactiveCompact} onChange={(v) => set('reactiveCompact', v)} />
      <div className="grid grid-cols-2 gap-3">
        <Field label="Maks. token resume denemesi" hint="Çıktı limiti aşılınca tur kaç kez sürdürülür (0 = kapalı; kısmi cevap olduğu gibi gösterilir)."><input type="number" value={draft.maxTokenRetries} onChange={(e) => set('maxTokenRetries', Number(e.target.value))} className={inputCls} /></Field>
        <Field label="Sıkıştırmada korunan mesaj" hint="Reaktif sıkıştırmada aynen tutulan en yeni mesaj sayısı (≥2)."><input type="number" value={draft.reactiveKeepRecent} onChange={(e) => set('reactiveKeepRecent', Number(e.target.value))} className={inputCls} /></Field>
        <Field label="Çıktı token tavanı" hint="Tur başına maks. çıktı tokeni (max_tokens). 0 = otomatik: modele göre aile-bazlı (opus/sonnet+minimax 32K, haiku/fable 16K, deepseek/gemini 8K). Pozitif değer tüm modeller için sabit tavanı zorlar. Düşük tavan resume döngüsünü daha sık tetikler."><input type="number" value={draft.maxOutputTokens} onChange={(e) => set('maxOutputTokens', Number(e.target.value))} className={inputCls} /></Field>
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
