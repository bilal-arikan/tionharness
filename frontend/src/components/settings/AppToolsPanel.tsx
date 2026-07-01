import { Sparkles } from 'lucide-react'
import { Field, Toggle, inputCls } from './primitives'
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
        Bu yetenekler varsayılan olarak <b>kapalıdır</b>: her biri ajanların gücünü ve
        token maliyetini artırır. Değişiklikler tüm workspace'lere canlı uygulanır.
      </div>
      <Toggle
        label="Kabuk (Bash) aracı"
        hint="Built-in `Bash`: ajan komut çalıştırır (Windows'ta PowerShell). claude-cli ajanlarında bu araç köprülenir ve CLI'nin native `Bash`'i bastırılır — tüm komutlar SwarmGo kabuğundan geçer. Yüksek risk."
        checked={draft.enableShell}
        onChange={(v) => set('enableShell', v)}
      />
      {/* Öz-yönetim araç paketi toggle'ı kaldırıldı: paket artık daima kurulu;
          görünürlük araç bazında "Araçlar" ekranından (Tam/Özet/İsim/Gizli) yönetilir. */}
      <Toggle
        label="Hook'ları claude-cli'ye geçir"
        hint="Açıkken workspace PreToolUse/PostToolUse hook'ları claude-cli ajanlarına da `--settings` ile uygulanır (yalnız native değil). Uyarı: CLI hook'ları CLI'nin kendi shell'inde koşar; SwarmGo shell'i (PowerShell) için yazılmış bir hook uyumsuz olabilir — sorun çıkarsa kapatın."
        checked={draft.enableCliHooks}
        onChange={(v) => set('enableCliHooks', v)}
      />
      <Toggle
        label="claude-cli oturum sürekliliği (--resume)"
        hint="Varsayılan açık. Her turda --resume ile önceki oturumu sürdürür ve yalnız yeni mesajı gönderir — CLI'nin sıcak prompt cache'ini tekrar kullanır (çok daha ucuz). Etkili olmasının sebebi: sistem promptu artık sabit (değişken bağlam mesaj kuyruğuna taşındı), böylece cache'li önek turdan tura bozulmaz. Yalnız tek-ajanlı sohbetlerde."
        checked={draft.claudeResume}
        onChange={(v) => set('claudeResume', v)}
      />
      <Toggle
        label="claude-cli kalıcı süreç (deneysel)"
        hint="Açıkken oturum başına TEK uzun-ömürlü claude süreci canlı tutulur ve turlar stdin'den beslenir (her tur yeni süreç açılmaz); sıcak turda yalnız yeni kullanıcı mesajı gider, süreç gerisini hatırlar. Açıkken --resume'un yerine geçer. Cache ısınması TTL'e bağlıdır. Deneysel — varsayılan kapalı; açmadan önce bir sohbette doğrulayın."
        checked={draft.claudePersistentSession}
        onChange={(v) => set('claudePersistentSession', v)}
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
      <Toggle
        label="Otonom boot doğrulama sırası"
        hint="Açıkken zamanlama/spawn/flow/subagent turlarına kısa bir açılış sırası hatırlatıcısı enjekte edilir (yönelim → hatırlama → tek görev seç → temel testi doğrula → işi yap → döngüyü kapat). Tam reçete: swarmgo-autonomous-ops becerisi (§10). Otonom tur başına birkaç token; kapatınca geri kazanılır. Önerilen: AÇIK."
        checked={draft.autonomousBootSeq}
        onChange={(v) => set('autonomousBootSeq', v)}
      />
    </>
  )
}
