# 04 — Teknoloji Seçimleri: İlk Plan (Arşiv)

> **Özet (2026-09-22):** `04-TEKNOLOJI-SECIMLERI.md`'nin 2026-06-15 öncesi ilk plan tabloları;
> 2026-09-22 doküman temizliğinde buraya taşındı. Satırların çoğu uygulanmadı (chi, sqlite, sqlc,
> golang-migrate, errgroup, resmi SDK'lar, mcp-go, Wails, shadcn/Zustand, WebSocket). Gerçekte
> kullanılan kararlar için `../04-TEKNOLOJI-SECIMLERI.md` "Uygulanan Durum" tablosu esastır.

## Backend (Go) — ⚠️ BAŞLANGIÇ PLANI (tarihsel)

> Aşağıdaki tablo **2026-06-15 öncesi ilk plandır, güncel değildir.** Birçok satır
> uygulanmadı (chi, sqlite, sqlc, golang-migrate, errgroup, resmi SDK'lar, mcp-go).
> **Gerçekte kullanılan kararlar için bir sonraki bölüme bakın →
> "Uygulanan Durum — Planlanan vs Gerçek".**

| İhtiyaç | Plan (uygulanmadı olabilir) | Gerekçe (o günkü) |
|---------|-------|---------|
| HTTP router | **go-chi/chi** | Hafif, idiomatic, stdlib uyumlu. (Alternatif: Echo, Fiber) |
| WebSocket | **coder/websocket** (eski nhooyr) | Modern, context-aware, bakımlı |
| Veritabanı | **modernc.org/sqlite** | Saf Go, CGO yok → çapraz derleme kolay |
| SQL kod üretimi | **sqlc** | Tip güvenli sorgular, derleme anında kontrol |
| Migration | **golang-migrate** veya basit runner | Sürümlü şema yönetimi |
| Zamanlama | **robfig/cron** | Cron ifadeleri, node-cron karşılığı |
| Eşzamanlılık | **golang.org/x/sync/errgroup** | Paralel görev + hata yayılımı |
| Anthropic SDK | **anthropics/anthropic-sdk-go** | Resmi SDK |
| OpenAI SDK | **sashabaranov/go-openai** | En yaygın, olgun |
| MCP istemci | **mark3labs/mcp-go** | Go için en olgun MCP kütüphanesi |
| Tarayıcı otomasyonu | **chromedp** veya **playwright-go** | Playwright karşılığı |
| Gözlemlenebilirlik | **go.opentelemetry.io/otel** | OpenTelemetry resmi |
| Şifreleme | stdlib **crypto/aes** + GCM | Harici bağımlılık yok |
| UUID | **google/uuid** | ID üretimi |
| Loglama | stdlib **log/slog** | Yapılandırılmış log, Go 1.21+ |

## Masaüstü Kabuk (ilk plan)

| Seçim | Gerekçe |
|-------|---------|
| **Wails v2** | Go backend + web frontend, native WebView, küçük boyut. Electron'un Go karşılığı. |

## Frontend (Web UI) (ilk plan)

| İhtiyaç | Seçim | Gerekçe |
|---------|-------|---------|
| Framework | **React + Vite** | En geniş ekosistem, Wails ile sorunsuz |
| Stil | **Tailwind CSS** | Hızlı, özelleştirilebilir |
| Bileşenler | **shadcn/ui** | Kopyala-sahip ol modeli, tam kontrol → kendi tasarım dili |
| State | **Zustand** veya **TanStack Query** | Hafif state + server cache |
| WebSocket | native `WebSocket` API | Canlı streaming |
