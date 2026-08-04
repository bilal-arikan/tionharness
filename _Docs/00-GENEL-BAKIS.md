# TionSwarm — Genel Bakış

> **TionSwarm**, Go diliyle, kendi UI/UX tasarımıyla sıfırdan yazılmış çok-ajanlı AI runtime'ıdır.

## Amaç

Açık kaynaklı, kendi sunucunda barındırılan (self-hosted) bir **çoklu-ajan (multi-agent) AI çalışma ortamı (runtime)** ve **kontrol düzlemi (control plane)** inşa etmek. Birden fazla otonom AI ajanını yöneten, görev dağıtan ve zamanlanmış işler çalıştıran bir sistem.

## Neden Go?

| Kriter | Kazanım |
|--------|---------|
| **Eşzamanlılık (concurrency)** | Goroutine + channel modeli, çoklu-ajan orkestrasyonu için doğal |
| **Tek binary** | Bağımlılıksız, kolay dağıtım |
| **Düşük RAM / yüksek performans** | Electron/Node yükü olmadan |
| **Çapraz derleme** | Tek komutla Windows / macOS / Linux |

## Teknoloji Özeti

| Bileşen | TionSwarm |
|---------|---------|
| Dil | Go 1.26+ |
| Masaüstü kabuk | Native WebView2 penceresi (`cmd/tionswarm-desktop`, CGO'suz — Wails gereksizleşti; bkz. `32-NATIVE-PENCERE.md`) |
| Web framework | Bağımsız frontend + Go API; `dist/` binary'e `go:embed` ile gömülü |
| Depolama | Dosya sistemi — JSON/JSONL, DB yok (bkz. `08-DEPOLAMA.md`) |
| Orkestrasyon | Kendi state-machine + goroutine/channel |
| Boyut | ~10-20 MB hedef |

## Temel Kavramlar

- **Agent (Ajan):** Kalıcı kimlik ve araç erişimi olan otonom AI varlığı. Bir LLM sağlayıcı/modeline bağlanır.
- **Swarm (Sürü):** Delegasyon ile işbirliği yapan ajan toplulukları.
- **Session (Oturum):** Mesaj geçmişini ve bağlamı koruyan konuşma dizisi.
- **Task (Görev):** Yürütme politikaları, retry mantığı ve bağımlılıkları olan pano-tabanlı iş kuyruğu.
- **Provider (Sağlayıcı):** LLM uç noktası soyutlaması (5 kind: `anthropic`, `claude-cli`, `minimax`, `minimax-anthropic`, `openrouter`).

## Doküman Dizini

| Doküman | İçerik |
|---------|--------|
| [00-GENEL-BAKIS.md](00-GENEL-BAKIS.md) | Bu dosya — projenin amacı ve özeti |
| [01-MIMARI.md](01-MIMARI.md) | Sistem mimarisi, katmanlar, modüller |
| [02-VERI-MODELI.md](02-VERI-MODELI.md) | Veritabanı tabloları ve veri modeli |
| [03-YOL-HARITASI.md](03-YOL-HARITASI.md) | Aşama aşama (faz) geliştirme planı |
| [04-TEKNOLOJI-SECIMLERI.md](04-TEKNOLOJI-SECIMLERI.md) | Kütüphane seçimleri ve gerekçeleri |
| [05-ILERLEME.md](05-ILERLEME.md) | Yapılanlar / sıradaki adımlar takibi (**canlı durum** — 2026-07-01'den bugüne; en yeni kayıt üstte) |
| [05-ARSIV.md](05-ARSIV.md) | İlerleme arşivi (**2026-06-30 ve öncesi** tamamlanmış kayıtlar). Kesim 2026-07-27'de 06-19'dan 06-30'a taşındı — ana dosya ay-başı sınırında tutulur |
| [06-WORKSPACES.md](06-WORKSPACES.md) | Workspace izolasyonu tasarımı (fiziksel ayrım) |
| [07-CHAT-UX.md](07-CHAT-UX.md) | Zengin sohbet arayüzü + SSE adım-adım akış |
| [08-DEPOLAMA.md](08-DEPOLAMA.md) | Dosya-tabanlı depolama tasarımı (JSON/JSONL, DB yok) |
| [09-CLAUDE-AGENT-SDK.md](09-CLAUDE-AGENT-SDK.md) | Karar kaydı (ADR): Claude Agent SDK paritesi + claude-cli'yi köprü olarak benimseme |
| [10-KAVRAMSAL-TASARIM-NOTLARI.md](10-KAVRAMSAL-TASARIM-NOTLARI.md) | Kavramsal tasarım notları kataloğu (ClaudeCode mimarisi → TionSwarm, taslak/yol haritası) |
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
| [23-ILISKI-GRAFIGI.md](23-ILISKI-GRAFIGI.md) | İlişki grafiği: workspace ağı (vis-network) |
| [24-SELF-MANAGEMENT.md](24-SELF-MANAGEMENT.md) | Self-management + ayarlar alt sistemi |
| [25-SUBAGENT-ISOLATION.md](25-SUBAGENT-ISOLATION.md) | Generic ajan yürütme çekirdeği + alt-ajan (subagent) izolasyonu |
| [26-CALISMA-DIZINI.md](26-CALISMA-DIZINI.md) | Çalışma dizini (working directory) — oturum-başına cwd |
| [27-CROSS-SESSION-SEARCH.md](27-CROSS-SESSION-SEARCH.md) | Oturumlar-arası tam-metin arama (`conversation_search`) |
| [28-PEER-MESAJLASMA-PLANI.md](28-PEER-MESAJLASMA-PLANI.md) | Ajanlar-arası peer mesajlaşma (send_message / mailbox) |
| [29-BILDIRIM-SINYALLERI.md](29-BILDIRIM-SINYALLERI.md) | Generic bildirim sinyalleri (nav + workspace) |
| [30-COKLU-PENCERE.md](30-COKLU-PENCERE.md) | Masaüstünde çoklu pencere (N süreç / N pencere) |
| [arsiv/31-MEMGPT-CORE-MEMORY.md](arsiv/31-MEMGPT-CORE-MEMORY.md) | ~~MemGPT/Letta tarzı self-editing çekirdek bellek~~ (**KALDIRILDI 2026-07-05**; 2026-07-27'de `arsiv/`'e taşındı — tarihsel referans, **31 numarası artık boş**) |
| [32-NATIVE-PENCERE.md](32-NATIVE-PENCERE.md) | Native masaüstü penceresi (WebView2, CGO'suz) |
| [33-DIS-AJAN-OTOMASYONU.md](33-DIS-AJAN-OTOMASYONU.md) | TionSwarm'yu dışarıdan (API/UI) sürme dostluğu |
| [34-YEDEKLEME.md](34-YEDEKLEME.md) | Workspace periyodik zip yedekleme + geri yükleme |
| [35-CONTEXT-RESET-HANDOFF.md](35-CONTEXT-RESET-HANDOFF.md) | Otonom turda context-reset / handoff |
| [36-KALICI-ILERLEME.md](36-KALICI-ILERLEME.md) | Kalıcı todo/PROGRESS dosyası (oturumlar-arası devralma) |
| [37-INGEST-MIMARISI.md](37-INGEST-MIMARISI.md) | Generic import (ingest) mimarisi — repo/plugin/local'den skill+agent+command+MCP keşfi |
| [38-SESSION-DEBUG.md](38-SESSION-DEBUG.md) | Oturum debug günlüğü (`debug.jsonl`) — paralel gözlemlenebilirlik + anomali tespiti |
| [39-DIZIN-SITE-REGISTRY.md](39-DIZIN-SITE-REGISTRY.md) | Dizin-sitesi köprüsü (search connector) — skill dizin sitelerinden markete arama/önizleme |
| [40-PLAN-MODE.md](40-PLAN-MODE.md) | Plan modu — claude-cli `ExitPlanMode` köprüsü + plan onay kartı |
| [41-ARAC-BOSLUKLARI-YAPILACAKLAR.md](41-ARAC-BOSLUKLARI-YAPILACAKLAR.md) | Araç boşlukları backlog'u (the external agent project↔TionSwarm karşılaştırması) |
| [42-REFAKTOR-MODULERLIK.md](42-REFAKTOR-MODULERLIK.md) | Refaktör/modülerlik — generic db/api/tools helper'ları + God-dosya bölmeleri |
| [43-REWIND.md](43-REWIND.md) | `/rewind` — sohbet checkpoint geri sarma (yalnız-sohbet MVP) |
| [44-CODE-EXECUTION-MCP.md](44-CODE-EXECUTION-MCP.md) | Code Execution with MCP — occupancy'yi kökten düşürme fizibilite + faz planı |
| [45-COKLU-SECIM.md](45-COKLU-SECIM.md) | Çoklu seçim (Ctrl/Cmd+Click) + toplu eylemler (frontend-only; eski 40 numarasından taşındı) |
| [46-ETIKET-OTOMASYON.md](46-ETIKET-OTOMASYON.md) | Etiketler (session/flow/schedule tags) + etiket-tetikleyicili döngü otomasyonları |
| [47-KOORDINATOR-COKLU-AJAN.md](47-KOORDINATOR-COKLU-AJAN.md) | Koordinatör & çoklu-ajan koordinasyonu (M2 koordinatör/worker: async spawn_worker + task-notification) |
| [48-VPS-REMOTE-CLIENT.md](48-VPS-REMOTE-CLIENT.md) | VPS uzak sunucu + mobil ince istemci (PWA/WebView APK) — tek kullanıcı/VPN, dosya önizle-indir-editle (fizibilite/tasarım) |
| [49-MOBIL-RESPONSIVE-UI.md](49-MOBIL-RESPONSIVE-UI.md) | Mobil/dikey ekran uyumlu UI (responsive) — NavRail→alt tab bar, çok-panel→drawer/stack, full-screen sheet modallar (48-F4 derinleştirme, fizibilite/tasarım) |
| [50-CLAUDE-CODE-CACHE-PARITE.md](50-CLAUDE-CODE-CACHE-PARITE.md) | Claude Code prompt-cache davranış paritesi — cache breakpoint stratejisi, API-native context editing, cache-break tespiti/telemetrisi |
| [51-CLAUDE-CONFIG-BIRLESIK.md](51-CLAUDE-CONFIG-BIRLESIK.md) | Per-workspace claude-cli config evi (`<workspace>/claude-home` = CLAUDE_CONFIG_DIR, yalnız login/settings); global home'dan tohumlama migration'ı + yedekten credential hariç tutma. (Faz 2/3 — skill'i claude-home'a taşıma + native Skill — geri alındı; skill'ler `<workspace>/skills`'te, tek yol use_skill) |
| [52-MCP-GATEWAY.md](52-MCP-GATEWAY.md) | MCP gateway (dinamik araç aktivasyonu) tasarım/analiz |
| [53-CRAFTAGENT-PROMPT-PARITE.md](53-CRAFTAGENT-PROMPT-PARITE.md) | the external agent project sistem-promptu paritesi: statik/dinamik bloklar — alınan (env marker, session_state, self-mgmt, deliverables) / bilinçli dışlanan (datatable/call_llm/render_template/_displayName) / farklı çözülen (recovery_context → dosya-tabanlı) |
| [54-CAPABILITY-PROBE.md](54-CAPABILITY-PROBE.md) | Generic capability probe → cachelenebilir context genişletme (opsiyonel harici tool varsa statik prefix'e kısa blok); ilk müşteri codebase-memory + per-workspace izole store (`CBM_CACHE_DIR`) + best-effort cwd auto-index |
| [55-API-NATIVE-YOL-HARITASI.md](55-API-NATIVE-YOL-HARITASI.md) | Anthropic API-native özellikler yol haritası — structured outputs, sunucu web search/fetch, server-side compaction, task budgets, native tool search, programmatic tool calling (P0–P4/P6/P7 tamam; **P5 "memory tool" 2026-07-05 hafıza kaldırma kararıyla iptal**) |
| [56-SELF-HEALING.md](56-SELF-HEALING.md) | Kendi kendini onaran oturum akışları: provider hata sınıflandırıcı + sınırlı retry (errclass), tool-loop guardrail (warn/block/halt), tur-içi mesaj dizisi onarımı (RepairSequence), kalıcı StuckTurns sayacı + `stuck` etiketi + otonom gate, hata→ders döngüsü (lessons) |
| [57-PROMPT-EPOCH.md](57-PROMPT-EPOCH.md) | Prompt Epoch — statik system + araç şemalarını (session,agent) başına dondurup oturum-ortası cache kırılmalarını önleme; context-change diff notu + cache-warmth göstergesi |
| [58-QUEUE-SENKRON.md](58-QUEUE-SENKRON.md) | Sohbet kuyruğu + çoklu-ekran senkronizasyonu (event-sourcing cutover): SessionHub cursor SSE + interaction CAS + durable send-queue + presence |
| [59-CLI-STEER-PLANI.md](59-CLI-STEER-PLANI.md) | claude-cli ajanlarında tur-ortası yönlendirme (steer). **UYGULANDI (2026-07-11, Faz 1+3)** — ancak 2026-07-13 notu: `auto` modda yapısal sınırlama var, doküman başındaki uyarıya bakın |
| [60-RETROSPEKTIF-TARAMA.md](60-RETROSPEKTIF-TARAMA.md) | Retrospektif geçmiş tarama (Insight Scan) — TASLAK/Faz1: editlenebilir lens dosyaları + inkremental ledger (UpdatedAt+fingerprint) + 3-aşamalı pipeline; iki kanal (app-fix / workspace-opt); manuel + cron tetik |
| [61-MERKEZI-PROMPT-REGISTRY.md](61-MERKEZI-PROMPT-REGISTRY.md) | Merkezi prompt registry (`internal/prompts`) — 15 gömülü prompt tek kayıt defterinde: embed edilmiş .md default'lar, workspace override + `{{yerTutucu}}` doğrulaması + default'a fallback, epoch rozeti, debug.jsonl prompt izi (promptKey/promptHash), drift-guard testi |
| [62-BIRLESIK-RUN-AWAIT.md](62-BIRLESIK-RUN-AWAIT.md) | Birleşik Run (C+D) — `await-input` keystone (flow durable suspend/resume: `State.WaitingAt` + `FlowWaiting` statüsü + CAS resume + input delivery API/UI); `LaunchRun` fresh-launch launcher seam'i (Faz 3); peer-bridge (`list_flow_runs`/`deliver_flow_input`); await timeout/GC sweeper; `subflow` node (senkron flow kompozisyonu). Flow motoru accumulate/loop/paralel-fold + session↔flow reify: [15-FLOW-CANVAS.md](15-FLOW-CANVAS.md) |
| [63-SOURCE-TEMPLATES-RENDER.md](63-SOURCE-TEMPLATES-RENDER.md) | Source template render (`render_template`) — kaynak-başına HTML şablonlarıyla tutarlı veri sunumu *(eski numara: 53)* |
| [64-GITHUB-COPILOT-CHRONICLE.md](64-GITHUB-COPILOT-CHRONICLE.md) | an external CLI agent `/chronicle` oturum-içgörü ailesi (tips/improve/standup/cost-tips/search) + yerel SQLite session store; TionSwarm muadilleriyle kıyas (salt referans, doküman-only) *(eski numara: 59)* |
| [65-DURABLE-ASK.md](65-DURABLE-ASK.md) | Durable Ask — native `ask_user` ve permission onayı temiz suspend noktasında diske park edilir (`SessionAsk` + CAS claim), cevap gelince tur kalıcı state'ten devam eder; `WithDurableAsk` gate'li, restart/crash'e dayanıklı |
| [66-VIEW-KATMANI.md](66-VIEW-KATMANI.md) | **Faz 1-4 + 6 canlı** — View (projeksiyon) katmanı: flow run / session / board durumunun bağlam-ucuz özeti (deterministik L0+L1, LLM yok). `tiny/card/full` bütçe tier'ları + 4 lens, birimli `Elided` (sessiz kesme yok), `get_view` aracı (pull kanalı) ve **ajanla aynı ham DSL'i gösteren `◱ Özet` paneli**; `workspace` roll-up'ı + grafiklerle **Panel (dashboard) ekranı** (`GET /api/dashboard`). Faz 5 (L2 incremental fold) tasarım |
| [67-BOARD-GORUNUMLERI.md](67-BOARD-GORUNUMLERI.md) | Board görünüm katmanı — facet filtre çubuğu (AND/OR semantiği, Türkçe I/ı arama katlaması), gruplama ekseni (durum/ajan/öncelik/etiket/tarih — sürükleme eksenin alanını yazar) ve workspace başına kayıtlı görünümler; aktif seçim pencere-yerel (localStorage) |
| [MALIYET-DUSURME-PLANI.md](MALIYET-DUSURME-PLANI.md) | Maliyet düşürme planı — claude-cli batching/serial maliyet analizi ve aksiyonları |
| [INSIGHT-BACKLOG.md](INSIGHT-BACKLOG.md) | **Otomatik üretilir** — Insight taramasının "app-fix" kanalı; uygulama-tarafı bulgu birikimi (elle düzenlenmez; bkz. [60](60-RETROSPEKTIF-TARAMA.md)) |
| [analiz-craftagent-arac-eslestirme.md](analiz-craftagent-arac-eslestirme.md) | the external agent project↔TionSwarm araç eşleştirme analizi |
| **arsiv/** | Tarihsel inceleme dokümanları (referans/appendix) |
| [arsiv/13-CRAFT-AGENTS-INCELEME.md](arsiv/13-CRAFT-AGENTS-INCELEME.md) | external-agent-oss release incelemesi → TionSwarm çıkarımları |
| [arsiv/14-PROVIDER-MIMARISI-INCELEME.md](arsiv/14-PROVIDER-MIMARISI-INCELEME.md) | Çoklu-provider mimarisi incelemesi (gelecek plan) |

> **Numara notu:** 13–14 tarihsel inceleme dokümanları `arsiv/` altına taşındı (ana dizinde
> 13–14 boş). 17/18/26 eski numara çakışmaları giderildi → native pencere **32**, çoklu
> pencere **30**, MemGPT çekirdek bellek **31**. 40 çakışması giderildi (2026-07-02) →
> plan modu **40**, çoklu seçim **45**. **Numara çakışmaları giderildi (2026-07-27):**
> source-templates render **53 → 63**, an external CLI agent chronicle **59 → 64**.
> Böylece her numara tek dosyaya karşılık gelir; **53** = the external agent project prompt paritesi,
> **59** = CLI steer planı.

## Kurulu Ortam

- ✅ Go 1.26+
- ✅ Node.js v24 + npm 11
- ✅ WebView2 runtime (native masaüstü penceresi; Wails planı iptal — `32-NATIVE-PENCERE.md`)

## Script'ler (`scripts\`)

> Hepsi PowerShell. **ASCII-only tutulmalı** — WinPS 5.1 BOM'suz UTF-8'i ANSI çözer, parse bozulur.

| Script | Ne yapar | Detay |
|---|---|---|
| `dev.ps1` | Tek komutla dev: backend 8090 + frontend 5173 (Ctrl+C ikisini de indirir). `-Loopback` / `-BackendOnly` / `-FrontendOnly`. Crash tanısı `_devlogs/`: backend stderr yakalaması (Go fatal/panic) + koşular arası `lifecycle.log` (kim neyi öldürdü) | [33](33-DIS-AJAN-OTOMASYONU.md) · [05](05-ILERLEME.md) |
| `build.ps1` | UI build + tek binary. `-Desktop` → `tionswarm-desktop.exe` (`-H windowsgui`) | [32](32-NATIVE-PENCERE.md) |
| `serve.ps1` | Temiz build + tek binary'yi koşar (yalnız backend, Vite yok; gömülü SPA'yı sunar). `go run`'ın bayat link cache'i sorununu aşmak için açık `go build` yapar | — |
| `tailscale-serve.ps1` | Tailnet üzerinden **otomatik HTTPS** ile sunar (telefonda mikrofon/STT için güvenli bağlam). Ön planda koşar, çıkışta serve config'i söker | [48](48-VPS-REMOTE-CLIENT.md) |
| `worktree.ps1` | Geliştirici git worktree yardımcısı (`add`/`list`/`remove`/`prune`); node_modules junction'lar | [26](26-CALISMA-DIZINI.md) |
| `e2e-smoke.ps1` | 12 adımlı uçtan uca smoke testi (`-SkipLLM` ile hızlı/ucuz) | [33](33-DIS-AJAN-OTOMASYONU.md) |
| `repair-encoding.ps1` | Bozuk UTF-8 kayıtlarını onarır (varsayılan dry-run, `-Apply`) | [33](33-DIS-AJAN-OTOMASYONU.md) |

## Proje Durumu (2026-07-27)

> Bu bölüm **periyodik anlık görüntüdür**; günlük ilerleme `05-ILERLEME.md`'de tutulur.
> Bir tarihe takılmadan önce oradaki en son kaydı kontrol edin.

✅ **Faz 0–8 + kapsamlı backlog tamamlandı:** İskelet · DB/Config · Provider+Chat (5 kind: anthropic/claude-cli/minimax/minimax-anthropic/openrouter) · React Web UI · Agent Runtime · Workspace İzolasyonu · Tasks+Schedules · Sağlamlaştırma (compaction + guardrail) · Tool-use+MCP · Orchestration (akışlar) · Lazy tool yükleme · Prefix'li ID'ler (WS/AGT/SES) · İlişki grafiği · Self-management suite · Hooks · İzin modeli · native masaüstü penceresi + çoklu pencere · oturum-başına cwd · MCP kalıcı bağlantı havuzu (+hibrit `shared`/`scoped` kapsam) · oturumlar-arası arama · peer mesajlaşma · bildirim sinyalleri.

✅ **Sonraki dalga (07-10 → 07-27):** merkezi prompt registry ([61](61-MERKEZI-PROMPT-REGISTRY.md)) · retrospektif tarama/Insight kokpiti ([60](60-RETROSPEKTIF-TARAMA.md)) · sohbet kuyruğu + çoklu-ekran event-sourcing cutover ([58](58-QUEUE-SENKRON.md)) · in-process `sqz` shell filtresi ([17](17-TOKEN-OPTIMIZASYON.md)) · Flow motoru: accumulate cache + `loop` + session↔flow köprüsü, Birleşik Run `await-input`/`subflow`, Start/End node'ları + çıktı sözleşmesi ([15](15-FLOW-CANVAS.md) · [62](62-BIRLESIK-RUN-AWAIT.md)) · Claude Opus 5 model desteği.

> **Not (2026-07-05):** Memory (hafıza) alt sistemi — journal recall + MemGPT/Letta tarzı core memory + hafıza grafiği + ilgili tool/API/UI/veri — projeden **tamamen kaldırıldı**. Detay: `05-ILERLEME.md`.

➡️ **Sıradaki:** API-native kalan UI boşlukları (deferred-katalog rozeti, task-budget tur rozeti; [55](55-API-NATIVE-YOL-HARITASI.md) — *P5 "memory tool" maddesi 2026-07-05 kaldırma kararıyla düştü*) · gerçek byte-tasarrufu ölçümü: sqz delta'sını `runPostToolHooks`'ta ölçmek ([38](38-SESSION-DEBUG.md)) · claude-cli batching maliyet kıyası, koşul başına n≥5 ([MALIYET-DUSURME-PLANI](MALIYET-DUSURME-PLANI.md)) · Code Execution with MCP Faz 4/5 ([44](44-CODE-EXECUTION-MCP.md)) · araç backlog'u açık kalemler: `call_llm`, Monitor, Worktree ([41](41-ARAC-BOSLUKLARI-YAPILACAKLAR.md)). Detay: [05-ILERLEME.md](05-ILERLEME.md) · arşiv: [05-ARSIV.md](05-ARSIV.md).
