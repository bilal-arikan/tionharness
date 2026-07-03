# SwarmGo — Genel Bakış

> **SwarmGo**, Go diliyle, kendi UI/UX tasarımıyla sıfırdan yazılmış çok-ajanlı AI runtime'ıdır.

## Amaç

Açık kaynaklı, kendi sunucunda barındırılan (self-hosted) bir **çoklu-ajan (multi-agent) AI çalışma ortamı (runtime)** ve **kontrol düzlemi (control plane)** inşa etmek. Birden fazla otonom AI ajanını yöneten, görev dağıtan, hafıza tutan ve zamanlanmış işler çalıştıran bir sistem.

## Neden Go?

| Kriter | Kazanım |
|--------|---------|
| **Eşzamanlılık (concurrency)** | Goroutine + channel modeli, çoklu-ajan orkestrasyonu için doğal |
| **Tek binary** | Bağımlılıksız, kolay dağıtım |
| **Düşük RAM / yüksek performans** | Electron/Node yükü olmadan |
| **Çapraz derleme** | Tek komutla Windows / macOS / Linux |

## Teknoloji Özeti

| Bileşen | SwarmGo |
|---------|---------|
| Dil | Go 1.26+ |
| Masaüstü kabuk | Native WebView2 penceresi (`cmd/swarmgo-desktop`, CGO'suz — Wails gereksizleşti; bkz. `32-NATIVE-PENCERE.md`) |
| Web framework | Bağımsız frontend + Go API; `dist/` binary'e `go:embed` ile gömülü |
| Depolama | Dosya sistemi — JSON/JSONL, DB yok (bkz. `08-DEPOLAMA.md`) |
| Orkestrasyon | Kendi state-machine + goroutine/channel |
| Boyut | ~10-20 MB hedef |

## Temel Kavramlar

- **Agent (Ajan):** Kalıcı kimlik, hafıza ve araç erişimi olan otonom AI varlığı. Bir LLM sağlayıcı/modeline bağlanır.
- **Swarm (Sürü):** Delegasyon ve paylaşımlı hafıza ile işbirliği yapan ajan toplulukları.
- **Session (Oturum):** Mesaj geçmişini ve bağlamı koruyan konuşma dizisi.
- **Memory (Hafıza):** Hibrit hatırlama — dokümanlar, günlük (journal), yansıtma (reflection) notları.
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
| [05-ILERLEME.md](05-ILERLEME.md) | Yapılanlar / sıradaki adımlar takibi (**canlı durum** — yalnız 2026-06-22+ kayıtları) |
| [05-ARSIV.md](05-ARSIV.md) | İlerleme arşivi (2026-06-19 ve öncesi tamamlanmış kayıtlar) |
| [06-WORKSPACES.md](06-WORKSPACES.md) | Workspace izolasyonu tasarımı (fiziksel ayrım) |
| [07-CHAT-UX.md](07-CHAT-UX.md) | Zengin sohbet arayüzü + SSE adım-adım akış |
| [08-DEPOLAMA.md](08-DEPOLAMA.md) | Dosya-tabanlı depolama tasarımı (JSON/JSONL, DB yok) |
| [09-CLAUDE-AGENT-SDK.md](09-CLAUDE-AGENT-SDK.md) | Karar kaydı (ADR): Claude Agent SDK paritesi + claude-cli'yi köprü olarak benimseme |
| [10-KAVRAMSAL-TASARIM-NOTLARI.md](10-KAVRAMSAL-TASARIM-NOTLARI.md) | Kavramsal tasarım notları kataloğu (ClaudeCode mimarisi → SwarmGo, taslak/yol haritası) |
| [11-INTERACTION-MCP.md](11-INTERACTION-MCP.md) | Interaction MCP: CLI ajanlara insan-etkileşimli araçlar (ask_user/todo_write/onay) |
| [12-LOGLAMA.md](12-LOGLAMA.md) | Loglama sistemi: slog ring buffer, /api/logs, access + iş logları, dış erişim |
| [15-FLOW-CANVAS.md](15-FLOW-CANVAS.md) | Görsel Flow Builder (React Flow canvas) |
| [16-PROFILLEME.md](16-PROFILLEME.md) | Profilleme (pprof) rehberi |
| [17-TOKEN-OPTIMIZASYON.md](17-TOKEN-OPTIMIZASYON.md) | Araç çıktısı token optimizasyonu |
| [18-HOOKS.md](18-HOOKS.md) | Hooks (PreToolUse / PostToolUse) — Faz P4 |
| [19-LAZY-TOOL-LOADING.md](19-LAZY-TOOL-LOADING.md) | Lazy tool loading (tasarım + uygulama) |
| [20-SCHEDULE-WAKE.md](20-SCHEDULE-WAKE.md) | `schedule_wake`: ajanın kendi sohbetine geri dönmesi |
| [21-MARKET.md](21-MARKET.md) | Uygulama içi market sistemi (marketplace) |
| [22-SPAWN-SESSION.md](22-SPAWN-SESSION.md) | Spawn session (fire-and-forget paralel işçi) |
| [23-ILISKI-GRAFIGI.md](23-ILISKI-GRAFIGI.md) | İlişki grafiği: workspace ağı + hafıza bilgi grafiği (vis-network) |
| [24-SELF-MANAGEMENT.md](24-SELF-MANAGEMENT.md) | Self-management + ayarlar alt sistemi |
| [25-SUBAGENT-ISOLATION.md](25-SUBAGENT-ISOLATION.md) | Generic ajan yürütme çekirdeği + alt-ajan (subagent) izolasyonu |
| [26-CALISMA-DIZINI.md](26-CALISMA-DIZINI.md) | Çalışma dizini (working directory) — oturum-başına cwd |
| [27-CROSS-SESSION-SEARCH.md](27-CROSS-SESSION-SEARCH.md) | Oturumlar-arası tam-metin arama (`conversation_search`) |
| [28-PEER-MESAJLASMA-PLANI.md](28-PEER-MESAJLASMA-PLANI.md) | Ajanlar-arası peer mesajlaşma (send_message / mailbox) |
| [29-BILDIRIM-SINYALLERI.md](29-BILDIRIM-SINYALLERI.md) | Generic bildirim sinyalleri (nav + workspace) |
| [30-COKLU-PENCERE.md](30-COKLU-PENCERE.md) | Masaüstünde çoklu pencere (N süreç / N pencere) |
| [31-MEMGPT-CORE-MEMORY.md](31-MEMGPT-CORE-MEMORY.md) | MemGPT/Letta tarzı self-editing çekirdek bellek |
| [32-NATIVE-PENCERE.md](32-NATIVE-PENCERE.md) | Native masaüstü penceresi (WebView2, CGO'suz) |
| [33-DIS-AJAN-OTOMASYONU.md](33-DIS-AJAN-OTOMASYONU.md) | SwarmGo'yu dışarıdan (API/UI) sürme dostluğu |
| [34-YEDEKLEME.md](34-YEDEKLEME.md) | Workspace periyodik zip yedekleme + geri yükleme |
| [35-CONTEXT-RESET-HANDOFF.md](35-CONTEXT-RESET-HANDOFF.md) | Otonom turda context-reset / handoff |
| [36-KALICI-ILERLEME.md](36-KALICI-ILERLEME.md) | Kalıcı todo/PROGRESS dosyası (oturumlar-arası devralma) |
| [37-INGEST-MIMARISI.md](37-INGEST-MIMARISI.md) | Generic import (ingest) mimarisi — repo/plugin/local'den skill+agent+command+MCP keşfi |
| [38-SESSION-DEBUG.md](38-SESSION-DEBUG.md) | Oturum debug günlüğü (`debug.jsonl`) — paralel gözlemlenebilirlik + anomali tespiti |
| [39-DIZIN-SITE-REGISTRY.md](39-DIZIN-SITE-REGISTRY.md) | Dizin-sitesi köprüsü (search connector) — skill dizin sitelerinden markete arama/önizleme |
| [40-PLAN-MODE.md](40-PLAN-MODE.md) | Plan modu — claude-cli `ExitPlanMode` köprüsü + plan onay kartı |
| [41-ARAC-BOSLUKLARI-YAPILACAKLAR.md](41-ARAC-BOSLUKLARI-YAPILACAKLAR.md) | Araç boşlukları backlog'u (the external agent project↔SwarmGo karşılaştırması) |
| [42-REFAKTOR-MODULERLIK.md](42-REFAKTOR-MODULERLIK.md) | Refaktör/modülerlik — generic db/api/tools helper'ları + God-dosya bölmeleri |
| [43-REWIND.md](43-REWIND.md) | `/rewind` — sohbet checkpoint geri sarma (yalnız-sohbet MVP) |
| [44-CODE-EXECUTION-MCP.md](44-CODE-EXECUTION-MCP.md) | Code Execution with MCP — occupancy'yi kökten düşürme fizibilite + faz planı |
| [45-COKLU-SECIM.md](45-COKLU-SECIM.md) | Çoklu seçim (Ctrl/Cmd+Click) + toplu eylemler (frontend-only; eski 40 numarasından taşındı) |
| [46-ETIKET-OTOMASYON.md](46-ETIKET-OTOMASYON.md) | Etiketler (session/flow/schedule tags) + etiket-tetikleyicili döngü otomasyonları |
| [47-KOORDINATOR-COKLU-AJAN.md](47-KOORDINATOR-COKLU-AJAN.md) | Koordinatör & çoklu-ajan koordinasyonu (M2 koordinatör/worker: async spawn_worker + task-notification) |
| [analiz-craftagent-arac-eslestirme.md](analiz-craftagent-arac-eslestirme.md) | the external agent project↔SwarmGo araç eşleştirme analizi |
| **arsiv/** | Tarihsel inceleme dokümanları (referans/appendix) |
| [arsiv/13-CRAFT-AGENTS-INCELEME.md](arsiv/13-CRAFT-AGENTS-INCELEME.md) | external-agent-oss release incelemesi → SwarmGo çıkarımları |
| [arsiv/14-PROVIDER-MIMARISI-INCELEME.md](arsiv/14-PROVIDER-MIMARISI-INCELEME.md) | Çoklu-provider mimarisi incelemesi (gelecek plan) |

> **Numara notu:** 13–14 tarihsel inceleme dokümanları `arsiv/` altına taşındı (ana dizinde
> 13–14 boş). 17/18/26 eski numara çakışmaları giderildi → native pencere **32**, çoklu
> pencere **30**, MemGPT çekirdek bellek **31**. 40 çakışması giderildi (2026-07-02) →
> plan modu **40**, çoklu seçim **45**.

## Kurulu Ortam (2026-06-15 itibarıyla)

- ✅ Go 1.26.4
- ✅ Node.js v24 + npm 11
- ✅ WebView2 runtime (native masaüstü penceresi; Wails planı iptal — `32-NATIVE-PENCERE.md`)

## Proje Durumu (2026-06-23)

✅ **Faz 0–8 + kapsamlı backlog tamamlandı** ve Chrome'da canlı test edildi: İskelet · DB/Config · Provider+Chat (5 kind: anthropic/claude-cli/minimax/minimax-anthropic/openrouter) · React Web UI · Agent Runtime · Workspace İzolasyonu · Tasks+Schedules · Memory · Sağlamlaştırma (compaction + bütçe guardrail) · Tool-use+MCP · Orchestration (akışlar) · Lazy tool yükleme · Prefix'li insan-okunabilir ID'ler (WS/AGT/SES, `cmd/migrate-ids`) · İlişki grafiği · Self-management suite · Hooks · İzin modeli.
✅ **Sonradan eklenenler (06-22 → 06-23):** native masaüstü penceresi (WebView2, CGO'suz) + çoklu pencere · oturum-başına çalışma dizini (cwd) · MemGPT/Letta tarzı self-editing çekirdek bellek · MCP kalıcı bağlantı havuzu · oturumlar-arası tam-metin arama · ajanlar-arası peer mesajlaşma · generic bildirim sinyalleri.
➡️ **Sıradaki (2026-07-03):** Code Execution with MCP — büyük-çıktılı senaryoyla A/B tekrarı ([44](44-CODE-EXECUTION-MCP.md)) · koordinasyon kalanları — CLI köprüsü + ayar UI'si + M3 scratchpad ([47](47-KOORDINATOR-COKLU-AJAN.md)). Native masaüstü pencere WebView2 ile yapıldı — Wails gereksizleşti ([32](32-NATIVE-PENCERE.md)). (Connectors fazı 2026-06-16'da kapsamdan çıkarıldı.) Detay: [05-ILERLEME.md](05-ILERLEME.md) · arşiv: [05-ARSIV.md](05-ARSIV.md).
