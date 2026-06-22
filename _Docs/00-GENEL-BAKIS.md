# SwarmGo — Genel Bakış

> **SwarmGo**, [SwarmClaw](https://github.com/swarmclawai/swarmclaw) projesinin Go diliyle, kendi UI/UX tasarımımızla, sıfırdan yeniden yazımıdır.

## Amaç

Açık kaynaklı, kendi sunucunda barındırılan (self-hosted) bir **çoklu-ajan (multi-agent) AI çalışma ortamı (runtime)** ve **kontrol düzlemi (control plane)** inşa etmek. Birden fazla otonom AI ajanını yöneten, görev dağıtan, hafıza tutan ve zamanlanmış işler çalıştıran bir sistem.

## Neden Go?

| Kriter | Kazanım |
|--------|---------|
| **Eşzamanlılık (concurrency)** | Goroutine + channel modeli, çoklu-ajan orkestrasyonu için doğal |
| **Tek binary** | Bağımlılıksız, kolay dağıtım |
| **Düşük RAM / yüksek performans** | SwarmClaw'un Electron/Node yükü olmadan |
| **Çapraz derleme** | Tek komutla Windows / macOS / Linux |

## Orijinal (SwarmClaw) vs SwarmGo

| Bileşen | SwarmClaw | SwarmGo |
|---------|-----------|---------|
| Dil | TypeScript / Node.js 22 | Go 1.26+ |
| Masaüstü kabuk | Electron | Wails v2 (Faz 9, opsiyonel) |
| Web framework | Next.js | Bağımsız frontend + Go API; `dist/` binary'e `go:embed` ile gömülü |
| Depolama | better-sqlite3 | Dosya sistemi — JSON/JSONL, DB yok (bkz. `08-DEPOLAMA.md`) |
| Orkestrasyon | LangGraph | Kendi state-machine + goroutine/channel |
| Boyut | ~100-150 MB | ~10-20 MB hedef |

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
| [05-ILERLEME.md](05-ILERLEME.md) | Yapılanlar / sıradaki adımlar takibi (**canlı durum**) |
| [06-WORKSPACES.md](06-WORKSPACES.md) | Workspace izolasyonu tasarımı (fiziksel ayrım) |
| [07-CHAT-UX.md](07-CHAT-UX.md) | Zengin sohbet arayüzü + SSE adım-adım akış |
| [08-DEPOLAMA.md](08-DEPOLAMA.md) | Dosya-tabanlı depolama tasarımı (JSON/JSONL, DB yok) |
| [09-CLAUDE-AGENT-SDK.md](09-CLAUDE-AGENT-SDK.md) | Karar kaydı (ADR): Claude Agent SDK paritesi + claude-cli'yi köprü olarak benimseme |
| [10-KAVRAMSAL-TASARIM-NOTLARI.md](10-KAVRAMSAL-TASARIM-NOTLARI.md) | Kavramsal tasarım notları kataloğu (ClaudeCode mimarisi → SwarmGo, taslak/yol haritası) |
| [11-INTERACTION-MCP.md](11-INTERACTION-MCP.md) | Interaction MCP: CLI ajanlara insan-etkileşimli araçlar (ask_user/todo_write/onay) |
| [12-LOGLAMA.md](12-LOGLAMA.md) | Loglama sistemi: slog ring buffer, /api/logs, access + iş logları, dış erişim |
| [13-CRAFT-AGENTS-INCELEME.md](13-CRAFT-AGENTS-INCELEME.md) | external-agent-oss release incelemesi → SwarmGo çıkarımları |
| [14-SWARMCLAW-PROVIDER-INCELEME.md](14-SWARMCLAW-PROVIDER-INCELEME.md) | swarmclaw çoklu-provider mimarisi incelemesi (gelecek plan) |
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

## Kurulu Ortam (2026-06-15 itibarıyla)

- ✅ Go 1.26.4
- ✅ Node.js v24 + npm 11
- ⏳ Wails v2 (Faz 9'da kurulacak)

## Proje Durumu (2026-06-19)

✅ **Faz 0–8 + kapsamlı backlog tamamlandı** ve Chrome'da canlı test edildi: İskelet · DB/Config · Provider+Chat (5 kind: anthropic/claude-cli/minimax/minimax-anthropic/openrouter) · React Web UI · Agent Runtime · Workspace İzolasyonu · Tasks+Schedules · Memory · Sağlamlaştırma (compaction + bütçe guardrail) · Tool-use+MCP · Orchestration (akışlar) · Lazy tool yükleme · Prefix'li insan-okunabilir ID'ler (WS/AGT/SES, `cmd/migrate-ids`) · İlişki grafiği · Self-management suite · Hooks · İzin modeli.
➡️ **Sıradaki: Faz 9 — Wails paketleme.** (Connectors fazı 2026-06-16'da kapsamdan çıkarıldı.) Detay: [05-ILERLEME.md](05-ILERLEME.md).
