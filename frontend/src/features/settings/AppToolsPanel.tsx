import { Sparkles } from 'lucide-react'
import { Field, Toggle, Segmented, inputCls } from './primitives'
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
        <div className="rounded-lg border border-[var(--color-warning,#f59e0b)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
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
        <Field
          label="Maks. delegasyon derinliği"
          hint="Zincirin kaç kat iç içe gidebileceği (1–10). Döngü koruması."
        >
          <input
            type="number"
            min={1}
            max={10}
            value={draft.delegationMaxDepth}
            onChange={(e) => set('delegationMaxDepth', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Tur başına maks. delegasyon"
          hint="Tek kullanıcı turunda toplam run_subagent çağrısı (1–100). Bütçe koruması."
        >
          <input
            type="number"
            min={1}
            max={100}
            value={draft.delegationMaxCalls}
            onChange={(e) => set('delegationMaxCalls', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
      </div>

      <SubHead icon={Sparkles}>Spawn (arka plan) limitleri</SubHead>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">
        Ayrık arka plan yüzeyi için sınırlar: <code>run_subagent</code> (async) ve köprülenen{' '}
        <code>spawn_session</code> + UI spawn düğmesi.
      </p>
      <div className="grid grid-cols-2 gap-3">
        <Field
          label="Maks. eşzamanlı spawn"
          hint="Aynı anda çalışabilen spawn edilmiş oturum sayısı (1–128)."
        >
          <input
            type="number"
            min={1}
            max={128}
            value={draft.spawnMaxConcurrent}
            onChange={(e) => set('spawnMaxConcurrent', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Tur başına maks. spawn"
          hint="Tek ajan turunda başlatılabilecek spawn sayısı (1–64)."
        >
          <input
            type="number"
            min={1}
            max={64}
            value={draft.spawnMaxPerTurn}
            onChange={(e) => set('spawnMaxPerTurn', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Spawn süresi — üst sınır (dk)"
          hint="Bir spawn/worker iş turunun MUTLAK süre tavanı; otomatik-devam turları da bu süreyi paylaşır (varsayılan 20)."
        >
          <input
            type="number"
            min={1}
            max={1440}
            value={draft.spawnTimeoutMin}
            onChange={(e) => set('spawnTimeoutMin', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Spawn boşta süresi (dk)"
          hint="Etkinlik izleyicisi: bir spawn/worker turu bu kadar süre hiçbir adım (araç/düşünce/token) yaymazsa 'asılı' sayılıp iptal edilir; üretken uzun tur üst sınıra kadar koşar (varsayılan 5)."
        >
          <input
            type="number"
            min={1}
            max={1440}
            value={draft.spawnIdleTimeoutMin}
            onChange={(e) => set('spawnIdleTimeoutMin', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Boşta yeniden başlatma (adet)"
          hint="Boşta izleyicisi bir arka-plan turunu kesince, kaldığı yerden sürmek için taze bir boşta penceresinde kaç kez otomatik yeniden başlatılacağı (varsayılan 1; 0 = kapalı). Sert süre tavanı yeniden başlatılmaz."
        >
          <input
            type="number"
            min={0}
            max={5}
            value={draft.idleResumeMax}
            onChange={(e) => set('idleResumeMax', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Zamanlama süresi (dk)"
          hint="Bir zamanlanmış tetiğin (cron görev/prompt + schedule_wake) ve elle 'Şimdi çalıştır' koşusunun süre sınırı (varsayılan 60)."
        >
          <input
            type="number"
            min={1}
            max={1440}
            value={draft.scheduleTimeoutMin}
            onChange={(e) => set('scheduleTimeoutMin', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Tur izleyicisi (dk)"
          hint="Kuyruktaki bir turun MUTLAK tavanı: bu süreyi aşan tur zorla iptal edilir ki oturum kuyruğu asılı bir turun arkasında tıkanmasın (varsayılan 120). Tıkanma freni olduğu için spawn/zamanlama sürelerinin ALTINA inemez — daha küçük girilirse otomatik yükseltilir."
        >
          <input
            type="number"
            min={1}
            max={1440}
            value={draft.turnWatchdogMin}
            onChange={(e) => set('turnWatchdogMin', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Tur boşta süresi (dk)"
          hint="Kuyruktaki bir tur bu kadar süre hiçbir etkinlik (araç adımı, düşünce, token) üretmezse asılı sayılıp iptal edilir; üretken tur üst sınıra kadar koşar (varsayılan 20). Üst sınırın üstüne çıkamaz."
        >
          <input
            type="number"
            min={1}
            max={1440}
            value={draft.turnIdleWatchdogMin}
            onChange={(e) => set('turnIdleWatchdogMin', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
      </div>

      <SubHead icon={Sparkles}>Araç çalıştırma</SubHead>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">
        Süreç-geneli araç davranışı: kabuk (Bash/PowerShell) komut süreleri ve bir aracın modele
        dönmeden önceki çıktı üst sınırı. Tek çağrıdaki <code>timeout_sec</code> argümanı
        varsayılanı geçersiz kılar (maks. ile kırpılır).
      </p>
      <div className="grid grid-cols-3 gap-3">
        <Field
          label="Kabuk varsayılan süre (sn)"
          hint="timeout_sec verilmezse kullanılan Bash/PowerShell süresi (varsayılan 30)."
        >
          <input
            type="number"
            min={1}
            max={3600}
            value={draft.shellDefaultTimeoutSec}
            onChange={(e) => set('shellDefaultTimeoutSec', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Kabuk maks. süre (sn)"
          hint="Bir komutun üst sınırı; timeout_sec bunu aşamaz (varsayılan 120)."
        >
          <input
            type="number"
            min={1}
            max={3600}
            value={draft.shellMaxTimeoutSec}
            onChange={(e) => set('shellMaxTimeoutSec', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Araç çıktı sınırı (KB)"
          hint="Bir aracın çıktısı bu boyutu aşarsa kesilir (MCP araçları dahil backstop; varsayılan 100)."
        >
          <input
            type="number"
            min={1}
            max={4096}
            value={draft.maxToolOutputKB}
            onChange={(e) => set('maxToolOutputKB', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
      </div>

      <SubHead icon={Sparkles}>Koordinatör (çoklu-ajan) limitleri</SubHead>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">
        M2 koordinatör/worker döngüsü için sınırlar: bir koordinatör aynı anda kaç worker
        çalıştırabilir ve koordinatör ağacı ne kadar derinleşebilir. Otomatik tur sayısı ve ağaç
        başına toplam worker oturumu <strong>sınırsızdır</strong> (limit kaldırıldı).
      </p>
      <div className="grid grid-cols-2 gap-3">
        <Field
          label="Koordinatör başına maks. worker"
          hint="Bir koordinatörün aynı anda çalıştırabileceği aktif worker sayısı (1–64)."
        >
          <input
            type="number"
            min={1}
            max={64}
            value={draft.coordinatorMaxWorkers}
            onChange={(e) => set('coordinatorMaxWorkers', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Maks. koordinatör derinliği"
          hint="Koordinatör ağacının kaç seviye derinleşebileceği (kök = 0). Bir alt-koordinatör ancak kendi worker'larına yer kalıyorsa açılabilir. -1 = sınırsız."
        >
          <input
            type="number"
            min={-1}
            max={12}
            value={draft.coordinatorMaxDepth}
            onChange={(e) => set('coordinatorMaxDepth', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Rapor backstop bekleme (sn)"
          hint="Bir alt-koordinatörün dalı tamamen sustuktan sonra, kendisi rapor vermezse otomatik 'incomplete' raporu gönderilene kadar beklenen süre. Kısa olursa yavaş modelin sentez turu yarışı kaybeder ve gereksiz 'incomplete' gider; uzun olursa takılmış bir dal üstündeki koordinatörü bekletir (5–1800)."
        >
          <input
            type="number"
            min={5}
            max={1800}
            value={draft.coordinatorSettleGraceSec}
            onChange={(e) => set('coordinatorSettleGraceSec', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
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
        <Field
          label="Gecikme tarayıcı penceresi (dk)"
          hint="Bir koordinatör bu kadar dakika sessiz kalıp hiç çalışan worker'ı yoksa tarayıcı yargıca sorar. 0 = varsayılan (5 dk). -1 = tarayıcıyı kapat (tur-sonu guard'ı yine çalışır)."
        >
          <input
            type="number"
            min={-1}
            max={1440}
            value={draft.coordinatorStallSweepMin}
            onChange={(e) => set('coordinatorStallSweepMin', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Maks. ardışık uyarı"
          hint="Aynı koordinatöre peş peşe kaç düzeltici not enjekte edilebileceği. Bu sınıra ulaşınca sistem uyarıp gözlemlenebilir bir hata bırakır (sonsuza dek dırdır etmez). 0 = varsayılan (2)."
        >
          <input
            type="number"
            min={0}
            max={10}
            value={draft.coordinatorStallMaxNudges}
            onChange={(e) => set('coordinatorStallMaxNudges', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
      </div>

      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
        <b>Çalışma dizini güvenliği.</b> Dosya/kabuk araçları artık workspace'e kilitli değil (her
        yola erişebilir). Aşağıdaki frenler bu gücü <b>otonom</b>
        (zamanlama/spawn/flow — insan döngüde değil) turlarda dengeler. İnteraktif sohbet
        etkilenmez.
      </div>
      <Toggle
        label="Otonom turları çalışma dizinine kilitle"
        hint="Açıkken zamanlama/spawn/flow ile çalışan ajanların dosya araçları yalnızca oturumun çalışma dizininde kalır (mutlak yol + `..` kaçışı reddedilir) ve `git push` engellenir. Önerilen: AÇIK."
        checked={draft.autonomousConfine}
        onChange={(v) => set('autonomousConfine', v)}
      />
      <Toggle
        label="Otonom boot doğrulama sırası"
        hint="Açıkken zamanlama/spawn/flow/subagent turlarına kısa bir açılış sırası hatırlatıcısı enjekte edilir (yönelim → hatırlama → tek görev seç → temel testi doğrula → işi yap → döngüyü kapat). Tam reçete: tionharness-autonomous-ops becerisi (§10). Otonom tur başına birkaç token; kapatınca geri kazanılır. Önerilen: AÇIK."
        checked={draft.autonomousBootSeq}
        onChange={(v) => set('autonomousBootSeq', v)}
      />
    </>
  )
}
