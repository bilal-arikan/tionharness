import { Sparkles } from 'lucide-react'
import { NumberField, Toggle, Segmented } from './primitives'
import { SubHead } from './settingsPanelShared'
import type { PanelProps } from './settingsPanelShared'

// ToolsPanel is the app-global transversal-capabilities category (shell, cli
// hooks, delegation, spawn/autonomy safety brakes). Exported as `ToolsPanel` for
// backwards compatibility; the file is named AppToolsPanel to avoid confusion
// with the workspace-level panels/ToolsPanel.
export function ToolsPanel({ draft, set }: PanelProps) {
  return (
    <>
      <SubHead icon={Sparkles}>Geçişli yetenekler</SubHead>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
        Bu yetenekler varsayılan olarak <b>kapalıdır</b>: her biri ajanların gücünü ve token
        maliyetini artırır. Değişiklikler tüm workspace'lere canlı uygulanır.
      </div>
      <Toggle
        label="Kabuk (Bash) aracı"
        hint="Built-in `Bash`: ajan komut çalıştırır (Windows'ta PowerShell). claude-cli ajanlarında bu araç köprülenir ve CLI'nin native `Bash`'i bastırılır — tüm komutlar TionHarness kabuğundan geçer. Yüksek risk."
        checked={draft.enableShell}
        onChange={(v) => set('enableShell', v)}
      />
      {/* Öz-yönetim araç paketi toggle'ı kaldırıldı: paket artık daima kurulu;
          görünürlük araç bazında "Araçlar" ekranından (Tam/Özet/İsim/Gizli) yönetilir. */}
      <Toggle
        label="Kod-modu (run_code + MCP binding'leri)"
        hint="Code execution with MCP (_Docs/44): MCP araçları şema yerine üretilmiş Python modülleri olarak sunulur; ajan araçları kod yazarak çağırır, ara veriler bağlama girmez. Kabuk yetkisi de açık olmalı (run_code keyfi kod çalıştırır). Yalnız native tool-loop yolunda; script içi MCP çağrıları izin modundan geçer."
        checked={draft.enableCodeMode}
        onChange={(v) => set('enableCodeMode', v)}
      />
      {draft.enableCodeMode && !draft.enableShell && (
        <div className="rounded-lg border border-[var(--color-warning)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
          ⚠️ <b>Kod-modu kabuk yetkisi olmadan etkisizdir.</b> <code>run_code</code> aracı yalnız
          "Kabuk (Bash) aracı" da açıkken kaydedilir — yukarıdaki toggle'ı da açın.
        </div>
      )}
      <Toggle
        label="Hook'ları claude-cli'ye geçir"
        hint="Açıkken workspace PreToolUse/PostToolUse hook'ları claude-cli ajanlarına da `--settings` ile uygulanır (yalnız native değil). Uyarı: CLI hook'ları CLI'nin kendi shell'inde koşar; TionHarness shell'i (PowerShell) için yazılmış bir hook uyumsuz olabilir — sorun çıkarsa kapatın."
        checked={draft.enableCliHooks}
        onChange={(v) => set('enableCliHooks', v)}
      />
      <Segmented
        label="claude-cli cache/oturum modu"
        value={draft.claudePersistentSession ? 'persistent' : draft.claudeResume ? 'resume' : 'off'}
        onChange={(mode) => {
          // Two mutually-exclusive booleans drive the runtime (toolloop.go picks
          // persistent when on; chat_resume.go gates --resume only when persistent is
          // off). Map each segment to a deterministic pair so exactly one path is live.
          set('claudePersistentSession', mode === 'persistent')
          set('claudeResume', mode === 'persistent' || mode === 'resume')
        }}
        options={[
          {
            value: 'persistent',
            label: 'Kalıcı süreç',
            hint: "Oturum başına TEK uzun-ömürlü claude süreci canlı tutulur; turlar stdin'den beslenir (her tur yeni süreç açılmaz), sıcak turda yalnız yeni mesaj gider. En düşük cache-write + ~%6–7 daha hızlı warm tur (ölçüm: _Docs/50). Cache ısınması TTL'e bağlı. TionHarness'e özgü — External Agent'ta yoktur.",
          },
          {
            value: 'resume',
            label: '--resume (delta)',
            hint: "Her tur yeni süreç açılır ama --resume ile önceki oturum sürdürülür; yalnız yeni mesaj (delta) gönderilir → CLI'nin sıcak server-side cache'i tekrar kullanılır. En düşük input. External Agent de tam olarak bu modu kullanır. Yalnız tek-ajanlı sohbetlerde.",
          },
          {
            value: 'off',
            label: 'Kapalı',
            hint: 'Ne kalıcı süreç ne --resume: her tur yeni süreç + TAM transcript gönderilir. En pahalı; yalnız hata ayıklama/karşılaştırma için.',
          },
        ]}
      />
      <Toggle
        label="claude-cli sistem promptunu dosyayla ekle (--append-system-prompt-file)"
        hint="Kapalı (varsayılan): sistem promptu doğrudan komut satırında --append-system-prompt <metin> ile geçer — daha basit, geçici dosya bırakmaz. Açık: geçici bir dosyaya yazılıp --append-system-prompt-file <yol> ile verilir. Dosya modu, çok büyük sistem promptlarında Windows'un ~32 KB komut satırı limitini (errno 206) aşmayı önler; prompt'unuz çok büyükse ve doğrudan modda süreç başlamıyorsa bunu açın."
        checked={draft.claudeSysPromptFile}
        onChange={(v) => set('claudeSysPromptFile', v)}
      />
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
        <b>Ajan→ajan delegasyon (run_subagent).</b> Bu araç artık daima kuruludur; açıp kapatmayı
        ajan bazında <b>Araçlar</b> ekranından yaparsınız. Aşağıdaki değerler her delegasyon
        çağrısında geçerli olan güvenlik/bütçe frenleridir. (Yalnızca native/anthropic tool yolunda
        çalışır.)
      </div>
      <div className="grid grid-cols-2 gap-3">
        <NumberField
          label="Maks. delegasyon derinliği"
          hint="Zincirin kaç kat iç içe gidebileceği (1–10). Döngü koruması."
          min={1}
          max={10}
          value={draft.delegationMaxDepth}
          onChange={(v) => set('delegationMaxDepth', v)}
        />
        <NumberField
          label="Tur başına maks. delegasyon"
          hint="Tek kullanıcı turunda toplam run_subagent çağrısı (1–100). Bütçe koruması."
          min={1}
          max={100}
          value={draft.delegationMaxCalls}
          onChange={(v) => set('delegationMaxCalls', v)}
        />
      </div>

      <SubHead icon={Sparkles}>Spawn (arka plan) limitleri</SubHead>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">
        Ayrık arka plan yüzeyi için sınırlar: <code>spawn_worker</code> ve köprülenen{' '}
        <code>spawn_session</code> + UI spawn düğmesi.
      </p>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">
        Eski <code>spawnTimeoutMin</code> ve <code>scheduleTimeoutMin</code> alanları yalnız wire ve
        storage uyumluluğu için korunur; deprecated ve etkisizdir (<code>0 = disabled</code>).
        Üretken işler toplam süreyle kesilmez. Spawn, worker ve zamanlanmış koşular aşağıdaki
        semantic boşta penceresiyle korunur.
      </p>
      <div className="grid grid-cols-2 gap-3">
        <NumberField
          label="Maks. eşzamanlı spawn"
          hint="Aynı anda çalışabilen spawn edilmiş oturum sayısı (1–128)."
          min={1}
          max={128}
          value={draft.spawnMaxConcurrent}
          onChange={(v) => set('spawnMaxConcurrent', v)}
        />
        <NumberField
          label="Maks. bekleyen spawn"
          hint="Kapasite doluyken kuyrukta bekleyebilecek spawn sayısı (1–128)."
          min={1}
          max={128}
          value={draft.spawnQueueMax}
          onChange={(v) => set('spawnQueueMax', v)}
        />
        <NumberField
          label="Tur başına maks. spawn"
          hint="Tek ajan turunda başlatılabilecek spawn sayısı (1–64)."
          min={1}
          max={64}
          value={draft.spawnMaxPerTurn}
          onChange={(v) => set('spawnMaxPerTurn', v)}
        />
        <NumberField
          label="Spawn boşta süresi (dk)"
          hint="Etkinlik izleyicisi: spawn, worker veya zamanlanmış koşu bu kadar süre anlamlı ilerleme üretmezse 'asılı' sayılıp iptal edilir; üretken uzun işlerin toplam süre tavanı yoktur (varsayılan 5)."
          min={1}
          max={1440}
          value={draft.spawnIdleTimeoutMin}
          onChange={(v) => set('spawnIdleTimeoutMin', v)}
        />
        <NumberField
          label="Sohbet turu boşta süresi (dk)"
          hint="İnteraktif sohbet turu bu kadar süre gerçek bir adım üretmezse iptal edilir; takılan bir sağlayıcı akışı böyle geri alınır. HTTP/SSE pingleri etkinlik sayılmaz (varsayılan 3; 0 = kapalı)."
          min={0}
          max={1440}
          value={draft.chatTurnIdleTimeoutMin}
          onChange={(v) => set('chatTurnIdleTimeoutMin', v)}
        />
        <NumberField
          label="Codex stdout sessizlik penceresi (sn)"
          hint="Akışa başlamış bir codex-cli alt süreci bu kadar saniye hiç çıktı üretmezse süreç ağacı öldürülür ve tur 'takıldı' olarak raporlanır (stdout kuyruğu debug.jsonl'e yazılır). Sohbet boşta süresinden küçük tutun (varsayılan 90; 0 = kapalı)."
          min={0}
          max={86400}
          value={draft.codexStdoutIdleSec}
          onChange={(v) => set('codexStdoutIdleSec', v)}
        />
        <NumberField
          label="Boşta yeniden başlatma (adet)"
          hint="Boşta izleyicisi bir arka-plan turunu kesince, kaldığı yerden sürmek için taze bir boşta penceresinde kaç kez otomatik yeniden başlatılacağı (varsayılan 1; 0 = kapalı)."
          min={0}
          max={5}
          value={draft.idleResumeMax}
          onChange={(v) => set('idleResumeMax', v)}
        />
        <NumberField
          label="Tur boşta süresi (dk)"
          hint="Kuyruktaki aynı run bu kadar süre anlamlı ilerleme üretmezse asılı sayılıp iptal edilir. Heartbeat, boş/tekrar delta ve başka run event'i süreyi yenilemez (varsayılan 20)."
          min={1}
          max={1440}
          value={draft.turnIdleWatchdogMin}
          onChange={(v) => set('turnIdleWatchdogMin', v)}
        />
      </div>

      <SubHead icon={Sparkles}>Araç çalıştırma</SubHead>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">
        Süreç-geneli araç davranışı: kabuk (Bash/PowerShell) komut süreleri ve bir aracın modele
        dönmeden önceki çıktı üst sınırı. Tek çağrıdaki <code>timeout_sec</code> argümanı
        varsayılanı geçersiz kılar (maks. ile kırpılır).
      </p>
      <div className="grid grid-cols-3 gap-3">
        <NumberField
          label="Kabuk varsayılan süre (sn)"
          hint="timeout_sec verilmezse kullanılan Bash/PowerShell süresi (varsayılan 30)."
          min={1}
          max={3600}
          value={draft.shellDefaultTimeoutSec}
          onChange={(v) => set('shellDefaultTimeoutSec', v)}
        />
        <NumberField
          label="Kabuk maks. süre (sn)"
          hint="Bir komutun üst sınırı; timeout_sec bunu aşamaz (varsayılan 120)."
          min={1}
          max={3600}
          value={draft.shellMaxTimeoutSec}
          onChange={(v) => set('shellMaxTimeoutSec', v)}
        />
        <NumberField
          label="Araç çıktı sınırı (KB)"
          hint="Bir aracın çıktısı bu boyutu aşarsa kesilir (MCP araçları dahil backstop; varsayılan 100)."
          min={1}
          max={4096}
          value={draft.maxToolOutputKB}
          onChange={(v) => set('maxToolOutputKB', v)}
        />
      </div>

      <SubHead icon={Sparkles}>Koordinatör (çoklu-ajan) limitleri</SubHead>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">
        M2 koordinatör/worker döngüsü için sınırlar: bir koordinatör aynı anda kaç worker
        çalıştırabilir ve koordinatör ağacı ne kadar derinleşebilir. Otomatik tur sayısı ve ağaç
        başına toplam worker oturumu <strong>sınırsızdır</strong> (limit kaldırıldı).
      </p>
      <div className="grid grid-cols-2 gap-3">
        <NumberField
          label="Koordinatör başına maks. worker"
          hint="Bir koordinatörün aynı anda çalıştırabileceği aktif worker sayısı (1–64)."
          min={1}
          max={64}
          value={draft.coordinatorMaxWorkers}
          onChange={(v) => set('coordinatorMaxWorkers', v)}
        />
        <NumberField
          label="Maks. koordinatör derinliği"
          hint="Koordinatör ağacının kaç seviye derinleşebileceği (kök = 0). Bir alt-koordinatör ancak kendi worker'larına yer kalıyorsa açılabilir. -1 = sınırsız."
          min={-1}
          max={12}
          value={draft.coordinatorMaxDepth}
          onChange={(v) => set('coordinatorMaxDepth', v)}
        />
        <NumberField
          label="Rapor backstop bekleme (sn)"
          hint="Bir alt-koordinatörün dalı tamamen sustuktan sonra, kendisi rapor vermezse otomatik 'incomplete' raporu gönderilene kadar beklenen süre. Kısa olursa yavaş modelin sentez turu yarışı kaybeder ve gereksiz 'incomplete' gider; uzun olursa takılmış bir dal üstündeki koordinatörü bekletir (5–1800)."
          min={5}
          max={1800}
          value={draft.coordinatorSettleGraceSec}
          onChange={(v) => set('coordinatorSettleGraceSec', v)}
        />
      </div>

      <SubHead icon={Sparkles}>Koordinatör donma koruması</SubHead>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">
        Bir koordinatör bazen "worker açtım / N kol açıldı" der ama gerçekte hiç `spawn_worker`
        çağırmaz ve olmayan worker'ları bekleyerek sonsuza dek donar. Bu koruma her koordinatör
        turundan sonra (ve periyodik bir tarayıcıyla) ucuz bir model yargıcına son mesajı sorar;
        fantom spawn tespit edilirse düzeltici not enjekte edip bir tur daha zorlar.
      </p>
      <Toggle
        label="Donma korumasını etkinleştir"
        hint="Açıkken yargıç-tabanlı tur-sonu guard'ı + gecikme tarayıcısı çalışır. Yargıç, oturumun başlık-modeli (yoksa koordinatörün kendi modeli) ile çağrılır ve yalnızca koordinatör boştayken (araç çağırmadan) tetiklenir. Önerilen: AÇIK."
        checked={draft.coordinatorStallGuard}
        onChange={(v) => set('coordinatorStallGuard', v)}
      />
      <div className="grid grid-cols-2 gap-3">
        <NumberField
          label="Gecikme tarayıcı penceresi (dk)"
          hint="Bir koordinatör bu kadar dakika sessiz kalıp hiç çalışan worker'ı yoksa tarayıcı yargıca sorar. 0 = varsayılan (5 dk). -1 = tarayıcıyı kapat (tur-sonu guard'ı yine çalışır)."
          min={-1}
          max={1440}
          value={draft.coordinatorStallSweepMin}
          onChange={(v) => set('coordinatorStallSweepMin', v)}
        />
        <NumberField
          label="Maks. ardışık uyarı"
          hint="Aynı koordinatöre peş peşe kaç düzeltici not enjekte edilebileceği. Bu sınıra ulaşınca sistem uyarıp gözlemlenebilir bir hata bırakır (sonsuza dek dırdır etmez). 0 = varsayılan (2)."
          min={0}
          max={10}
          value={draft.coordinatorStallMaxNudges}
          onChange={(v) => set('coordinatorStallMaxNudges', v)}
        />
      </div>

      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
        <b>Çalışma dizini güvenliği.</b> Dosya/kabuk araçları artık workspace'e kilitli değil (her
        yola erişebilir). Aşağıdaki frenler bu gücü <b>otonom</b>
        (zamanlama/spawn/flow — insan döngüde değil) turlarda dengeler. İnteraktif sohbet
        etkilenmez.
      </div>
      <Toggle
        label="Otonom turlarda dosya araçlarını çalışma dizinine kilitle"
        hint="Açıkken zamanlama/spawn/flow ile çalışan ajanların DOSYA araçları yalnızca oturumun çalışma dizininde kalır (mutlak yol + `..` kaçışı reddedilir) ve `git push` engellenir. Dikkat: `shell` ve `powershell` bundan ETKİLENMEZ; kilitli bir turda bile dışarıya yazabilir. `transform_data` argüman yollarını kısıtlar, ancak script gövdesi rastgele ana makine kodu çalıştırarak bu kısıtı aşabilir. Önerilen: AÇIK."
        checked={draft.autonomousConfine}
        onChange={(v) => set('autonomousConfine', v)}
      />
      <Toggle
        label="Otonom boot doğrulama sırası"
        hint="Açıkken zamanlama/spawn/flow/subagent turlarına kısa bir açılış sırası hatırlatıcısı enjekte edilir (yönelim → hatırlama → tek görev seç → temel testi doğrula → işi yap → döngüyü kapat). Tam reçete: tionharness-autonomous-ops becerisi (§10). Otonom tur başına birkaç token; kapatınca geri kazanılır. Önerilen: AÇIK."
        checked={draft.autonomousBootSeq}
        onChange={(v) => set('autonomousBootSeq', v)}
      />
      <Toggle
        label="Otomatik devam (otonom turlarda kendi kendine tamamlama)"
        hint="Açıkken zamanlama/spawn/wake turu işi yarım bırakırsa (açık todo maddeleri veya son eylemi bir lazy-tool aktivasyonu) oturuma bir 'Otomatik devam' dürtmesi yazılır ve tur devam ettirilir. Elle durdurulan ya da bir tavana takılıp yarıda kesilen turlarda (süre/araç limiti/guardrail) bu dürtme hiç yazılmaz. Kapatınca mesaj hiç oluşmaz, yarım kalan otonom tur olduğu yerde durur. Önerilen: AÇIK."
        checked={draft.autonomousAutoContinue}
        onChange={(v) => set('autonomousAutoContinue', v)}
      />
      {draft.autonomousAutoContinue && (
        <NumberField
          label="Otomatik devam üst sınırı"
          hint="Bir otonom çalıştırmada en fazla kaç devam turu açılabilir. 0 = yerleşik varsayılan (3)."
          min={0}
          value={draft.autonomousAutoContinueMax}
          onChange={(v) => set('autonomousAutoContinueMax', v)}
        />
      )}
    </>
  )
}
