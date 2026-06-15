# SwarmGo — Teknoloji Seçimleri

Her seçim, SwarmClaw'daki TypeScript karşılığının Go ekosistemindeki en uygun eşleniğidir.

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
- ✅ `modernc.org/sqlite` (saf Go) — ❌ `mattn/go-sqlite3` (CGO)
- Bu sayede `GOOS=darwin go build` gibi tek komutla diğer platformlara derlenir.

## Test Stratejisi

- Birim test: stdlib `testing` + `stretchr/testify`
- Provider/Connector arayüzleri mock'lanabilir (interface tabanlı tasarım)
- Integration test: gerçek SQLite (in-memory `:memory:`)
