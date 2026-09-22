# 50 — Claude Code Cache/Context Paritesi Planı

> **Amaç:** TionHarness'in **native sağlayıcı** (anthropic / OpenAI-compat) context "paketini",
> Claude Code'un prompt-cache + compaction mekaniğine yaklaştırmak — **SDK'ya bağımlı
> olmadan** (çok-sağlayıcı, anahtarsız claude-cli, dosya-tabanlı felsefe korunur).
>
> Referanslar: Claude Code'un gözlemlenen prompt-cache ve compaction davranışı
> (cache breakpoint kırılması, otomatik/mikro compaction)
> ve `external-agent-oss` (context/cache'i **Claude Agent SDK / native `claude` binary'ye
> devrediyor** — kendi cache/compaction kodu yok). Bağlam: `_Docs\17-TOKEN-OPTIMIZASYON.md`.

## 0. Tespit — mevcut durum (kod doğrulandı 2026-07-04)

| Yol | Cache breakpoint'leri | Gerçek durum |
|-----|----------------------|--------------|
| **claude-cli** | `--resume` warm prefix (System + ilk N mesaj server-side) | **Zaten Claude Code-benzeri.** Dinamik bağlam bilinçli olarak **son kullanıcı mesajına** dokunuyor (`claudecli_session.go withDynamic(lastUserText…)`) → sıcak prefix bozulmuyor. |
| **anthropic** (native) | 3 breakpoint: son tool, statik System, **son mesajda rolling history** (`anthropic.go` `systemField`/`attachHistoryBreakpoint`), 1h TTL | Tools + statik System **HIT** alıyor. **Ama** volatile Dinamik, `systemField`'de statik System'in ardında (yani tools+mesajların ÖNÜNDE `system` alanında) → **history breakpoint her tur ISKALIYOR** (prefix dinamikte ayrışıyor). Rolling history breakpoint pratikte **ölü**. |
| **OpenAI-compat** (minimax/openrouter) | statik System breakpoint + transcript-tail breakpoint (`minimax.go buildSystemMessage`/`attachHistoryBreakpoint`) | Aynı sorun: dinamik system mesajının içinde → tail breakpoint ıskalıyor. Yalnız statik prefix HIT. |
| **Compaction** | `session.Summary` + `SummaryMsgCount`; özet `conversationSummaryBlock` ile **Dinamik'e** enjekte | Özet volatile bölgede → **hiç cache'lenmiyor**, her tur taze. Katlanan mesajlar düşürülüyor (doğru). |

**Kök neden:** Native yolda büyüyen kısım (mesaj geçmişi + özet) cache breakpoint'inin
gerisinde ama **önünde duran volatile Dinamik** onları her tur geçersizleştiriyor. Tools ve
statik System (küçük, sabit) HIT alıyor; asıl tasarruf potansiyeli olan **geçmiş** alamıyor.

## 1. Hedef mekanik (Claude Code deseni)

1. **System tamamen STATİK ve cache'li.** Volatile içerik system alanından çıkar.
2. **Volatile per-tur içerik, cache breakpoint'inin GERİSİNDE** — yalnız **uçuştaki (yeni)
   kullanıcı mesajına** eklenir, **persist edilmez**. Rolling breakpoint son **persist edilmiş**
   mesaja (önceki turun sonu) konur → cache öneki tamamen değişmez (immutable) → **HIT**.
   (Bu, claude-cli'nin zaten yaptığı desenin native'e genellenmesi.)
3. **Özet = cache'li mesaj akışının parçası** (compact boundary mesajı), volatile blok değil.
4. **Tek rolling history breakpoint** son stabil mesajda; anthropic ≤4 breakpoint limitinde
   (tools + system + history = 3).
5. (Opsiyonel) **API-native context editing** (anthropic `clear_tool_uses_20250919`) ile
   eski tool-result'ları cache'i bozmadan yerinde buda — Claude Code microcompact muadili.
6. **Cache-break tespiti + telemetri** debug journal'a.

```mermaid
graph LR
    subgraph "ÖNCE (native)"
      A1["tools ✅HIT"] --> A2["system: statik ✅HIT + DİNAMİK(volatile)❌"] --> A3["mesajlar ❌MISS<br/>(dinamik önekte)"]
    end
    subgraph "SONRA (hedef)"
      B1["tools ✅"] --> B2["system: yalnız statik ✅"] --> B3["mesajlar+özet ✅HIT"] --> B4["yeni mesaj + volatile eki<br/>(breakpoint'in gerisinde, taze)"]
    end
```

## 2. İş paketleri

> P1–P7'nin hepsi uygulandı; aşağıda yalnız her paketin ✅ durum bloğu kalır. Uygulama öncesi
> "Değişim" tasarım maddeleri ve plan bölümleri (§3 sıralama, §4 riskler, §5 test planı,
> §6 kapsam dışı, §7 özet) → [arsiv/50-CLAUDE-CODE-CACHE-PARITE-PLAN.md](arsiv/50-CLAUDE-CODE-CACHE-PARITE-PLAN.md).

### P1 — Volatile dinamiği system'den mesaj kuyruğuna taşı (native paritesi) — *en büyük kazanç* ✅ UYGULANDI (2026-07-04)
> **Durum:** anthropic + OpenAI-compat (openrouter) yollarında uygulandı; yalnız `extendedCache`/
> `cacheSystem` açıkken devreye girer (cache kapalı yol birebir korundu). `anthropic.go`:
> `systemField` artık statik-only, `toAnthropicMessages(msgs, cache, dynamic)` dinamiği son
> mesaja breakpoint'ten SONRA trailing blok olarak ekler. `minimax.go`: `buildSystemMessage`
> statik-only, `attachHistoryBreakpoint` işaretlediği indeksi döndürür, dinamik o mesaja eklenir.
> Testler: `anthropic_test.go` (yeni: `TestToAnthropicMessages_DynamicTrailsAfterBreakpoint`,
> `TestToAnthropicMessages_DynamicIgnoredWhenCacheOff`, güncellenen systemField testleri) +
> `minimax_test.go` `TestOpenRouter_SystemCacheControl` yeni yerleşimi doğrular. **289 test yeşil.**
> **Canlı doğrulandı (2026-07-05, OpenRouter `anthropic/claude-haiku-4.5`):** statik System
> (~8.2k tok) + geçmiş, dinamik **her tur değişmesine rağmen** turn-2'de `cache_read=8197`
> HIT aldı → dinamiğin mesaj tail'ine taşınması önekin cache'ini bozmuyor. (P1'den önce
> dinamik system'de olduğu için bu 0 olurdu.)

### P2 — Özeti mesaja çevir (compact boundary), cache'lenebilir yap ✅ UYGULANDI (2026-07-05)
> **Durum:** Özet artık volatile Dinamik'ten çıktı; `providers.Request.Summary` alanıyla
> taşınıyor. Native yollar (anthropic + openrouter) onu **sentetik head user mesajı** olarak
> cache'li önekin başına koyuyor (`prependSummaryMessage`, `internal/providers/summary.go`) →
> rolling breakpoint son mesajda kaldığı için özet iki katlama arası **cache-read**. Cache
> KAPALI yolda özet `system`'e geri katlanıyor (pre-P2 paritesi). **claude-cli native-only
> kararı gereği DEĞİŞMEDİ** — özet hâlâ `[Context]` tail'ine dokunularak her tur taze gider
> (warm delta yolunda katlama sonrası bayat kalmasın diye; `joinNonEmpty(dynamic, summary)`).
> Önizleme (P6): `cachePreview.SummaryCached` + "Özet" bölümü yeşil + cache-read notu; token
> `req.Summary` üzerinden. Testler: `anthropic_test.go` (`TestPrependSummaryMessage`,
> `TestBuildSystemAndMessages_SummaryHeadCachedWhenOn` / `...FoldsIntoSystemWhenOff`),
> `minimax_test.go` (`TestOpenRouter_SummaryHeadMessage`, `TestMinimax_SummaryFoldsIntoSystem`),
> `claudecli_live_test.go` (özet [Context] tail regresyon guard'ı). **311 test yeşil, tsc temiz.**
> **Canlı doğrulandı (2026-07-05, OpenRouter):** turn-1 özet head'i yazıldıktan sonra turn-2
> aynı özet + geçmiş için `cache_read=8216` HIT aldı → stabil özet head'i cache'li önekin
> parçası, her tur taze gönderilmiyor.

### P3 — API-native context editing (opsiyonel, anthropic-only) — *microcompact muadili* ✅ UYGULANDI (2026-07-05)
> **Durum:** `anthropic.go` — `context_management` beta (`context-management-2025-06-27` header).
> `anthropicReq.ContextManagement` + `contextMgmt()` yalnız `contextEditing` açıkken bir
> `clear_tool_uses_20250919` edit'i ekler (trigger 100k input_tokens, keep 3 tool_uses,
> clear_at_least 5k). `WithBetas(extendedCache, contextEditing)` genişledi; `ResolvedConfig.
> ContextEditing` + `Registry.betaContextEditing` + `SetAnthropicBetas(...)`. Ayar
> `AnthropicContextEditing` (settings.go/store.go, **vars. kapalı**), `api/server.go` canlı
> uygular. Frontend: ContextPanel'de yeni toggle + `AppSettings.anthropicContextEditing`.
> Default skill `tionharness-settings` belgeler. Testler: `TestContextEditing_Off/On`
> (contextMgmt + betaHeader + body serileştirme). Client-side fold'a **ek**, alternatif değil.

### P4 — Cache-break tespiti + telemetri (debug journal) ✅ UYGULANDI (2026-07-05)
> **Durum:** `internal/agent/cachebreak.go` — `Runtime.cacheProbes` (sync.Map, oturum-başına
> `{prefixSig, model, warmed}`) turlar-arası durum tutar. `noteCacheOutcome` her ana konuşma
> turu provider çağrısında (recordedComplete ×2 + recordedStream; yalnız `isConversationKind`:
> chat/task/schedule/flow/spawn — yardımcı title/summary/reflect/compact/subagent hariç) sıcak
> önek kaybını yakalar: **warmed && cacheRead==0 && cacheWrite≥2000** → `cache_break` debug
> olayı, sebep atıflı (`attributeCacheBreak`: model-changed / prompt-or-tools-changed /
> ttl-or-server-eviction). `cachePrefixSig` yalnız statik System + araç adı/şemasını hash'ler
> (dinamik/mesajlar hariç — P1/P2 sonrası önek dışı). `db.DebugCacheBreak` sabiti + `DebugSummary`
> `CacheBreaks`/`LastCacheBreak` + anomali (1→info, ≥2→warn). Frontend: Debug kartında "Cache
> kırılması" pill + event filtresi + etiket. Testler: `cachebreak_test.go` (sig/atıf/kind),
> `debug_journal_test.go TestDebugSummaryCacheBreaks`. **go test yeşil, tsc temiz.**
> **Canlı doğrulama (2026-07-05) sırasında bulunan iyileştirme:** OpenRouter cache-write
> sayacını raporlamıyor (soğuk öneki düz `input` olarak faturalıyor). Tetik koşulu
> `cacheWrite≥floor`'dan **`cacheWrite+input≥floor`**'a genişletildi → kırılma hem native
> Anthropic'te (prefix cache_creation'da) hem OpenRouter'da (prefix input'ta) yakalanır.
> Canlı: sistem öneki başından değişince `cache_read` 8216→0, soğuk önek input=8222 → tetiklenir.

### P5 — TTL / breakpoint kararlılığı (hardening) ✅ UYGULANDI (2026-07-05)
> **Durum:** Tüm anthropic breakpoint'leri (tools + statik System + rolling history) artık
> tek `cacheTTL = "1h"` sabitinden türer (`anthropic.go`) → istek-içi TTL drift'i (Anthropic'in
> "sonraki breakpoint daha kısa TTL taşıyamaz" kuralını bozacak karışık-TTL) imkânsız. P1/P2
> sonrası tek-stabil-marker ilkesi test'le kilitlendi: `TestCacheBreakpointStability` bir tam
> istekte (tools+system+summary head+dynamic+mesajlar) **tam olarak bir** rolling mesaj
> breakpoint'i olduğunu ve son **persist** blokta durduğunu (volatile dinamik trailer'da veya
> özet head'inde DEĞİL), ve tüm TTL'lerin `cacheTTL` olduğunu doğrular. openrouter/minimax
> yolu tek tip `ephemeral` kullanır (TTL yok → drift riski yok). Mid-session flip yalnız
> kullanıcı ayarı değişince olur (beklenen; P4 detektörü yakalar).

### P6 — Önizleme cache haritasını gerçeğe hizala ✅ UYGULANDI (2026-07-04)
> `computeCachePreview` anthropic dalı: `CachedMsgCount = msgCount-1` (rolling), Araçlar+Sistem+
> geçmiş cache'li, dinamik "tail/taze" notu. `hasDynamic` artık system'de değil.

### P7 — Chat yüzeyi: kırılımı sohbette göster ✅ UYGULANDI (2026-08-11)
> **Durum:** P4 tespiti/atfı zaten vardı ama yalnız oturum-seviyesi Debug kartında görünüyordu;
> mesaj başına hiç yoktu (`GetTurnDebug` `cache_break` olayını toplamıyordu). Beş yüzey eklendi
> — detay ve kasıt tablosu: `_Docs\07-CHAT-UX.md` → "Prompt-cache görünürlüğü".
>
> - **Backend:** `db.TurnDebug` += `CacheBreaks/CacheBreakReason/CacheBreakDetail/
>   CoolingWasteUSD/CoolingWasteEstimated` + `GetTurnDebug`'a `DebugCacheBreak` dalı (olaylar
>   zaten `TurnID` damgalı — `emitDebug`). Yeni `agent.StepCacheBreak` adımı +
>   `TurnStep.ColdTokens`; `noteCacheOutcome` "bir şey değişti" sebeplerinde
>   `Runtime.pendingCacheBreaks`'e tek-atımlık not bırakır, `ConsumeCacheBreak` boşaltır,
>   `api.consumeCacheBreakLead` turun kalıcı izinin **başına** ekler (canlı SSE yok — sebep
>   `07`'de). TTL kırılımı bilerek kartsız.
> - **Frontend:** `CacheBreakCard` (katlanabilir, `prompt-or-tools-changed`'de epoch uyarısı +
>   "Bağlamı yenile" aksiyonu), `CacheWarmthStrip` (composer üstü geri sayım + oturumun soğuk
>   tur sayısı), `ColdCacheDivider` (transkriptte TTL'i aşan boşluk ayracı), `CacheWarmthDot`
>   (tur altbilgisinde 🔥/❄), `MessageDebugPanel`'de sebep + kaçınılabilir fazla ödeme.
> - **Testler:** `db.TestGetTurnDebugCacheBreak` (tur izolasyonu + waste), `agent.
>   TestInlineCacheBreak` / `TestConsumeCacheBreak` / `TestCacheBreakStep`. Tüm Go suite +
>   frontend `tsc`/vitest/build yeşil.

## 8. claude-cli süreç modeli ölçümü — respawn+resume vs kalıcı süreç (2026-07-05)

> Bağlam: claude-cli yolunda cache YERLEŞİM disiplini `external-agent-oss` ile aynı (statik
> system + volatil mesaj-kuyruğu). Tek gerçek fark **süreç sıcaklık modeli**: Craft her tur
> subprocess'i **respawn+`--resume`** eder; TionHarness buna ek olarak **kalıcı süreç havuzu**
> (`claudecli_session.go`) sunar. Bu ikisini aynı statik system + aynı volatil saatle, aynı 3
> turda kafa-kafaya ölçen benchmark: `providers.TestLiveCacheCompareModes`
> (`claudecli_cachebench_test.go`, `TIONHARNESS_LIVE_CLI=1` ile).

### İlk koşu (3 tur, n=2 warm) — GÜRÜLTÜLÜ, aşağıda düzeltildi
warm-tur ort. persistent 4851 ms vs respawn 7143 ms → görünüşte ~%32. Ama n=2, bir 8446 ms
aykırı değer ve çapraz-koşu cache bulaşması (persistent tur-1 read=37998) sonucu şişirdi.

### Sağlamlaştırılmış koşu (6 tur, n=5 warm, mod-başına benzersiz prefix nonce) — 2026-07-05

| mod | tur | wall_ms | input | cache_read | cache_write |
|-----|-----|---------|-------|-----------|-------------|
| respawn+resume | 1 | 8876 | 3654 | 37998 | 5580 |
| respawn+resume | 2 | 7574 | 1570 | 48093 | 9549 |
| respawn+resume | 3 | 5922 | 2 | 57642 | 1644 |
| respawn+resume | 4 | 5421 | 2 | 59286 | 70 |
| respawn+resume | 5 | 5839 | 2 | 59356 | 78 |
| respawn+resume | 6 | 5572 | 2 | 59434 | 70 |
| persistent | 1 | 8317 | 3654 | 37998 | 5754 |
| persistent | 2 | 5414 | 2412 | 54622 | 9537 |
| persistent | 3 | 6102 | 2 | 64159 | 2486 |
| persistent | 4 | 7638 | 2 | 66645 | 70 |
| persistent | 5 | 4623 | 2 | 66715 | 78 |
| persistent | 6 | 4777 | 2 | 66793 | 70 |

warm (2–6, n=5): **respawn** wall mean=6065 median=5839, cache_read mean=56762 · **persistent**
wall mean=5710 median=5414, cache_read mean=63786.

**Düzeltilmiş bulgular:**
- **Latency avantajı ~%32 DEĞİL, ~%6–7** (medyan 5414 vs 5839 ms). Sağlamlaştırma (N↑, benzersiz
  prefix, medyan) ilk koşunun gürültüsünü ayıkladı — **hardening'in asıl değeri: yanlış sonucu düzeltti.**
- **Cache_read'de persistent ~%12 yüksek** (63786 vs 56762) — kalıcı süreç in-memory tam transcript'i
  tutup her tur daha büyük warm prefix okuyor.
- İki modun da **tur-1 cache_read=37998** okuması → bizim (nonce'lu) prefix'imiz değil, **claude CLI'ın
  kendi sabit preset'i** (global server-side cache) — gerçekçi, bulaşma değil.
- Medyan < ortalama (persistent tur-4'te 7638 ms aykırı) → küçük N'de medyan daha dürüst.

**Sonuç:** §0'daki *"claude-cli yolu zaten optimal (cache yerleşimi)"* doğrulandı. Kalıcı havuzun
katkısı ölçülü: **warm-latency ~%6–7 + cache_read ~%12** — dramatik değil ama pozitif. Varsayılan
açmadan önce çok-koşulu (repeat) + TTL-ayrık ölçümle teyit et. Test: `TestLiveCacheCompareModes`.
