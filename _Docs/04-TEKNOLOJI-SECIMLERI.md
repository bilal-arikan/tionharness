# SwarmGo — Teknoloji Seçimleri

Her seçim, SwarmClaw'daki TypeScript karşılığının Go ekosistemindeki en uygun eşleniğidir.

> ⚠️ **GÜNCEL (2026-06-15):** **Depolama SQLite'tan dosya sistemine taşındı.** Aşağıdaki
> tablolarda **modernc.org/sqlite / sqlc / golang-migrate / `:memory:` test** satırları
> artık geçerli **değil** — kalıcılık entity-başına JSON + oturum-başına JSONL üzerinde,
> bağımlılıksız. Güncel depolama tasarımı: **`_Docs/08-DEPOLAMA.md`**.

## Backend (Go)

| İhtiyaç | Seçim | Gerekçe |
|---------|-------|---------|
| HTTP router | **go-chi/chi** | Hafif, idiomatic, stdlib uyumlu. (Alternatif: Echo, Fiber) |
| WebSocket | **coder/websocket** (eski nhooyr) | Modern, context-aware, bakımlı |
| Veritabanı | **modernc.org/sqlite** | Saf Go, CGO yok → çapraz derleme kolay |
| SQL kod üretimi | **sqlc** | Tip güvenli sorgular, derleme anında kontrol |
| Migration | **golang-migrate** veya basit runner | Sürümlü şema yönetimi |
| Zamanlama | **robfig/cron** | Cron ifadeleri, SwarmClaw node-cron karşılığı |
| Eşzamanlılık | **golang.org/x/sync/errgroup** | Paralel görev + hata yayılımı |
| Anthropic SDK | **anthropics/anthropic-sdk-go** | Resmi SDK |
| OpenAI SDK | **sashabaranov/go-openai** | En yaygın, olgun |
| MCP istemci | **mark3labs/mcp-go** | Go için en olgun MCP kütüphanesi |
| Discord | **bwmarrin/discordgo** | Standart Go Discord kütüphanesi |
| Slack | **slack-go/slack** | Resmi olmayan ama standart |
| Tarayıcı otomasyonu | **chromedp** veya **playwright-go** | Playwright karşılığı |
| Gözlemlenebilirlik | **go.opentelemetry.io/otel** | OpenTelemetry resmi |
| Şifreleme | stdlib **crypto/aes** + GCM | Harici bağımlılık yok |
| UUID | **google/uuid** | ID üretimi |
| Loglama | stdlib **log/slog** | Yapılandırılmış log, Go 1.21+ |

## Uygulanan Durum (2026-06-15) — Planlanan vs Gerçek

Bu tablo başlangıç planıydı. Faz 0–8 sonunda gerçekte kullanılan kararlar:

| İhtiyaç | Plan | **Gerçekte** | Not |
|---------|------|--------------|-----|
| HTTP router | go-chi/chi | **stdlib `net/http` ServeMux** | Go 1.22+ method+path pattern → bağımlılık gerekmedi |
| Depolama | modernc.org/sqlite | **dosya sistemi (JSON/JSONL, DB yok)** | bellek-içi maps + atomik diske yazma; bkz. `08-DEPOLAMA.md` |
| Migration | golang-migrate | **yok (şema yok)** | dosya-store'da migration kavramı yok |
| SQL üretimi | sqlc | **elle yazılmış store** | `internal/db/store_*.go` (artık SQL değil, dosya I/O) |
| Anthropic | resmi SDK | **ince HTTP istemci (SDK yok)** | Tam kontrol; ayrıca **claude-cli** (anahtarsız) |
| Zamanlama | robfig/cron | ✅ **robfig/cron/v3** | Workspace başına scheduler |
| WebSocket/streaming | coder/websocket | ✅ **SSE** (`POST /api/chat/stream`); WebSocket yok | SSE adım-adım akış kuruldu (bkz. `07-CHAT-UX.md`); kalıcı WebSocket hub'ı gerekmedi |
| Frontend bileşen | shadcn/ui | **kendi Tailwind v4 bileşenleri** | Koyu tema, sıfır UI bağımlılığı |
| Frontend state | Zustand/TanStack | **düz React `useState`** | Yeterli; ileride eklenebilir |
| Recall/embedding | embedding tabanlı | **saf Go lexical cosine** | Anahtarsız/çevrimdışı; embedding ileride |
| UUID / log / şifreleme | google/uuid · slog · crypto/aes | ✅ hepsi kullanıldı | — |
| MCP istemci | mark3labs/mcp-go | **SDK'sız elle JSON-RPC 2.0** (stdio) | Bağımlılıksız felsefe; SSE/HTTP henüz yok (Faz 8 ✅) |
| Connectors · OTel | discordgo · otel | ⏳ ilgili fazlarda (9/sonra) | Henüz eklenmedi |

> İlke: bağımlılığı ancak gerçekten gerektiğinde ekle. Depolama dosya sistemine taşındıktan sonra `go.mod`'da yalnızca `google/uuid` ve `robfig/cron/v3` kaldı (`modernc.org/sqlite` + ~8 dolaylı bağımlılık kaldırıldı).

## Masaüstü Kabuk

| Seçim | Gerekçe |
|-------|---------|
| **Wails v2** | Go backend + web frontend, native WebView, küçük boyut. Electron'un Go karşılığı. |

> Not: Wails opsiyoneldir. Uygulama önce saf web servisi (`localhost`) olarak çalışır; Wails sadece native pencere sarmalayıcısı olarak Faz 10'da eklenir.

## Frontend (Web UI)

| İhtiyaç | Seçim | Gerekçe |
|---------|-------|---------|
| Framework | **React + Vite** | En geniş ekosistem, Wails ile sorunsuz |
| Stil | **Tailwind CSS** | Hızlı, özelleştirilebilir |
| Bileşenler | **shadcn/ui** | Kopyala-sahip ol modeli, tam kontrol → kendi tasarım dili |
| State | **Zustand** veya **TanStack Query** | Hafif state + server cache |
| WebSocket | native `WebSocket` API | Canlı streaming |

> Alternatif: Daha küçük bundle istenirse **Svelte + SvelteKit**. React, ekosistem genişliği için varsayılan.

## Karar: CGO Yok İlkesi

Çapraz derlemeyi kolaylaştırmak için **CGO gerektiren kütüphanelerden kaçınılır**:
- Depolama tamamen **stdlib** (`os`/`encoding/json`) üzerine; hiçbir veritabanı bağımlılığı yok (eski `modernc.org/sqlite` de saf Go idi, ama artık tümüyle kaldırıldı).
- Bu sayede `GOOS=darwin go build` gibi tek komutla diğer platformlara derlenir.

## Test Stratejisi

- Birim test: stdlib `testing` (CGO yok → `-race` kullanılmaz)
- Provider/Connector arayüzleri mock'lanabilir (interface tabanlı tasarım)
- Depolama testi: `internal/db/filestore_test.go` — geçici dizinde round-trip (create→reopen→reload).
- Provider HTTP: `internal/providers/transport_test.go` (`postJSON`) + `minimax_test.go` (`httptest` ile `Complete`).
- Runtime tunables: `internal/agent/tunables_test.go` (get/set + eşzamanlı erişim).
- Rota kaydı: `internal/api/server_test.go` (yinelenen/bozuk pattern panik regresyon koruması).
- MCP canlı testi: `internal/mcp/live_test.go`.
