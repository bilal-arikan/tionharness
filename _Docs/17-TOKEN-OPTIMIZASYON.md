# 17 — Araç Çıktısı Token Optimizasyonu

> Ajan araç çıktılarının (shell, dosya, MCP) LLM context'ine girmeden önce küçültülmesi.
> **İki bağımsız sistem**, paralel veya tek başına çalışabilir; her biri Ayarlar'dan ayrı konfigüre edilir.

## Neden?

SwarmGo'nun native agentic döngüsünde (`agent/toolloop.go`) her araç çağrısının çıktısı bir
`ToolResult` olarak konuşmaya eklenir ve sonraki model çağrısında **girdi token'ı** olarak ücretlenir.
`Bash` gibi araçlar 64 KB'ye kadar ham çıktı döndürebilir. `git status`, test runner, `ls -R`, `grep`
gibi komutlar context'i hızla şişirir. Bu katman, çıktı transcript'e *girmeden önce* onu kırpar — mevcut
`internal/conversation` compaction'ı (transcript bütçesi) ve prompt-cache'i tamamlar.

## İki Sistem

| | **Sistem A — Deterministik** | **Sistem B — LLM Özeti** |
|---|---|---|
| Paket | `internal/tools/compact` | `internal/agent/compactor.go` |
| İlham | `rtk-ai/rtk` (Rust Token Killer) | `external-agent-oss` (Large Response Handling) |
| Yöntem | Kural tabanlı: dedupe + boş-satır sadeleştirme + ortadan kırpma | Ucuz modelle niyet-farkında özet |
| Maliyet | Sıfır (yerel) | Ekstra model çağrısı (`KindCompact`) |
| Tetik | Her başarılı araç çıktısı | Yalnız (A sonrası) eşik üstü çıktı |
| Varsayılan | **Açık** | **Kapalı** (opt-in, maliyetli) |

İkisi de açıksa **ardışık**: önce ücretsiz A, kalan hâlâ eşik üstündeyse B. Biri açıksa yalnız o çalışır.
İkisi de kapalıysa eski davranış (sadece tool'un kendi 64 KB hard-cap'i) korunur.

```mermaid
graph TD
    A["Tool çıktısı (res.Content)"] --> E{"IsError / boş?"}
    E -->|evet| OUT["dokunulmaz → context"]
    E -->|hayır| CA{"Sistem A açık?"}
    CA -->|evet| RA["compact.Compact:<br/>dedupe / boş-satır / ortadan kırp"]
    CA -->|hayır| SB
    RA --> SB{"Sistem B açık<br/>& boyut > eşik?"}
    SB -->|evet| LB["summarizeToolOutput<br/>(cheap model, intent-aware)"]
    SB -->|hayır| OUT2["context'e + persisted step"]
    LB -->|başarı| OUT2
    LB -->|hata/boş| OUT2
    style RA fill:#2d6,stroke:#093
    style LB fill:#69d,stroke:#036
```

## Sistem A — `internal/tools/compact`

`Compact(output string, opts Options) (string, Stats)` — dep-siz, hata döndürmez (en kötü ihtimalle
orijinali verir).

- **Dedupe:** Ardışık aynı satırlar `satır  (×N)` olarak birleşir (trailing whitespace yok sayılır).
- **Boş satır blokları:** 2+ ardışık boş satır → tek boş satır.
- **Satır eleme:** `MaxLines` aşılırsa baş + son yarı korunur, ortaya `… N satır atlandı …` işareti.
- **Bayt kırpma:** `MaxBytes` aşılırsa baş (2/3) + son (1/3) korunarak ortaya `… [çıktı N bayt kırpıldı] …`;
  kesim **rune sınırında** yapılır (UTF-8 bozulmaz — Türkçe karakterler güvenli).
- `Stats{BeforeBytes, AfterBytes, Applied}` + `Saved()` — log ve tasarruf ölçümü.
- **Kalıcı tasarruf sayacı:** `Stats.Saved()` (Sistem A'nın kazandırdığı bayt) `compactToolResult` içinde
  `db.AddCompactionSavings(agentID, bytes)` ile günlük usage rollup'una yazılır →
  `Usage.CompactSavedBytes` (ajan+gün başına, `compactSavedBytes` JSON). LLM çağrısı/token sayaçlarından
  **bağımsız** bir ölçer (maliyet etkisi yok). **Bütçe ekranında gösteriliyor (2026-06-24):** bkz.
  [§Bütçe görünürlüğü](#bütçe-görünürlüğü--tasarruf-merkezi--session-bazlı-2026-06-24).

## Sistem B — `agent/compactor.go`

- `compactToolResult(ctx, agent, toolName, input, res)` — A'yı uygular, sonra B eşiğini kontrol eder.
  `IsError` veya boş sonuçlar **hiç dokunulmadan** geçer.
- `summarizeToolOutput(...)` — `guardedComplete` + `WithCallKind(ctx, KindCompact)`.
  **Model çözüm zinciri:** adanmış sıkıştırma modeli (`CompactModel`) → yoksa başlık modeli (`TitleModel`)
  → yoksa ajanın kendi modeli. **Sağlayıcı her zaman ajanın sağlayıcısıdır** (`guardedComplete`
  `r.providers.Get(agent.Provider)` ile çözer — ayrı seçilemez). `CompactModel` yalnızca bir model-id'dir;
  o yüzden ajanın sağlayıcısıyla uyumlu, ucuz bir model (ör. `claude-haiku-4-5`) verilmelidir.
  **Niyet** = tool adı + (varsa) input özeti (`intentInputRunes=300`).
  Sistem prompt: olguları (yol/kimlik/hata/sayı/sonuç) koru, uydurma yapma, sadece sonucu döndür.
- Bütçeyi **gate'lemez** (autonomous=false) — titler/summary/reflect ile aynı politika; usage yine işlenir.
- **Tasarruf sayacı (2026-06-24):** özet başarılıysa `len(içerik)-len(özet)` bayt `db.AddLLMCompactionSavings`
  ile günlük rollup'a (`Usage.CompactSavedBytesLLM`, `compactSavedBytesLLM` JSON) yazılır — Sistem A ölçerinden
  **ayrı** (özet çağrısının kendi token maliyeti `UsageKindCompact` altında zaten kayıtlı; bu, brüt çıktı azaltımı).
- Yalnız **native döngüde** (Anthropic/MiniMax) etkilidir; claude-cli delegasyonu kendi döngüsünü sürdürür
  (çıktıları SwarmGo'nun `ToolResult` katmanından geçmez).

## Ayarlar

`settings.Settings` / `DTO` / `Patch` (clamp'ler `store.go::validate`'de):

| Alan | Sistem | Vars. | Clamp |
|------|--------|-------|-------|
| `compactToolOutput` | A aç/kapa | `true` | — |
| `compactMaxLines` | A satır sınırı | `200` | 0 (=default) – 5000 |
| `compactMaxBytes` | A bayt sınırı | `16384` | 0 (=default) – 262144 |
| `compactLlmSummary` | B aç/kapa | `true` | — |
| `compactLlmThreshold` | B eşik (bayt) | `12288` | 0 (=default) – 262144 |
| `compactModel` | B model-id | `""` | trim'lenir; boş = TitleModel → ajan modeli |

Canlı push: `api/server.go::applySettings` → `Tunables.SetToolCompaction(...)`. 0 değerleri Tunables
getter'larında built-in default'a (`DefaultCompact*`) çevrilir. UI: **Ayarlar → Bağlam** içinde iki
ayrı bölüm (`frontend/.../settings/appPanels.tsx` `ContextPanel`).

> **Varsayılan politika değişikliği (2026-06-22):** Sistem B artık **varsayılan AÇIK**, the external agent project
> tarzı (~12KB eşik). Kritik bağımlılık: **A'nın bayt cap'i (16KB) B eşiğinin (12KB) ÜSTÜNDE** olmalı —
> aksi halde A çıktıyı B eşiğinin altına kırpıp B'yi hiç tetiklenmez bırakır. Yeni varsayılanlar bu
> sırayı korur: 12–16KB bandı A'dan geçip B'ye ulaşır, >16KB ise A 16KB'ye kırpar sonra B özetler.
> B bir ucuz model çağrısı maliyetlidir → `compactModel`'i ucuz bir modele (ör. `claude-haiku`) pinle.

## Harici araç tespiti (presence-only)

Ayarlar → **Tanılama** ekranındaki "Kurulu mu kontrol et" butonu, bu cihazda isteğe bağlı harici
token araçlarının (`rtk`, `sqz`, `context-mode`) **kurulu olup olmadığını** gösterir.

- Backend: `GET /api/external-tools` (`api/external_tools.go`) → `exec.LookPath` ile PATH'te arar.
  **Araçları kurmaz, çalıştırmaz, değiştirmez** (Windows'ta PATHEXT'e saygılı). Dönüş: `[{name,desc,url,found,path}]`.
- Frontend: `systemApi.externalTools()` + `DiagnosticsPanel` butonu; her araç için ✓ kurulu / — bulunamadı + repo linki.
- Bu yalnızca **bilgilendirme**dir; SwarmGo bu araçları otomatik kullanmaz (Sistem A/B native'dir). Kullanıcı
  isterse manuel entegrasyon için varlığı görür.

## Sınırlar / Notlar

- Sıkıştırma hem modele giden `ToolResult`'a **hem de** UI'da gösterilen/persist edilen `TurnStep.Output`'a
  uygulanır → kullanıcı, modelin gördüğü çıktıyı görür (tutarlılık).
- Komut-özel akıllı kısaltıcılar (git/test/grep'e özgü) henüz yok; A jeneriktir. → bkz. [Yapılacak](#yapılacak--craftagenttan-aktarılacak-fikirler).
- claude-cli delegasyon yolu kapsam dışıdır (yukarıdaki sebep).

## Yapılacak — the external agent project'tan Aktarılacak Fikirler

> Kaynak: `external-agent-oss` ([repo](https://github.com/external-agent-project/external-agent-oss)) bağlam-yönetimi
> incelemesi (2026-06-22). the external agent project çoğu bağlam işini Claude Agent SDK'ye devreder; SwarmGo'nun açık
> motoru genel olarak daha kontrollü. Aşağıdakiler the external agent project'ta işe yarayan, SwarmGo'ya değer katacak
> birkaç pratik dokunuş — **henüz yapılmadı**.

### 1. Komut-aile-bazlı deterministik bash sıkıştırıcı (RTK tarzı) 🔶

the external agent project yerel **RTK (Rewrite Toolkit)** binary'siyle `git diff`, `ls -R`, `bun test`, `npm install`,
`grep` gibi gürültülü bash çıktılarını **model'e gitmeden önce** komut-ailesine özel kurallarla yeniden yazar
(tasarruf istatistiği de tutar). SwarmGo'da Sistem A jeneriktir (dedupe + boş-satır + ortadan kırpma);
komut-özel akıllı kısaltıcı yok.

- **Yapılacak:** `internal/tools/compact` içine komut-aile tanıyıcı bir katman (ör. `compact/rules_*.go`):
  `git diff`/`git status` → dosya başına özet, `ls -R`/`tree` → derinlik kırpma, test runner → yalnız
  fail+özet satırları, `grep` → eşleşme yoğunluğu kırpma.
- Tetik: `Bash` tool input'undaki komut adına göre kural seçimi; kural yoksa mevcut jenerik A'ya düş.
- Sistem A ile aynı sözleşme: dep-siz, hata döndürmez, `Stats.Saved()` rollup'a yazılır.

### 2. `_intent` — açık niyet enjeksiyonu 🔶

the external agent project her MCP tool çağrısında şemaya bir **`_intent`** alanı enjekte eder; bu, büyük-sonuç
özetlemesinin **neye odaklanacağını** açıkça söyler. SwarmGo'da Sistem B niyeti *çıkarımla* buluyor
(tool adı + ilk `intentInputRunes=300` input). Açık niyet daha iyi sinyal verir.

- **Yapılacak:** native tool-use döngüsünde (`agent/toolloop.go`) modelin tool çağrısına opsiyonel bir
  `_intent` (kısa amaç cümlesi) taşımasını sağla; `summarizeToolOutput` bunu çıkarım yerine doğrudan
  niyet olarak kullansın. MCP araçlarında şema NormalizeSchema sırasında `_intent` alanı eklenebilir.
- Düşük maliyet / yüksek fayda: Sistem B özet kalitesini, ekstra model çağrısı olmadan artırır.

### 3. Büyük-sonuç özet eşiğini the external agent project ile hizala ✅ YAPILDI (2026-06-22)

the external agent project büyük tool sonuçlarını Haiku ile **varsayılan otomatik** özetler. SwarmGo Sistem B eşiği
eskiden **8192 bayt** ve varsayılan **kapalı**ydı. Artık the external agent project tarzı: **Sistem B varsayılan AÇIK,
eşik 12288 bayt (~12KB)**; A'nın bayt cap'i 16384'e yükseltildi ki A→B sırası korunsun (yukarıdaki
"Varsayılan politika değişikliği" notu). Mekanizma zaten vardı; bu yalnızca varsayılan + eşik ayarıydı.

### 4. Density-aware token tahmini (CG-9, birinci yarı) ✅ YAPILDI (2026-06-22)

Transcript bütçesi (`conversation/tokens.go`) eskiden sabit **chars/4** kullanıyordu → base64/hex/
minified gibi yoğun içerik ~%60 eksik sayılıp gerçek context window'u sessizce taşırıyordu ("session
poisoning"). Artık `estimateText` **density-aware**: uzun ve neredeyse boşluksuz (`<%3` whitespace,
≥256 rune) içerik **~1.5 chars/token** (`runes*2/3`), düz metin **~4 chars/token**. Tek geçiş, bağımlılık
yok. `tokens_test.go`.

### 5. Bütçe-orantılı tool eşikleri (CG-9, ikinci yarı) ✅ YAPILDI (2026-06-22)

the external agent project araç-sonucu eşiğini context window'a göre ölçekler (`tokenLimitFor`, ctx×0.10). SwarmGo'nun
karşılığı: eşikleri **transcript bütçesine** (`settings.MaxContextTokens`) orantıla — büyük bütçe → büyük
tool sonucu compaction'dan önce tolere edilir. `Tunables.budgetScaleLocked()` = `budget / 12000`, clamp
**[1×, 5×]**. **Hem** A bayt cap'i **hem** B eşiği **aynı** faktörle çarpılır → `A-cap > B-threshold`
değişmezi her ölçekte korunur. `tunables_compact_test.go`.

- Varsayılan bütçe (12000) → 1× → tam yapılandırılmış değerler (A=16384, B=12288) — regresyon yok.
- 5× tavan → B≈60KB tetik, the external agent project'ın ~60KB özet tavanıyla örtüşür.
- Wiring: `applySettings` → `Tunables.SetContextBudget(MaxContextTokens)` (`s.convo.SetLimits` yanında).
  `contextBudgetTokens=0` → 1× (güvenli no-op). **Not:** runtime wiring tek satır; `api` paketi paralel
  oturumun MemGPT Parça-4 WIP'iyle geçici kırık olduğundan bu satır o paket bütünleşince commit'lenir
  (dormant — wiring olmadan da davranış birebir mevcut varsayılan).

### 6. Per-model context-window metadata (tokenLimitFor zemini) ✅ YAPILDI (2026-06-22)

`ModelInfo`'ya **`ContextWindow int`** (token) eklendi; `Catalog()` build-time'da merkezi
**`ContextWindowFor(provider, model)`** aile-tablosundan doldurur (manifest'ler temiz kalır). Aile-bazlı,
**bilinçli muhafazakâr**: yalnız emin olunan aileler (Opus 4.8/Sonnet 4.6 **1M**, Haiku 4.5 **200K**;
MiniMax/DeepSeek/Gemini 1M; Fable/genel Claude 200K),
gerisi 0 = "bilinmiyor" → çağıran fallback yapar. UI model picker'ı artık pencere boyutunu gösterebilir.
`context_window_test.go`. **Mimari not — gerçek `tokenLimitFor` neden doğrudan takılmadı:** the external agent project
eşiği **model penceresine** (window×0.10) ölçekler çünkü tüm pencereyi SDK'ye kullandırır. SwarmGo
transcript'i **bilinçle 12K token'a** bütçeler (ucuz); eşiği 200K–1M pencereye ölçeklemek, tek bir tool
sonucunun **tüm transcript bütçesini aşmasına** yol açardı (tutarsız). Bu yüzden tool eşikleri **bütçeye**
(§5) bağlı kaldı; pencere metadata'sı **UI + bütçe-tavanı guard** için. Pencereyi gerçekten kullanmak
istenirse doğru hamle: **modele göre akıllı varsayılan bütçe** (flat 12K yerine `min(window, hedef)`),
sonra §5 zaten onu ölçekler.

### 7. Modele göre akıllı varsayılan bütçe (Option B) ✅ YAPILDI (2026-06-22)

Flat 12K transcript bütçesi büyük modelin penceresini boşa harcıyordu. `conversation.EffectiveBudget(provider,
model, configured)`: model penceresi biliniyorsa bütçeyi **`clamp(window × 0.10, configured, 32K)`**'a yükseltir
— yapılandırılmış değer **taban** (asla altına inmez), 32K **tavan** (1M modelde maliyet patlamasın). Bilinmeyen
pencere → değişmez. `Manager.Prepare` artık compaction tetiğini ve pressure oranını bu model-aware bütçeyle
hesaplıyor; `maxTokens≤0` (bütçe kapalı) dokunulmaz. `budget_test.go`.

- **Değerler (2026-06-22, 1M kullanımı artırıldı):** `fraction 0.10→0.20`, `ceil 32K→128K`, tool-eşik scale
  clamp `[1×,5×]→[1×,12×]`. Sonuç: Opus 4.8/Sonnet 4.6/MiniMax/DeepSeek/Gemini **1M → 128K** (tavan) ·
  Haiku 4.5 **200K → 40K** · bilinmeyen → 12K. 1M modellerde **ceil** operatif sayıdır (window×fraction onu
  aşar) → "1M'i ne kadar kullanırız" knob'u = ceil. 128K ≈ 1M'in %12.8'i. Maliyet: 1M modelde ~10× eski varsayılan
  (prompt-cache ile hafifler); daha çok/az istenirse `budgetAutoCeil` ayarlanır.
- **Ayarlanabilir + 512K varsayılan (2026-06-23) — ⚠️ rot-bilinçli revize edildi, bkz. [§12](#12--context-rot-farkındalığı-ve-bütçe-stratejisi-2026-06-25):** `fraction` ve `ceil` artık **settings'ten canlı yapılandırılabilir**
  (`ContextBudgetFraction` / `ContextBudgetCeil`; env `SWARMGO_CONTEXT_BUDGET_FRACTION` / `SWARMGO_CONTEXT_BUDGET_CEIL`).
  Bu fazın varsayılanları: **`fraction 0.20→0.6`, `ceil 128K→512K`**. Sonuç: 1M model **1M×0.6=600K → 512K** (tavana kırpılır,
  ≈ pencerenin %51'i) · Haiku 200K → **120K** · bilinmeyen → taban (12K). Amaç: 1M modelde **kullanıcının ilk mesajı
  ilk sessiz katlamaya kadar çok daha uzun süre aynen kalsın** (External Agent/Claude Code'un "tüm transkripti 1M pencerede
  tut" davranışına yaklaşır). `EffectiveBudget(provider, model, configured, fraction, ceil)` imzası fraction/ceil alır;
  ≤0 değerler paket varsayılanına düşer. `Manager.SetBudgetShape` ile canlı güncellenir (`server.go applySettings`).
  Frontend: Ayarlar ▸ Bağlam penceresi → "Bütçe tavanı" + "Pencere oranı". `budget_test.go` güncellendi.
  **Not (2026-06-25):** bu 512K/0.6 varsayılanları context-rot takası nedeniyle **256K + adaptif fraction**'a çekildi → §12.
- **Tool eşikleriyle hizalama ✅ YAPILDI (2026-06-22):** Compactor artık her tur için
  `conversation.EffectiveBudget(agent.Provider, agent.Model, tun.ContextBudgetTokens())` hesaplayıp
  `Tunables.CompactMaxBytesFor(budget)` / `CompactLLMThresholdFor(budget)` ile **per-model** ölçekliyor.
  Zincir tam tutarlı: model penceresi → EffectiveBudget → §5 eşik ölçeği. Büyük-pencere modelde A cap +
  B eşiği orantılı büyür, A>B değişmezi korunur. No-arg getter'lar process-geneli bütçeyle geriye-uyumlu
  kaldı. `tunables_compact_test.go` (For varyantları).

### 8. Yapılandırılmış konuşma-özeti (Claude Code parite, 1. faz) ✅ YAPILDI (2026-06-23)

Transkript compaction'ının (`conversation/manager.go`) rolling-summary prompt'u eskiden **"under 200
words"** ile sınırlıydı ve her katlamada **eski özet + yeni mesajları** tekrar 200 kelimeye sıkıştırıyordu
→ çok katlamada erken bağlam **kademeli eriyordu (decay)**; üretilen özet de kullanıcıya "çok kısa"
görünüyordu. Claude Code'un compaction motoru incelendi (`observed-behavior/src/services/compact/`) ve
prompt onun **yapılandırılmış çok-bölümlü** yaklaşımına yakınlaştırıldı.

- **Yeni `compactPrompt`:** sabit **8 bölüm** (Primary Request/Intent · Key Technical Concepts · Files and
  Code · Errors and Fixes · Decisions and User Feedback · Pending Tasks · Current Work · Next Step) +
  **açık anti-decay talimatı**: "önceki özetteki her kalıcı gerçeği taşı, yer açmak için kısaltma/yeniden
  sıkıştırma yapma". 200-kelime cap'i kaldırıldı. Konuşma diline cevap verir. İki `%s` placeholder
  (mevcut özet, yeni mesajlar) korundu → `reactive.go` (mid-loop reaktif compact) aynı sabiti yeniden
  kullanmaya devam ediyor.
- **Output bütçesi:** `compactMaxOutputTokens = 8192` sabiti eklendi; hem `summarize` (rolling) hem
  reactive yol `Request.MaxTokens` ile bunu geçiyor → uzun yapılandırılmış özet provider default'unda
  (anthropic 4096) **bölüm ortasında kesilmiyor**. `build ./...` + `conversation` testleri yeşil.
- **Canlı test bulgusu — kapanış cue'su (2026-06-23):** Gerçek SES2 mesajlarıyla `claude -p` üzerinde test
  edilince ilk prompt **başarısız**: model transcript ile bittiği için onu "devam ettirilecek konuşma" sanıp
  özet yerine **son mesaja cevap verdi**. claude-cli `--append-system-prompt` ile Claude Code'un ajan
  framing'ini (araçlar dahil) koruduğundan summarize çağrısı tam-ajan olarak koşuyor. **Düzeltme:** prompt'a
  `NEW MESSAGES`'tan **sonra** güçlü kapanış talimatı eklendi ("yukarısı özetlenecek transcript — devam etme,
  cevap verme, araç çağırma; doğrudan '1. Primary Request and Intent:' ile başla"). Claude Code'un
  `NO_TOOLS_TRAILER` deseninin karşılığı. Tekrar test: **~7.7KB tam yapılandırılmış 8-bölümlü özet** (eski
  ~1.1KB digest'e karşı), tüm task/agent/flow/karar yakalandı, model "transcript yalnız özetlenecek" notuna
  uydu. (Test çıktıları geçiciydi; in-server teyitten sonra temizlendi.)
- **Uçtan-uca in-server teyit (2026-06-23):** dev binary yeniden derlenip 8090'da başlatıldı, WS2/SES2'de
  gerçek `POST /api/sessions/SES2/summary {kind:compact}` (=`ForceCompact`, claude-cli provider) tetiklendi.
  Sonuç: 8 mesaj katlandı, `summaryMsgCount` 12→20, özet **1176→4464 char** (yapılandırılmış 8-bölüm, Türkçe),
  konuşma-devamı yok, eski digest'te olmayan detaylar (paralel flow şema sözdizimi, validator hatası+fix)
  yakalandı → anti-decay merge in-server doğrulandı.
- **Not — fork bilinçli eklenMEdi:** Claude Code özetleyiciye **tüm konuşmayı** yollar (bu yüzden
  prompt-cache paylaşan fork şart). SwarmGo `summarize` yalnız **katlanan dilim + eski özeti** yollar →
  çağrı zaten ucuz, fork'un çözeceği pahalılık yok. Birincil provider `claude-cli` (anahtarsız) cache
  paylaşımını CC gibi kontrol edemez → fork ROI düşük, ertelendi.
- **Opsiyonel 3. faz:** partial compact (`from`/`up_to`) + boundary marker UI. Decay'i tamamen sıfırlamak
  isterse: merge yerine her katlamada `history` prefix'inden **sıfırdan** özetleme (CC paritesi, maliyet ↑
  → fork tartışmasına bağlı).

### 9. Post-compact kurtarma işaretçisi — Claude Code parite 2. faz ✅ YAPILDI (2026-06-23)

CC compaction sonrası özet mesajına bir **transcript pointer** ekler ("pre-compaction detayı lazımsa
şu transcript'i oku: …") + son okunan dosyaları `createPostCompactFileAttachments` ile geri enjekte eder.
SwarmGo'ya **birebir port mimariye ters**: SwarmGo turlar arası yalnız `role+text` taşır (`toProviderMessages`),
tool sonuçları/dosya okumaları **zaten cross-turn context'te değil** → "dosya re-injection" diye geri
verilecek bir şey yok; ajan serbest fs araçlarıyla istediğinde **yeniden okur**. Ayrıca kalıcı durum
(artifacts/todos/core-memory/goal/summary) `composeTurnRequest`'te zaten **her tur** re-inject ediliyor.

**Uygulanan (CC mekanizmasının SwarmGo'nun gerçek kurtarma araçlarına uyarlanmışı):** özet bloğu artık
`conversationSummaryBlock(summary)` ile sarılıyor (`api/chat_turn.go`) — özetin altına **kurtarma notu**
ekleniyor: "bu özetten önceki turlar katlandı, tam metni context'te yok; kesin detay (kod/hata/dosya
içeriği/karar) gerekiyorsa **tahmin etme**: `conversation_search` ile ara ya da ilgili dosyaları **fs
araçlarıyla yeniden aç". Böylece compact sonrası ajan körleşmez — kaybolan her şey ya `conversation_search`
(`builtin_conversation_search.go`, db tam-metin tarama, LLM'siz) ile ya da re-read ile **geri alınabilir**.
Sıfır yeni altyapı; readFileState tracker **bilinçle eklenmedi** (mimariye gereksiz). `go build ./...` +
`internal/api` + `internal/conversation` testleri yeşil.

- **Skill/plan re-injection neden gerekmedi:** skill **kataloğu** (slug+özet) statik prefix'te zaten her
  tur var (`SkillsCatalogBlockForAgent`) → compact onu silmez (katlanan mesajlarda değil); yüklü skill
  *gövdesi* `use_skill` ile tekrar çekilir. Plan-mode dosyası SwarmGo'da CC'deki gibi yok.

### 10. `conversation_search` güçlendirme — birebir kurtarma (2026-06-24)

§9'un kurtarma yolu olan `conversation_search` (`builtin_conversation_search.go`) snippet'le sınırlıydı; compact
sonrası **kelime kelime** kurtarma için yetersizdi. Eklenenler:

- **`full=true`** → eşleşen mesajın **tam metni** birebir döner (snippet değil). İlk soruyu/kararı aynen geri almak için.
- **`context=N`** (0–5) → her isabetin **N tur öncesi + sonrası** birebir eklenir; isabet `»»` ile işaretlenir. Çevre
  diyaloğu görmek için.
- **`session_id`** → aramayı tek oturuma daraltır (ör. mevcut oturum). `db.SearchOpts.OnlyID` ile.
- DB: `db.MessagesAround(sid, mid, before, after)` — bellekteki transkriptten çevre turları O(n) çeker (LLM'siz).
  `builtin_conversation_search_test.go` full/context/session_id senaryolarını kapsar. Detay: `27-CROSS-SESSION-SEARCH.md`.

### 11. claude-cli oturum sürekliliği (`--resume`) — sıcak prompt cache (2026-06-24, opt-in)

External Agent/Claude Code'un "ilk soruyu hatırlama"sının asıl ucuzlatıcısı: tam transkripti her tur **yeniden besleyip**
prompt cache'ten okumak (cacheRead ≈ girişin %99'u). SwarmGo claude-cli yolu eskiden her tur transkripti stdin'den
**yeniden serialize** ediyordu → cache reuse yok. Yeni opt-in özellik (`ClaudeResume` ayarı, **varsayılan kapalı**):

- Açıkken her turda CLI **`--resume <id>`** ile önceki oturumu sürdürür; SwarmGo yalnız **delta**yı (CLI'nin görmediği
  yeni mesajları) gönderir → CLI'nin **server-side geçmişi + sıcak cache**'i tekrar kullanılır.
- `providers.Request.ResumeSessionID` / `providers.Response.SessionID`; CLI `session_id`'yi stream'in `system/init` ve
  `result` event'lerinden yakalar (id her resume turunda **rotate olur**, sonuncu saklanır).
- Durum oturum-başına: `db.Session.CLISessionID` + `CLISentMsgCount` (delta sınırı), `SetSessionCLIResume` ile yazılır.
- Akış: `api/chat_resume.go planClaudeResume` → ilk tur **cold** (tam transkript, id yakala), sonraki turlar **warm**
  (delta + `--resume`). Yalnız **tek-ajanlı** sohbette (id oturum-başına; çoklu-ajan çakışırdı). Tutarsızlık (mesaj
  düzenleme → sınır geçersiz) → otomatik cold fallback.
- **Mimari gerilim (bilinçli):** warm modda bağlam yönetimini CLI devralır → SwarmGo'nun kendi compaction'ı o oturumda
  devre dışı kalır. Bu yüzden **opt-in + deneysel**: açtıktan sonra bir sohbette doğrulanmalı. `claudecli_resume_test.go`
  parser'ın session_id yakalamasını kapsar; canlı `--resume` davranışı kullanıcı doğrulamasına bağlı.

## Bütçe görünürlüğü — Tasarruf Merkezi + session bazlı (2026-06-24)

Önceden tüm tasarruf mekanizmaları (cache, Sistem A/B) toplanıyordu ama dağınık/gizliydi; kullanım yalnız
**ajan+gün** anahtarlıydı (oturum-başına atfedilemiyordu). Üç fazlık geliştirme:

### Faz 1 — Sıkıştırma tasarrufunu görünür kıl
- DB: `Usage.CompactSavedBytesLLM` alanı + `AddLLMCompactionSavings` (Sistem B brüt çıktı azaltımı). Sistem A'nın
  `CompactSavedBytes`'ı zaten vardı.
- API: `GET /api/usage` totals + cumulative + trend artık `compactSavedBytes`/`compactSavedBytesLLM` taşır;
  `GET /api/agents/{id}/usage` de ekledi.
- UI: Bütçe ekranında **Tasarruf Merkezi** bölümü (3 hücre: Prompt-cache USD · Sistem A bayt · Sistem B bayt +
  token-eşdeğeri tahmini). Bayt ölçerdir, gerçek faturalandırma değil.

### Faz 2 — Session bazlı kullanım/maliyet
- DB: yeni `SessionUsage` rollup (`internal/db/store_session_usage.go`) — **sessionID anahtarlı, ömür-boyu**
  (gün-reset YOK); ByKind/ByModel + cache + her iki compaction ölçeri. Dosya `store/session-usage/<sid>.json`.
  Metotlar: `AddSessionUsageKind` · `AddSessionCompactionSavings` · `AddSessionLLMCompactionSavings` ·
  `GetSessionUsage`. Boş sessionID = no-op.
- Wiring: `RecordUsage` + `compactToolResult` ctx'teki `SessionIDFrom`'u okuyup ajan kaydının **yanında** session
  rollup'a da yazar (chat/schedule/spawn/flow yolları zaten `WithSessionID` damgalı; chat_stream.go:256).
- API: `GET /api/sessions/{id}/usage-detail` (cost helper'ları `modelRowsFor` ile paylaşılır → workspace ekranıyla
  birebir tutarlı). UI: `SessionDetailPanel`'de **"Bu oturumun harcaması"** kartı (maliyet + kazanç/tasarruf
  kırılımı) — eskiden yalnız "ajanın bugünkü toplamı" gösteriliyordu.

### Faz 3 — Tasarruf Merkezi (birleşik kazanç görünümü)
- Tüm tasarruf kaynakları tek panelde: cache (gerçek USD) + Sistem A (ücretsiz bayt) + Sistem B (LLM bayt) +
  toplam context tasarrufu (bayt → ~token tahmini, `bytes/4`). Bütçe ekranı kümülatif penceresinden beslenir.
- **Trend metrik seçici (2026-06-25):** günlük trend grafiği artık 4 seri arasında geçiş yapar —
  **Token · Maliyet · Cache tasarrufu · Sıkıştırma** (`TREND_METRICS`, `BudgetPanel`). Backend her `dayPoint`'e
  `compactSavedBytes`/`compactSavedBytesLLM` ekledi → trend bunları gün-bazında çizebiliyor; bar rengi+formatlayıcı
  metriğe göre değişir, altta pencere-toplamı gösterilir.

**Notlar / sınırlar:**
- Hook'lar (`PreToolUse`/`PostToolUse`) hâlâ tasarruf **ölçmez** (Claude Code sözleşmesi; gerçek token-tasarruf
  mekanizması Sistem A/B'dir). `context-mode`/`rtk`/`sqz` harici araçları yalnız **presence-only** tespit edilir,
  SwarmGo çıktıları onlardan geçirmez → ölçülen kazanç yok; yerel eşdeğer = Sistem A.
- Bayt→token→USD: Sistem A/B için yalnız bayt + ~token gösterilir, **USD'ye çevrilmez** (uydurma sayı olmaması
  için). Gerçek USD yalnız prompt-cache'te.
- Geriye-uyumlu: eski usage dosyaları yeni alanları taşımaz (omitempty → 0); session rollup yeni turlardan dolar.
- Testler: `store_session_usage_test.go` (session attribution + reload), `store_usage_test.go` (`AddLLMCompactionSavings`).

## Context reset (handoff) — in-place compaction'ın tamamlayıcısı

Bu doküman **in-place** compaction'ı anlatır (aynı oturum, rolling-summary). Anthropic'in
"harness design" bulgusu: uzun otonom görevlerde in-place compaction tek başına **"context
anxiety"**yi (model limite yaklaşınca erken toparlama) çözmez. Tamamlayıcı = **context reset**:
özetlemek yerine bir **handoff artifact** yaz + **temiz bir pencerede** (yeni oturum) devam et.

- **Ne zaman compact?** İnteraktif sohbet, çok-turlu gidip-gelme, kullanıcı sürücü. (Bu doküman.)
- **Ne zaman reset?** Uzun otonom iş, net kilometre taşları, "temiz sayfa" gerektiğinde — manuel
  `/handoff`, `handoff_session` aracı veya basınç eşiğinde otomatik. Detay: **[35-CONTEXT-RESET-HANDOFF.md](35-CONTEXT-RESET-HANDOFF.md)**.

Reset, compaction çekirdeğini (`summarizeRendered`/`compactMaxOutputTokens`) yeniden kullanır;
yalnız devam-odaklı bir prompt (`conversation.handoffPrompt`, 10 bölüm + DONE/TODO + Next Step) ve
bir env snapshot ekler.

## 12 — Context-rot farkındalığı ve bütçe stratejisi (2026-06-25)

**Bağlam.** Anthropic *Effective context engineering for AI agents* makalesi bir gerçeği netleştirir:
token sayısı arttıkça modelin o bağlamdan **doğru geri-çağırma** yeteneği düşer ("context rot").
Sebep transformer mimarisi — *"every token attends to every other token… n² pairwise relationships
for n tokens"*: `n` token → `n²` ikili dikkat ilişkisi; sabit "attention budget" daha çok ilişkiye
yayılır. Bu bir **uçurum değil, performans gradyanıdır** (*"a performance gradient rather than a hard
cliff: models remain highly capable at longer contexts but may show reduced precision for information
retrieval and long-range reasoning"*) — model uzun bağlamda hâlâ yetkin ama **hassasiyet kaybeder**.
İlke: *"the smallest possible set of high-signal tokens"* — bağlam değerli, sonlu bir kaynaktır.
Ham pencereye alternatifler: compaction · structured note-taking · just-in-time retrieval · sub-agent
izolasyonu.

**SwarmGo'nun önceki bahsi (§7, 2026-06-23).** `EffectiveBudget` `fraction=0.6 / ceil=512K`'ye
çıkarılmıştı ("1M pencerede her şeyi ham tut" → Claude Code/External Agent davranışına yaklaşmak). Bu,
**bilinçli olarak rot ile takastı**: ham pencere büyüdükçe `n²` yüzeyi ve recall hassasiyeti kaybı
büyür. Ayrıca Claude Code o davranışı **prompt-cache + fork**'la ucuzlatır; SwarmGo'nun birincil yolu
(`claude-cli`, anahtarsız) bu paylaşımı CC gibi kontrol edemez → büyük ham pencerenin getiri/maliyet
oranı SwarmGo'da daha zayıf.

**Kritik içgörü — dayanıklılık ≠ ham pencere boyutu.** Bir detayın kaybolmaması için 512K ham
transkript *gerekmez*. SwarmGo'nun dayanıklılığı zaten **retrieval katmanında**: `memory_add` + lexical
recall (uzun-dönem) · `core_memory_replace/append` (her tur enjekte working memory) · `conversation_search`
(`full=true`/`context=N` ile **birebir** kurtarma, §10) · post-compact kurtarma notu ("tahmin etme;
ara ya da yeniden oku", §9) · `memoryPressureWarn` ("şimdi yaz" uyarısı). Katlanan detay **birebir geri
alınabilir** → ham pencereyi küçültmek recall **kaybettirmez**, sadece dayanıklılığı "büyük pencere"den
"ucuz retrieval"a kaydırır ve `n²` rot yükünü azaltır.

```mermaid
graph LR
    A["Ham pencere ↑ (512K)"] --> B["n² ilişki ↑"]
    B --> C["recall hassasiyeti ↓ (context rot)"]
    A --> D["dayanıklılık (kaybolmama)"]
    E["retrieval katmanı:<br/>memory · conv_search · core"] --> D
    E -.zaten var.-> F["ham pencere küçülse de<br/>detay birebir kurtarılır"]
    style C fill:#d66,stroke:#900,color:#fff
    style E fill:#2d6,stroke:#093
```

**Benimsenen strateji — retrieval-destekli adaptif pencere.** Üç seçenek tartıldı: (A) sabit 512K
"her şeyi ham tut" — yüksek rot; (C) note-taking ağırlıklı agresif kısma (0.25/128K) — sık katlama,
"neden bu kadar erken özetledi" hissi; (B) **orta, retrieval-destekli pencere** — seçilen. Gradyan
uçurum değil → ne aşırı büyük (rot) ne aşırı küçük (gereksiz sık katlama) optimaldir.

| Knob | Eski | Yeni | Gerekçe |
|---|---|---|---|
| `ContextBudgetCeil` | 512K | **256K** (`262144`) | `n²` yükü ~¼; gradyanın yüksek-hassasiyet bölgesi |
| `ContextBudgetFraction` | 0.6 (sabit) | **0 = otomatik** → adaptif 0.35–0.45 | aile-bazlı rot toleransı |
| `memoryPressureWarn` | 0.75 | **0.70** | "şimdi yaz" penceresini katlamadan önce öne al |

**Adaptif fraction (`providers.AdaptiveBudgetFraction`).** `ContextWindowFor`'un aile sınıflamasını
yeniden kullanır → yeni model ailesi eklenince tek yerde güncellenir. Uzun-bağlam-güvenilir aileler
daha yüksek pay alır, küçük/bilinmeyen modeller muhafazakâr kalır:

| Aile | Pencere | Fraction | Etkin (ceil 256K) |
|---|---|---|---|
| Opus 4.8 / Sonnet 4.6 | 1M | 0.45 | 450K → **256K** (tavan) |
| Haiku 4.5 | 200K | 0.40 | **80K** |
| MiniMax / DeepSeek / Gemini | 1M | 0.35 | 350K → **256K** (tavan) |
| Fable / genel Claude | 200K | 0.40 | **80K** |
| Bilinmeyen | 0 | — | taban (`MaxContextTokens`) |

**Semantik & geriye-uyumluluk.** `ContextBudgetFraction = 0` artık **"otomatik/adaptif"** anlamına gelir
(negatif → 0'a clamp'lenir; pozitif → manuel sabit pay). `MaxContextTokens` (configured) **taban** olarak
korunur → kimse mevcut tabanının altına düşmez. `EffectiveBudget(provider, model, configured, fraction,
ceil)`: `fraction<=0` ise `AdaptiveBudgetFraction`, o da 0 ise paket fallback (0.4). `SetBudgetShape`
artık fraction 0'ı (auto) saklar (eskiden yok sayardı). Eski `settings.json`'larda kalan açık `0.6` değeri
**manuel sabit** olarak yaşamaya devam eder (kullanıcı sıfırlayana dek); yeni kurulumlar adaptif başlar —
her iki durumda da **ceil 256K** rot getirisini sağlar. Büyük ham pencere isteyen kullanıcı Ayarlar'dan
`ContextBudgetCeil`/`ContextBudgetFraction`'ı yükseltebilir; varsayılan artık **rot-bilinçli**.

**Sınırlar.** Bu bir *varsayılan politika* ayarıdır, sert sınır değil. claude-cli `--resume` warm modunda
bağlam yönetimi CLI'a geçer → bu bütçe o oturumda baypas edilir (bilinen gerilim, §11). Testler:
`budget_test.go` (`TestEffectiveBudgetAdaptive`), `context_window_test.go` (`TestAdaptiveBudgetFraction`).

## Ayrıca Bakınız

- **[35-CONTEXT-RESET-HANDOFF.md](35-CONTEXT-RESET-HANDOFF.md)** — Context reset + handoff artifact
  (bu in-place compaction'ın tamamlayıcısı: özet yerine temiz pencerede devam).
- **[19-LAZY-TOOL-LOADING.md](19-LAZY-TOOL-LOADING.md)** — Araç şemalarının talep üzerine yüklenmesi
  (sistem promptundan araç token yükü azaltmanın tamamlayıcı yolu): self-management + MCP araçları
  katalog özetiyle yayımlanır, `activate_tools` çağrılınca tam şema gelir.
