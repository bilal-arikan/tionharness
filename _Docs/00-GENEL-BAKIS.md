# TionHarness — Genel Bakış

> **Özet (2026-10-03):** Projenin amacı, teknoloji özeti, temel kavramları ve aktif/tarihsel doküman dizinidir. Uygulama canlıdır; ilk fazlar tamamlandı, memory alt sistemi 2026-07-05'te kaldırıldı. Güncel değişiklikler kısa `05-ILERLEME.md` günlüğünde, önceki aylar tarihli arşivlerde tutulur. GitHub kamusal yayın hattıdır; Gitea ayrı bir kendi barındırılan sürüm feed'ine yayın yapabilir. Merkezi ajan kataloğu, ayarlar ve on karar mercii ilgili rehberlerde açıklanır.

> **TionHarness**, Go diliyle, kendi UI/UX tasarımıyla sıfırdan yazılmış çok-ajanlı AI runtime'ıdır.

## Amaç

Açık kaynaklı, kendi sunucunda barındırılan (self-hosted) bir **çoklu-ajan (multi-agent) AI çalışma ortamı (runtime)** ve **kontrol düzlemi (control plane)** inşa etmek. Birden fazla otonom AI ajanını yöneten, görev dağıtan ve zamanlanmış işler çalıştıran bir sistem.

> **Public yayın (2026-08-27):** repo `https://github.com/bilal-arikan/tionharness`
> altında herkese açık, tanıtım sitesi `https://tionharness.com` adresinde canlı
> (GitHub Pages), lisans **Apache-2.0**. Kamusal yayın GitHub Actions üzerinden
> yapılır; Gitea'nın ayrı kendi barındırılan feed hattı kamusal feed'i değiştirmez.
> Ayrıntı `75-YAYIN-SURECI.md`.

## Neden Go?

| Kriter | Kazanım |
|--------|---------|
| **Eşzamanlılık (concurrency)** | Goroutine + channel modeli, çoklu-ajan orkestrasyonu için doğal |
| **Tek binary** | Bağımlılıksız, kolay dağıtım |
| **Düşük RAM / yüksek performans** | Electron/Node yükü olmadan |
| **Çapraz derleme** | Tek komutla Windows / macOS / Linux |

## Teknoloji Özeti

| Bileşen | TionHarness |
|---------|---------|
| Dil | Go 1.26.4+ |
| Masaüstü kabuk | Native WebView2 penceresi (`cmd/tionharness-desktop`, CGO'suz — Wails gereksizleşti; bkz. `32-NATIVE-PENCERE.md`) |
| Web framework | Bağımsız frontend + Go API; `dist/` binary'e `go:embed` ile gömülü |
| Depolama | Dosya sistemi — JSON/JSONL, DB yok (bkz. `08-DEPOLAMA.md`) |
| Orkestrasyon | Kendi state-machine + goroutine/channel |
| Boyut | ~10-20 MB hedef |

## Temel Kavramlar

- **Agent (Ajan):** Kalıcı kimlik ve araç erişimi olan otonom AI varlığı. Bir LLM sağlayıcı/modeline bağlanır.
- **Swarm (Sürü):** Delegasyon ile işbirliği yapan ajan toplulukları.
- **Session (Oturum):** Mesaj geçmişini ve bağlamı koruyan konuşma dizisi.
- **Task (Görev):** Yürütme politikaları, retry mantığı ve bağımlılıkları olan pano-tabanlı iş kuyruğu.
- **Provider (Sağlayıcı):** LLM uç noktası soyutlaması; her kind bir `internal/providers/kind_*.go` dosyasıdır (güncel liste için oraya bak).

## Doküman Dizini

- [89 — Asenkron kullanıcı soruları](89-ASENKRON-KULLANICI-SORULARI.md): soru cevabı
  beklenirken bağımsız çalışma, native/CLI cevap teslimi ve çoklu soru kartları.

| Doküman | İçerik |
|---------|--------|
| [00-GENEL-BAKIS.md](00-GENEL-BAKIS.md) | Bu dosya — projenin amacı ve özeti |
| [01-MIMARI.md](01-MIMARI.md) | Sistem mimarisi, katmanlar, modüller |
| [02-VERI-MODELI.md](02-VERI-MODELI.md) | Dosya-tabanlı varlık modeli ve ilişkileri |
| [03-YOL-HARITASI.md](03-YOL-HARITASI.md) | Aşama aşama (faz) geliştirme planı |
| [04-TEKNOLOJI-SECIMLERI.md](04-TEKNOLOJI-SECIMLERI.md) | Kütüphane seçimleri ve gerekçeleri |
| [05-ILERLEME.md](05-ILERLEME.md) | Yakın dönem ilerleme günlüğü (2026-09-28 ve sonrası; en yeni tarih üstte) ve önceki ayların arşiv bağlantıları |
| [05-ARSIV.md](05-ARSIV.md) | 2026-06-30 ve öncesinin tamamlanmış kayıtları; Temmuz–Eylül tarihli arşivler ana günlükte listelenir |
| [06-WORKSPACES.md](06-WORKSPACES.md) | Workspace izolasyonu tasarımı (fiziksel ayrım) |
| [07-CHAT-UX.md](07-CHAT-UX.md) | Zengin sohbet arayüzü + SSE adım-adım akış + clipboard/artifact görsel çizimi |
| [08-DEPOLAMA.md](08-DEPOLAMA.md) | Dosya-tabanlı depolama tasarımı (JSON/JSONL, DB yok) |
| [09-CLAUDE-AGENT-SDK.md](09-CLAUDE-AGENT-SDK.md) | Karar kaydı (ADR): Claude Agent SDK paritesi + claude-cli'yi köprü olarak benimseme |
| [11-INTERACTION-MCP.md](11-INTERACTION-MCP.md) | Interaction MCP: CLI ajanlara insan-etkileşimli araçlar (ask_user/todo_write/onay) |
| [12-LOGLAMA.md](12-LOGLAMA.md) | Loglama sistemi: slog ring buffer, /api/logs, access + iş logları, dış erişim |
| [15-FLOW-CANVAS.md](15-FLOW-CANVAS.md) | Görsel Flow Builder (React Flow canvas) |
| [16-PROFILLEME.md](16-PROFILLEME.md) | Profilleme (pprof) rehberi |
| [17-TOKEN-OPTIMIZASYON.md](17-TOKEN-OPTIMIZASYON.md) | Araç çıktısı token optimizasyonu (built-in sıkıştırma 2026-07-10'da kaldırıldı → harici `rtk`/`sqz`; harici araç tespiti + prompt-cache/bütçe konuları) |
| [18-HOOKS.md](18-HOOKS.md) | Hooks (PreToolUse / PostToolUse) — Faz P4 |
| [19-LAZY-TOOL-LOADING.md](19-LAZY-TOOL-LOADING.md) | Lazy tool loading (tasarım + uygulama) |
| [20-SCHEDULE-WAKE.md](20-SCHEDULE-WAKE.md) | `schedule_wake`: ajanın kendi sohbetine geri dönmesi |
| [21-MARKET.md](21-MARKET.md) | Uygulama içi market sistemi (marketplace) |
| [22-SPAWN-SESSION.md](22-SPAWN-SESSION.md) | Spawn session (fire-and-forget paralel işçi) |
| [24-SELF-MANAGEMENT.md](24-SELF-MANAGEMENT.md) | Self-management + ayarlar alt sistemi |
| [25-SUBAGENT-ISOLATION.md](25-SUBAGENT-ISOLATION.md) | Generic ajan yürütme çekirdeği + alt-ajan (subagent) izolasyonu |
| [26-CALISMA-DIZINI.md](26-CALISMA-DIZINI.md) | Çalışma dizini (working directory) — oturum-başına cwd |
| [27-CROSS-SESSION-SEARCH.md](27-CROSS-SESSION-SEARCH.md) | Oturumlar-arası tam-metin arama (`conversation_search`) |
| [28-PEER-MESAJLASMA-PLANI.md](28-PEER-MESAJLASMA-PLANI.md) | Ajanlar-arası peer mesajlaşma (send_message / mailbox) |
| [29-BILDIRIM-SINYALLERI.md](29-BILDIRIM-SINYALLERI.md) | Generic bildirim sinyalleri (nav + workspace) |
| [30-COKLU-PENCERE.md](30-COKLU-PENCERE.md) | Masaüstünde çoklu pencere (N süreç / N pencere) |
| [arsiv/31-MEMGPT-CORE-MEMORY.md](arsiv/31-MEMGPT-CORE-MEMORY.md) | ~~MemGPT/Letta tarzı self-editing çekirdek bellek~~ (**KALDIRILDI 2026-07-05**; 2026-07-27'de `arsiv/`'e taşındı — tarihsel referans, **31 numarası artık boş**) |
| [32-NATIVE-PENCERE.md](32-NATIVE-PENCERE.md) | Native masaüstü penceresi (WebView2, CGO'suz) |
| [33-DIS-AJAN-OTOMASYONU.md](33-DIS-AJAN-OTOMASYONU.md) | TionHarness'i dışarıdan (API/UI) sürme dostluğu |
| [34-YEDEKLEME.md](34-YEDEKLEME.md) | Workspace periyodik zip yedekleme + geri yükleme |
| [35-CONTEXT-RESET-HANDOFF.md](35-CONTEXT-RESET-HANDOFF.md) | Otonom turda context-reset / handoff |
| [36-KALICI-ILERLEME.md](36-KALICI-ILERLEME.md) | Kalıcı todo/PROGRESS dosyası (oturumlar-arası devralma) |
| [37-INGEST-MIMARISI.md](37-INGEST-MIMARISI.md) | Generic import (ingest) mimarisi — repo/plugin/local'den skill+agent+command+MCP keşfi |
| [38-SESSION-DEBUG.md](38-SESSION-DEBUG.md) | Oturum debug günlüğü (`debug.jsonl`) — paralel gözlemlenebilirlik + anomali tespiti |
| [39-DIZIN-SITE-REGISTRY.md](39-DIZIN-SITE-REGISTRY.md) | Dizin-sitesi köprüsü (search connector) — skill dizin sitelerinden markete arama/önizleme |
| [40-PLAN-MODE.md](40-PLAN-MODE.md) | Plan modu — claude-cli `ExitPlanMode` köprüsü + plan onay kartı |
| [41-ARAC-BOSLUKLARI-YAPILACAKLAR.md](41-ARAC-BOSLUKLARI-YAPILACAKLAR.md) | Araç boşlukları backlog'u (the external agent project↔TionHarness karşılaştırması) |
| [43-REWIND.md](43-REWIND.md) | `/rewind` — sohbet checkpoint geri sarma (yalnız-sohbet MVP) |
| [44-CODE-EXECUTION-MCP.md](44-CODE-EXECUTION-MCP.md) | Code Execution with MCP — occupancy'yi kökten düşürme fizibilite + faz planı |
| [45-COKLU-SECIM.md](45-COKLU-SECIM.md) | Çoklu seçim (Ctrl/Cmd+Click) + toplu eylemler (frontend-only; eski 40 numarasından taşındı) |
| [46-ETIKET-OTOMASYON.md](46-ETIKET-OTOMASYON.md) | Etiketler (session/flow/schedule tags) + etiket-tetikleyicili döngü otomasyonları |
| [47-KOORDINATOR-COKLU-AJAN.md](47-KOORDINATOR-COKLU-AJAN.md) | Koordinatör & çoklu-ajan koordinasyonu (M2 koordinatör/worker: async spawn_worker + task-notification) |
| [48-VPS-REMOTE-CLIENT.md](48-VPS-REMOTE-CLIENT.md) | VPS uzak sunucu + mobil ince istemci (PWA/WebView APK) — tek kullanıcı/VPN, dosya önizle-indir-editle (fizibilite/tasarım) |
| [49-MOBIL-RESPONSIVE-UI.md](49-MOBIL-RESPONSIVE-UI.md) | Mobil/dikey ekran uyumlu UI (responsive) — NavRail→alt tab bar, çok-panel→drawer/stack, full-screen sheet modallar; **2026-09-05'ten beri dört katmanlı kabuk** (narrow/square/wide/ultra + en-boy oranı: `useViewport`/`useShellLayout`, kare katmanda peek rail + drawer detay, ultra'da 88rem okuma ölçüsü, CSS durum geçişleri; tüm sol liste panelleri daraltılabilir, varsayılan açık, yeniden-açma rayı) |
| [50-CLAUDE-CODE-CACHE-PARITE.md](50-CLAUDE-CODE-CACHE-PARITE.md) | Claude Code prompt-cache davranış paritesi — cache breakpoint stratejisi, API-native context editing, cache-break tespiti/telemetrisi |
| [52-MCP-GATEWAY.md](52-MCP-GATEWAY.md) | MCP gateway (dinamik araç aktivasyonu, `list_changed` push) — **uygulandı** (Faz 0-4); ilk plan gövdesi `arsiv/52-MCP-GATEWAY-PLANLAMA.md` |
| [53-HARICI-AJAN-PROMPT-PARITE.md](53-HARICI-AJAN-PROMPT-PARITE.md) | the external agent project sistem-promptu paritesi: statik/dinamik bloklar — alınan (env marker, session_state, self-mgmt, deliverables) / bilinçli dışlanan (datatable/call_llm/render_template/_displayName) / farklı çözülen (recovery_context → dosya-tabanlı) |
| [54-CAPABILITY-PROBE.md](54-CAPABILITY-PROBE.md) | Generic capability probe → cachelenebilir context genişletme (opsiyonel harici tool varsa statik prefix'e kısa blok); ilk müşteri codebase-memory (tek cache root — per-workspace izole store 2026-08-11'de kaldırıldı) + best-effort cwd auto-index; zvec-grep anlamsal arama aynı katmanlarla (2026-09-14: prompt bloğu, `root` prefill, `[INDEX_MISSING]` onarımı, repo köküne otomatik indeks, allowlist muafiyeti) |
| [56-SELF-HEALING.md](56-SELF-HEALING.md) | Kendi kendini onaran oturum akışları: provider hata sınıflandırıcı + sınırlı retry (errclass), tool-loop guardrail (warn/block/halt), tur-içi mesaj dizisi onarımı (RepairSequence), kalıcı StuckTurns sayacı + `stuck` etiketi + otonom gate, hata→ders döngüsü (lessons) |
| [57-PROMPT-EPOCH.md](57-PROMPT-EPOCH.md) | Prompt Epoch — statik system + araç şemalarını (session,agent) başına dondurup oturum-ortası cache kırılmalarını önleme; context-change diff notu + cache-warmth göstergesi |
| [58-QUEUE-SENKRON.md](58-QUEUE-SENKRON.md) | Sohbet kuyruğu + çoklu-ekran senkronizasyonu (event-sourcing cutover): SessionHub cursor SSE + interaction CAS + durable send-queue + presence |
| [59-CLI-STEER-PLANI.md](59-CLI-STEER-PLANI.md) | claude-cli ajanlarında tur-ortası yönlendirme (steer). **UYGULANDI (2026-07-11, Faz 1+3)** — ancak 2026-07-13 notu: `auto` modda yapısal sınırlama var, doküman başındaki uyarıya bakın |
| [60-RETROSPEKTIF-TARAMA.md](60-RETROSPEKTIF-TARAMA.md) | Retrospektif geçmiş tarama (Insight Scan) — TASLAK/Faz1: editlenebilir lens dosyaları + inkremental ledger (UpdatedAt+fingerprint) + 3-aşamalı pipeline; iki kanal (app-fix / workspace-opt); manuel + cron tetik |
| [61-MERKEZI-PROMPT-REGISTRY.md](61-MERKEZI-PROMPT-REGISTRY.md) | Merkezi prompt registry (`internal/prompts`) — 15 gömülü prompt tek kayıt defterinde: embed edilmiş .md default'lar, workspace override + `{{yerTutucu}}` doğrulaması + default'a fallback, epoch rozeti, debug.jsonl prompt izi (promptKey/promptHash), drift-guard testi; sistem ajanına bağlı 14 prompt ekrandan çıkarıldı (Soul üzerinden düzenlenir) + soul editöründe "koddaki prompta dön" |
| [62-BIRLESIK-RUN-AWAIT.md](62-BIRLESIK-RUN-AWAIT.md) | Birleşik Run (C+D) — `await-input` keystone (flow durable suspend/resume: `State.WaitingAt` + `FlowWaiting` statüsü + CAS resume + input delivery API/UI); `LaunchRun` fresh-launch launcher seam'i (Faz 3); peer-bridge (`list_flow_runs`/`deliver_flow_input`); await timeout/GC sweeper; `subflow` node (senkron flow kompozisyonu). Flow motoru accumulate/loop/paralel-fold: [15-FLOW-CANVAS.md](15-FLOW-CANVAS.md) |
| [63-SOURCE-TEMPLATES-RENDER.md](63-SOURCE-TEMPLATES-RENDER.md) | Source template render (`render_template`) — kaynak-başına HTML şablonlarıyla tutarlı veri sunumu *(eski numara: 53)* |
| [65-DURABLE-ASK.md](65-DURABLE-ASK.md) | Durable Ask — native `ask_user` ve permission onayı temiz suspend noktasında diske park edilir (`SessionAsk` + CAS claim), cevap gelince tur kalıcı state'ten devam eder; `WithDurableAsk` gate'li, restart/crash'e dayanıklı |
| [66-VIEW-KATMANI.md](66-VIEW-KATMANI.md) | **Faz 1-4 + 6 canlı** — View (projeksiyon) katmanı: flow run / session / board durumunun bağlam-ucuz özeti (deterministik L0+L1, LLM yok). `tiny/card/full` bütçe tier'ları, birimli `Elided` (sessiz kesme yok), `get_view` aracı (pull kanalı) ve **ajanla aynı ham DSL'i gösteren `◱ Özet` paneli**; `workspace` roll-up'ı + grafiklerle **Panel (dashboard) ekranı** (`GET /api/dashboard`). Faz 5 (L2 incremental fold) tasarım |
| [67-BOARD-GORUNUMLERI.md](67-BOARD-GORUNUMLERI.md) | Board görünüm katmanı — facet filtre çubuğu (AND/OR semantiği, Türkçe I/ı arama katlaması), gruplama ekseni (durum/ajan/öncelik/etiket/tarih — sürükleme eksenin alanını yazar) ve workspace başına kayıtlı görünümler; aktif seçim pencere-yerel (localStorage) |
| [68-OZET-HARITASI.md](68-OZET-HARITASI.md) | **Faz 1-3 + TSK66 canlı** — Özet Haritası / Workspace Explorer: View katmanı üstünde kök düğümden tıkladıkça bir katman açılan semantic-zoom drill-down. Backend: `agent`/`budget`/`tools`/`category` + TSK66'da `artifact`/`automation`/`skill`/`insight`/`logs` Kind'leri + deterministik projeksiyonlar (LLM yok), `Children(ref)` + `GET /children`, ajan **`expand` aracı**, `Sources` (skills/findings/logs) + `WithSources`. Frontend: NavRail "Harita" ekranı (`features/explorer`), **2026-09-04'ten beri vis-network tek fizik ağı** (`GET /api/views/graph` tüm haritayı tek çağrıda verir; merkezde workspace, halkada 11 grup, üyeler gruba bağlı; tek tık = odak + gömülü özet paneli), canlı SSE, URL deep-link + harita-içi arama. Opsiyonel/ertelendi: MCP resource tree + sigma.js (büyük-workspace) |
| [69-CODEX-CLI-SAGLAYICI.md](69-CODEX-CLI-SAGLAYICI.md) | **Uygulandı; GPT-6 desteği, CLI erişim denemesi ve Desktop farkları §14 (2026-09-28); ilk uygulama notları §13** — OpenAI Codex CLI'yi claude-cli gibi arka planda sürme fizibilitesi. `codex exec` bayrak yüzeyi, `--json` JSONL olay şeması (`thread.*`/`turn.*`/`item.*`), `CODEX_HOME` config izolasyonu, `-c` TOML override'ları, MCP köprüsü uyumu (Streamable HTTP + Bearer, `mcp__srv__tool` namespace'i), `developer_instructions` sistem-prompt kanalı ve **27 maddelik parite matrisi**. İki gerçek boşluk: exec'te per-tool onay yok + native araç bastırma sınırlı. İlk doğrulama: `codex-cli 0.147.0` + `openai/codex` kaynak kodu; güncel 0.153.3/GPT-6 notları §14 |
| [69-CODEX-BENCHMARK-2026-09-28.md](69-CODEX-BENCHMARK-2026-09-28.md) | TionHarness taşıyıcısı ile doğrudan CLI karşılaştırması: Astra/Sol, üç görev, 12 deneme ve 96 kontrol; süre/token ölçümleri ve sınırlar |
| [69-CODEX-TOOL-BENCHMARK-2026-09-28.md](69-CODEX-TOOL-BENCHMARK-2026-09-28.md) | Ayrıntılı araç izleri: 48 adım, çağrı birleştirme, temizlik, süreler ve doğrulama derinliği; 96 test ve 2048 ek grafik kontrolü |
| [69-CODEX-ARAC-IZINLERI-2026-09-28.md](69-CODEX-ARAC-IZINLERI-2026-09-28.md) | CLI 0.157.1: MCP araç izin/yasakları ve native shell kontrolü uygulandı; joker sınırı ve kotasız 16 davranış testi |
| [71-SAGLAYICI-ORNEKLERI-PLANI.md](71-SAGLAYICI-ORNEKLERI-PLANI.md) | **Faz 0-5 BİTTİ (2026-08-18)**; §4.5 claude-cli kimlik sağlığı + yedek dışlama (eski 51'den) — Sağlayıcı taslak→örnek modeli; uygulama-geneli `providers.json`; ajanların örnek seçimi; yeni CLI örneklerinde otomatik `<dataDir>/provider-homes/<instance-id>` izolasyonu ve örnek-bazlı auth rotaları; legacy workspace auth fallback'i; `InstanceCatalog()` ile aynı kind'ın örneklerini ayrı gösterme. **§12 (2026-09-04):** yerel sağlayıcılar — LM Studio kind'ı, host tabanlı anahtarsız çalışma, yüklenen bağlam boyutuna göre muhafazakâr pencere, sıfır maliyet |
| [72-TANITIM-SITESI.md](72-TANITIM-SITESI.md) | **UYGULANDI (2026-08-25)** — `website/` altındaki statik tanıtım ve dokümantasyon sitesi (Astro 5 + Tailwind v4). Placeholder politikası (`site.config.ts`'te `null` = henüz yok → ölü link yerine "Coming soon"), bölüm akışı, uygulamadan elle senkronlanan tema dosyaları, `scripts\shots.ps1` Playwright screenshot hattı ve koda karşı doğrulanan içerik kaynakları |
| [73-LOKALIZASYON.md](73-LOKALIZASYON.md) | **Altyapı UYGULANDI (2026-08-25)** — UI i18n: `Settings.UILanguage` (ajan yanıt dilinden ayrı eksen, `""` = onu izle), i18next + feature-bazlı JSON katalogları, `Intl` biçimlendirme katmanı (`shared/lib/intl.ts`), dil değişiminde ağaç remount'u, katalog parite/çoğul guard testleri, migre klasörler için ESLint hardcoded-metin kapısı ve `npm run i18n:extract`. Kelime çevirileri kademeli |
| [74-SISTEM-AJANLARI.md](74-SISTEM-AJANLARI.md) | Sistem ajanları: canonical registry, kilitli yerleşik satır + özelleştirme çocuğu modeli (2026-09-03), çözümleme ve fallback, restore/disable semantiği, API/UI, özyineleme koruması ve usage taksonomisi |
| [75-YAYIN-SURECI.md](75-YAYIN-SURECI.md) | Kamusal GitHub Release + Pages hattı, Gitea'nın ayrı kendi barındırılan feed yayını, `latest.json` şeması ve yerel Docker önizlemesi |
| [76-ARTIFACT-SISTEMI.md](76-ARTIFACT-SISTEMI.md) | **Uygulandı (TSK476/TSK477)** — Artifact sistemi: image artifact'ları üzerine tarayıcıda çizim ve türetilmiş (`derivedFromArtifactId`) artifact olarak kaydetme |
| [77-ROTA-ALTYAPI-PLANI.md](77-ROTA-ALTYAPI-PLANI.md) | **UYGULANDI: R1–R10 (2026-09-02), üstüne Rota F0–F5 (2026-09-02/03)** — Rota (oturumları dinamik/çatallanan akış grafiğine hizalama + workspace canlı görünümü + koşu-sonu optimizer) öncesi altyapı hazırlığı: 10 refactor kalemi (oturum kökeni `SessionOrigin`, canlılık kaydı, workspace olay günlüğü, sidecar soyutlaması, otomasyon tetik registry + arşiv, reçete şeması, koordinasyon gözlemcisi, flow bağlantı düzeltmeleri, view/graph, frontend), üç dalga, kapı ölçütleri |
| [78-ROTA-EKRANI.md](78-ROTA-EKRANI.md) | **F0–F5 tamamlandı (2026-09-02/03)** — Rota ekranı: workspace-kök zaman ekseni kanvası (git-graf metaforu: kök şerit + worker/handoff alt şeritleri, `spawned`/`reported`/`forked_from` kenarları, akış koşusu çubukları, ⚡/↷/✕ işaretleri, kurulu zamanlayıcı gelecek şeridi), `laneStore` veri yolu, `GET /api/trajectories[/{id}]`, sağ panelde `ViewPanel`; F1a: rota varlığı üretimi (reçeteden tohum, R7 gözlemcisiyle spawn/rapor/akış koşusu/Ask kapısı bağlama), `trajectory` aracı (`get|plan|phase|finish`), `<trajectory>` durum bloğu; F1b: rota-içi faz-sütunlu görünüm + derin bağlantı, sohbet başlığında rol rozeti + mini rota şeridi, oturum kökeni çipi, Beceriler'de reçete çipleri, otomasyon ateşleme defteri, `ws:*` → toast köprüsü; F2: `phase`/`trajectory_end` tetikleri, reçete izleyicilerinin ateşlenmesi ve grafta "neden ateşlenmedi"; F3: rota bitiş özeti (süre/token/maliyet/hayalet faz/sessiz izleyici), reçete istatistikleri, LLM'siz haftalık küratör (arşivle/öner, provenance, pin) + Küratör paneli; F4: `recipe-optimizer` sistem ajanı, kodda zorlanan değişmezler (kanıt, büyüme bütçesi), `recipe-opt` içgörü kanalı (yalnız öneri); F5: faz kapıları (artifact/verdict/human = Durable Ask), kanvastan faz ekle/atla/tamamla, buradan çatalla, RunView → Rota; F4-v2 `auto_prune` oto-budama |
| [79-HARICI-API.md](79-HARICI-API.md) | Harici REST API (opt-in bearer auth) |
| [80-AJAN-REFERANSI.md](80-AJAN-REFERANSI.md) | **Ajan referansı (2026-09-03)** — CLAUDE.md'den taşınan ayrıntılar: internal/db haritası, codebase-memory `project`, deferred araçlar, Playwright, kısmi commit, test notları, `website/` |
| [81-PAKET-BOLME-PLANI.md](81-PAKET-BOLME-PLANI.md) | **Plan (2026-09-03)** — `internal/agent` (62k satır) ve `internal/api` (46k) paketlerinin alt paketlere bölünmesi: küme ölçümleri, bağımlılık yönü, aşamalı sıra ve kabul ölçütleri |
| [82-AJAN-KALITIMI.md](82-AJAN-KALITIMI.md) | **Uygulandı (2026-09-03)** — Ajan kalıtımı (`parentId` + alan bazlı `overrides`, okuma anında çözümleme), kilitli yerleşik sistem ajanları (`locked`, her boot'ta koddan dayatılır) ve rolü devralan özelleştirme çocukları; derive API'si, göç, UI (kalıtım şeritleri, alan alan devralındı/override rozetleri) |
| [84-CODEX-PLUGIN-DESTEGI.md](84-CODEX-PLUGIN-DESTEGI.md) | **Uygulandı (2026-09-14)** — `codex exec` turlarında Codex plugin'leri (skill / MCP sunucusu / hook taşıyan paketler): workspace ayarı `codexPluginsEnabled` (varsayılan açık), marketplace + plugin listeleri, her turun `config.toml`'una yazılan `[marketplaces]`/`[plugins]` anahtarları **ve** kalıcı sohbet evine bir kez yapılan kurulum (ikisi birden gerekir — ölçüldü), rezerve adlar için kopyala-yeniden adlandır içe aktarma akışı, `--strict-config` kenar doğrulaması, Windows yolu literal-string tuzağı, akışta skill'lerin iz bırakmaması |
| [83-EVRIM-MEKANIZMASI.md](83-EVRIM-MEKANIZMASI.md) | **Tarihsel araştırma ve tasarım; özellik 2026-09-29'da kaldırıldı.** Hedef güdümlü workspace evrimi için önceki yaklaşımı, uygulanmış eski fazları ve sonraki faz önerilerini kaydeder; güncel ürün davranışını anlatmaz. |
| [85-MONITOR-SOZLESMESI.md](85-MONITOR-SOZLESMESI.md) | **Uygulandı (2026-09-22)** — `monitor` aracı: arka plan kabuğunun çıktısını bir regex'e karşı izler ve eşleşmede ajanı uyandırır, böylece bekleyiş tur harcamaz. `MonitorSource` arayüzü (terminal durum tam bir kez, geçici hata yoklamayı kesmez), kendi bayt imlecini tutan kabuk kaynağı (`drainFrom` — `shell_manage` ile tam bağımsız okuma), `ScheduleWake`'ten çıkarılan ortak `armWake` üzerine kurulu `WakeNow` (tek teslim zinciri, en fazla bir kez), kapasiteler (8 monitör / 2 KB yük / 10 olay / 5 sn soğuma / `max_fires`), `ReleaseSessionRuntimeState` ile `monitorMgrs`+`shellMgrs`+`readTrackers` sızıntı düzeltmesi; **v2 (TSK941)** ile dosya (sandbox'tan geçen, `fsnotify` yerine `os.Stat` yoklaması — gerekçeli), URL (gövde değişiminde ateşler, 30 sn taban aralık) ve WebSocket (okuyucu goroutine + sınırlı kuyruk) kaynakları, `shell_id`/`path`/`url`'den tam birini isteyen kaynak seçimi, ve WebFetch ile paylaşılan SSRF korumalı dialer (`egress_guard.go`) |
| [86-DUYURU-FAN-OUT.md](86-DUYURU-FAN-OUT.md) | **Uygulandı (2026-09-22)** — Sürüm duyurusunu CI botu değil TionHarness'in kendi schedule + agent zinciri yapar (dogfood): `announce_release` aracı `_Docs/release.json`'u okur, manşet üretir (yalnız BREAKING/Features/Fixes, bölüm başına 5 giriş, 1800 rune sınırı, kırpmada notlar linki korunur) ve Discord webhook / Telegram Bot API'ye atar. Adresler **vault'ta** (`ANNOUNCE_*`), argümanda değil; açıkça istenen ama yapılandırılmamış kanal hatadır; araç idempotent **değildir** ve kısmi başarıda hangi kanallara çıktığını söyler; egress WebFetch ile aynı SSRF korumalı taşıyıcıdan geçer |
| [87-KARAR-KATMANI.md](87-KARAR-KATMANI.md) | Tipli karar modelleri ve on kayıtlı merci: dört güvenlik/akış noktası ile [92](92-JEV-IS-AKISLARI.md)'deki altı iş akışı. `off/shadow/on`, eşik, ana/yedek/rakip model; ortak `Pick` ve merciye özel soru kurucuları |
| [87-KARAR-DEBUG.md](87-KARAR-DEBUG.md) | **Uygulandı (2026-10-01)** — Karar debug: çağrı zaman çizelgesi, model/HTTP/yedek/rakip denemeleri, eşik ve olasılıklar, oturum/tur korelasyonu, sınırlı JSONL geçmişi, filtre/JSON dışa aktarımı ve `read_decider_debug`; dört mevcut kayıt ve canlı Jev testinden elde edilen fayda değerlendirmesi |
| [87-JEV-FIKIRLERI.md](87-JEV-FIKIRLERI.md) | **Öneri (2026-10-01)** — Alt ajan araştırması ve kullanıcı örneklerinden 20 kullanım fikri: compact/hatırlatma, toplu skill/tool seçimi, provider/model router, clarification, worker sentezi; her fikir için ölçüm ve hata yolu |
| [88-SUREC-IZLEME.md](88-SUREC-IZLEME.md) | **Uygulandı (2026-09-23)** — Süreç defteri (`internal/procwatch`): ajanlar adına başlatılan yerel süreçler (kabuk çağrıları, arka plan kabukları, `run_code`/`transform_data` yorumlayıcıları, claude-cli/codex-cli taşıyıcıları, stdio MCP sunucuları, hook'lar, harici araç koşuları) tek bir sınırlı defterde toplanır: komut satırı, PID, sahip (workspace/oturum/ajan), durum (`running`/`succeeded`/`failed`/`killed`/`timed_out`), çıkış kodu, 4 KB çıktı kuyruğu. Sahiplik tur ctx'ine tek yerden damgalanır; okuma `GET /api/workspace/processes` + salt okunur `list_processes` aracı + Workspace → **İşlemler** sekmesi (yüksüz `process` SSE frame'iyle canlı); durdurma yalnız kullanıcıya ait (onaylı), `shell_manage` kill'i de aynı kapıdan geçer |
| [90-MERKEZI-AGENT-KATALOGU.md](90-MERKEZI-AGENT-KATALOGU.md) | Merkezi ajan profilleri, workspace atamaları, kalıtım, onay ve `agent-catalog/` taşıma/yedekleme kuralları |
| [91-CLAUDE-55-DESTEGI.md](91-CLAUDE-55-DESTEGI.md) | **Uygulandı (2026-10-01)** — Sonnet 5.5 ve Opus 5.5: Anthropic API, Claude CLI ve OpenRouter model kimlikleri; adaptif/between-tools düşünme, imzalı geçmiş uyumluluğu, fiyat tahminleri ve abonelik tüketmeden yerel doğrulama |
| [MALIYET-DUSURME-PLANI.md](MALIYET-DUSURME-PLANI.md) | Maliyet düşürme planı — claude-cli batching/serial maliyet analizi ve aksiyonları |
| [AYARLAR-SADELESTIRME.md](AYARLAR-SADELESTIRME.md) | Ayar kategorileri, kategoriye özel kayıt davranışı ve eski bağlantıların yönlendirilmesi |
| [92-JEV-IS-AKISLARI.md](92-JEV-IS-AKISLARI.md) | **Uygulandı (2026-10-01)** — Session hazırlığı, model router, soru kontrolü, compact koruması, hatırlatma ve worker incelemesi; gruplu Karar Mercileri UI, oturum pin/feedback ve iz bağlantıları |
| [INSIGHT-BACKLOG.md](INSIGHT-BACKLOG.md) | **Otomatik üretilir** — Insight taramasının "app-fix" kanalı; uygulama-tarafı bulgu birikimi (elle düzenlenmez; bkz. [60](60-RETROSPEKTIF-TARAMA.md)) |
| [analiz-harici-ajan-arac-eslestirme.md](analiz-harici-ajan-arac-eslestirme.md) | the external agent project↔TionHarness araç eşleştirme analizi |
| [analiz-harici-baglam-yonetimi.md](analiz-harici-baglam-yonetimi.md) | Harici ajan ↔ TionHarness bağlam yönetimi karşılaştırması (salt analiz) |
| [TEMIZLIK-2026-10-03.md](TEMIZLIK-2026-10-03.md) | 65 maddelik depo temizliğinin sonuçları, değişiklikler ve doğrulama kanıtları |
| **arsiv/** | Tarihsel dokümanlar: kaldırılan özellikler, tamamlanan planlar ve ana dokümanlardan taşınan plan gövdeleri (referans/appendix) |
| [arsiv/05-ILERLEME-2026-07.md](arsiv/05-ILERLEME-2026-07.md) | Temmuz 2026 ilerleme kayıtları; içerik korunarak tarih sırasına kondu |
| [arsiv/05-ILERLEME-2026-08.md](arsiv/05-ILERLEME-2026-08.md) | Ağustos 2026 ilerleme kayıtları |
| [arsiv/05-ILERLEME-2026-09.md](arsiv/05-ILERLEME-2026-09.md) | 27 Eylül ve öncesinin Eylül 2026 ilerleme kayıtları |
| [arsiv/03-YOL-HARITASI-TAMAMLANAN.md](arsiv/03-YOL-HARITASI-TAMAMLANAN.md) | İlk fazlar ve tamamlanan işlerin tarih/commit/kanıt dökümü; eski açık işaretler tarihsel görüntüdür |
| [arsiv/68-OZET-HARITASI-ODAK-TASARIMI.md](arsiv/68-OZET-HARITASI-ODAK-TASARIMI.md) | Eski React Flow odak grafiği, erişilebilirlik kararları ve bileşen planı |
| [arsiv/HARICI-ARAC-GUNCELLEMELERI-2026-09-29.md](arsiv/HARICI-ARAC-GUNCELLEMELERI-2026-09-29.md) | Kişisel araç bakım raporu; bekleyen yönetici adımları/yerel yedekler korunur, Go uyumluluğu [04](04-TEKNOLOJI-SECIMLERI.md)'te |
| [arsiv/araclar/README.md](arsiv/araclar/README.md) | Tarihsel gateway prototipi, göç betikleri ve encoding onarım aracının kullanım sınırları |
| [Ajan ayarları tasarım arşivi](arsiv/tasarim/agent-edit-toggle-options/README.md) | Aktif rehbere bağlı olmayan ajan ayarları tasarım kaynakları |
| [arsiv/13-HARICI-AJANLAR-INCELEME.md](arsiv/13-HARICI-AJANLAR-INCELEME.md) | external-agent-oss release incelemesi → TionHarness çıkarımları |
| [arsiv/14-PROVIDER-MIMARISI-INCELEME.md](arsiv/14-PROVIDER-MIMARISI-INCELEME.md) | Çoklu-provider mimarisi incelemesi (gelecek plan) |
| [arsiv/23-ILISKI-GRAFIGI.md](arsiv/23-ILISKI-GRAFIGI.md) | ~~İlişki grafiği / Ağ ekranı~~ (**KALDIRILDI 2026-09-05**, yerini Harita [68](68-OZET-HARITASI.md) aldı; 2026-09-22'de `arsiv/`'e taşındı) |
| [arsiv/42-REFAKTOR-MODULERLIK.md](arsiv/42-REFAKTOR-MODULERLIK.md) | Refaktör/modülerlik — generic db/api/tools helper'ları + God-dosya bölmeleri (tamamlanan plan) |
| [arsiv/51-CLAUDE-CONFIG-BIRLESIK.md](arsiv/51-CLAUDE-CONFIG-BIRLESIK.md) | Eski per-workspace claude-home modeli (kaldırıldı); geçerli kimlik mekanikleri 71 §4.5'te |
| [arsiv/55-API-NATIVE-YOL-HARITASI.md](arsiv/55-API-NATIVE-YOL-HARITASI.md) | Anthropic API-native özellikler yol haritası — structured outputs, sunucu web search/fetch, server-side compaction, task budgets, native tool search, programmatic tool calling (P0–P4/P6/P7 tamam; **P5 "memory tool" 2026-07-05 hafıza kaldırma kararıyla iptal**) |
| [arsiv/64-HARICI-CLI-CHRONICLE.md](arsiv/64-HARICI-CLI-CHRONICLE.md) | an external CLI agent `/chronicle` oturum-içgörü ailesi (tips/improve/standup/cost-tips/search) + yerel SQLite session store; TionHarness muadilleriyle kıyas (salt referans, doküman-only) *(eski numara: 59)* |
| [arsiv/70-CODEX-CLI-UYGULAMA-PLANI.md](arsiv/70-CODEX-CLI-UYGULAMA-PLANI.md) | **Faz 1-4 canlı (kod merge edildi, 2026-08-18)** — `codex-cli` ProviderKind'ının faz faz uygulama planı ve gerçekleşen durumu: `CLIProvider` arayüz refactor'ı, `codexcli*.go` (provider + JSONL parser + config.toml renderer + hata sınıflandırma), `kind_codexcli.go` (Order 8, gpt-5.x katalog), registry/settings entegrasyonu, `codexmcp.go` MCP köprüsü, katalog sürüm probe'u + fiyatlandırma + frontend ayar kartı. Fark notları planın sonunda — güncel özgün bilgi 69 §13'te |
| [arsiv/03-YOL-HARITASI-KALDIRILAN.md](arsiv/03-YOL-HARITASI-KALDIRILAN.md) | 03'ten taşınan memory tabanlı bölümler (Faz 6, C3/C5/C6, HA-1) |
| [arsiv/04-TEKNOLOJI-ILK-PLAN.md](arsiv/04-TEKNOLOJI-ILK-PLAN.md) | 04'ün 2026-06-15 öncesi ilk teknoloji planı tabloları |
| [arsiv/11-INTERACTION-MCP-TARIHSEL.md](arsiv/11-INTERACTION-MCP-TARIHSEL.md) | 11'in plan/tarihsel bölümleri (tahmini `RunSession`, tahmini dosya listesi, oturum hedefi araçları) |
| [arsiv/25-SUBAGENT-ISOLATION-PLAN.md](arsiv/25-SUBAGENT-ISOLATION-PLAN.md) | 25'in plan gövdesi (primitif kader tablosu, dosya haritası, fazlama, test, riskler) |
| [arsiv/27-CROSS-SESSION-SEARCH-PLAN.md](arsiv/27-CROSS-SESSION-SEARCH-PLAN.md) | 27'nin Parça 1–3 plan gövdesi |
| [arsiv/30-COKLU-PENCERE-PLAN.md](arsiv/30-COKLU-PENCERE-PLAN.md) | 30'un plan gövdesi |
| [arsiv/32-NATIVE-PENCERE-PLAN.md](arsiv/32-NATIVE-PENCERE-PLAN.md) | 32'nin plan gövdesi (CGO analizi, taslaklar, build script) |
| [arsiv/39-DIZIN-SITE-REGISTRY-PLAN.md](arsiv/39-DIZIN-SITE-REGISTRY-PLAN.md) | 39'un §1 sonrası plan gövdesi |
| [arsiv/41-ARAC-BOSLUKLARI-TAMAMLANANLAR.md](arsiv/41-ARAC-BOSLUKLARI-TAMAMLANANLAR.md) | 41'in tamamlanan maddeleri (1/3/4/5/7/8 + Bölüm E) |
| [arsiv/44-CODE-EXECUTION-MCP-PLAN.md](arsiv/44-CODE-EXECUTION-MCP-PLAN.md) | 44'ün §5 fazlandırma ve §9 ilk faz gövdesi |
| [arsiv/47-KOORDINATOR-PLAN.md](arsiv/47-KOORDINATOR-PLAN.md) | 47'nin plan bölümleri (§1.2, §4–§8) |
| [arsiv/49-MOBIL-RESPONSIVE-UI-ANALIZ.md](arsiv/49-MOBIL-RESPONSIVE-UI-ANALIZ.md) | 49'un §1–§6 analiz/plan bölümleri |
| [arsiv/50-CLAUDE-CODE-CACHE-PARITE-PLAN.md](arsiv/50-CLAUDE-CODE-CACHE-PARITE-PLAN.md) | 50'nin P1–P7 "Değişim" maddeleri + §3–§7 |
| [arsiv/52-MCP-GATEWAY-PLANLAMA.md](arsiv/52-MCP-GATEWAY-PLANLAMA.md) | 52'nin uygulama öncesi analizi/faz planı (§0–§10) |
| [arsiv/59-CLI-STEER-PLANI-TASARIM.md](arsiv/59-CLI-STEER-PLANI-TASARIM.md) | 59'un ilk plan/tasarım bölümleri |

> **Numara notu:** Numara bir konu ailesini gösterir; 05, 69 ve 87 gibi ailelerde
> farklı alt rehberler aynı numarayı kullanabilir. Yeni bağımsız konuya sıradaki
> numara verilir. Ana dizindeki boş numaralar (10, 13–14, 23, 31, 42, 51, 55, 64, 70)
> tarihsel dokümanların `arsiv/`'e taşınmasından kalır.

## Kurulu Ortam

- ✅ Go 1.26.4+
- ✅ Node.js v24 + npm 11
- ✅ WebView2 runtime (native masaüstü penceresi; Wails planı iptal — `32-NATIVE-PENCERE.md`)

## Script'ler (`scripts\`)

> `.ps1` script'leri **ASCII-only tutulmalı** — WinPS 5.1 BOM'suz UTF-8'i ANSI çözer, parse bozulur. `.sh` script'leri Git Bash ile koşar.

| Script | Ne yapar | Detay |
|---|---|---|
| `dev.ps1` | Tek komutla dev: backend 8090 + frontend 5173 (Ctrl+C ikisini de indirir). `-Loopback` / `-BackendOnly` / `-FrontendOnly`. Backend **derlenip** (`bin\tionharness-dev.exe`) doğrudan çalıştırılır — `go run` sarmalayıcısı yok, öksüz sunucu yok, gerçek exit kodu görünür. Crash tanısı `_devlogs/`: backend stderr yakalaması (Go fatal/panic) + koşular arası `lifecycle.log` (kim neyi öldürdü); kendi kendine ölümde port süpürülür + uygulama logunun sonu ekrana basılır. Kendiliğinden çıkış satırları `reason=` ile **çözümlenmiş** exit kodu taşır (NTSTATUS/DBG); Vite `NODE_OPTIONS=--max-old-space-size=4096` ile koşar (uzun uptime'da heap sürüklenmesi) | [33](33-DIS-AJAN-OTOMASYONU.md) · [05](05-ILERLEME.md) |
| `build.ps1` | UI build + tek binary. `-Desktop` → `tionharness-desktop.exe` (`-H windowsgui`) | [32](32-NATIVE-PENCERE.md) |
| `serve.ps1` | Temiz build + tek binary'yi koşar (yalnız backend, Vite yok; gömülü SPA'yı sunar). `go run`'ın bayat link cache'i sorununu aşmak için açık `go build` yapar | — |
| `tailscale-serve.ps1` | Tailnet üzerinden **otomatik HTTPS** ile sunar (telefonda mikrofon/STT için güvenli bağlam). Ön planda koşar, çıkışta serve config'i söker | [48](48-VPS-REMOTE-CLIENT.md) |
| `worktree.ps1` | Geliştirici git worktree yardımcısı (`add`/`list`/`remove`/`prune`); node_modules junction'lar | [26](26-CALISMA-DIZINI.md) |
| `e2e-smoke.ps1` | 12 adımlı uçtan uca smoke testi (`-SkipLLM` ile hızlı/ucuz) | [33](33-DIS-AJAN-OTOMASYONU.md) |
| [repair-encoding.ps1](arsiv/araclar/repair-encoding.ps1) | Tarihsel UTF-8 onarım aracı; aktif geliştirme/derleme hattında kullanılmaz, varsayılan dry-run | [33](33-DIS-AJAN-OTOMASYONU.md) |
| `test.sh` | Test kapısı: `fast` (değişen Go paketleri + vitest) / `full` (`go test ./...` + vitest + `depcheck.sh` + `git diff --check`) | `CLAUDE.md` |
| `depcheck.sh` | `internal/agent`'tan ayrılan paketlerin onu geri import etmediğini doğrular (`test.sh full` koşar) | [81](81-PAKET-BOLME-PLANI.md) |
| `ci.sh` | CI ortak kapısı: `bash scripts/ci.sh backend`, `frontend` veya `release`; kurulum/derleme tekrarı önlenir | [75](75-YAYIN-SURECI.md) |
| `build-release.sh` | Sürüm derlemesi (çoklu hedef; CI `release.yml` kullanır) | [75](75-YAYIN-SURECI.md) |
| `install.sh` / `install.ps1` | Sürüm feed'inden binary indirip kuran kurulum script'leri | [75](75-YAYIN-SURECI.md) |
| `shots.ps1` | Tanıtım sitesi için ürün ekran görüntüleri (çalışan örneğe bağlanır, hash rotalarını gezer, `website/public/shots/`'a yazar). Playwright talep üzerine kurulur: `-InstallDeps` | [72](72-TANITIM-SITESI.md) |

## Proje Durumu

Güncel değişiklikler [05-ILERLEME.md](05-ILERLEME.md) içindedir; önceki ayların
bağlantıları aynı dosyanın başındadır. 2026-06-30 ve öncesi [05-ARSIV.md](05-ARSIV.md)
dosyasında korunur. Açık işler [03-YOL-HARITASI.md](03-YOL-HARITASI.md) içindedir.
Memory (hafıza) alt sistemi 2026-07-05'te tamamen kaldırıldı.
