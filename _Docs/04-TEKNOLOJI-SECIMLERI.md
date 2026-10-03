# TionHarness — Teknoloji Seçimleri

> **Özet (2026-09-22):** Backend/frontend/masaüstü için yapılan teknoloji seçimlerini, ilk plan ile gerçekte kullanılan kararları karşılaştıran bir tablo halinde sunar (ör. planlanan chi/sqlite/sqlc yerine gerçekte stdlib ServeMux + dosya-tabanlı depolama + elle yazılmış store kullanıldı). Durum: **uygulandı** — "Uygulanan Durum" tablosu güncel gerçeği yansıtır; ilk plan tablosu `arsiv/04-TEKNOLOJI-ILK-PLAN.md`'ye taşındı. En önemli ilke: bağımlılığı ancak gerçekten gerektiğinde ekle, CGO gerektiren kütüphanelerden kaçın (çapraz derleme için). Dayandığı dosyalar: `go.mod`, `internal/providers/kind_*.go`, `internal/web/embed.go`.

Her seçim, TypeScript dünyasındaki karşılığının Go ekosistemindeki en uygun eşleniğidir.

> İlk plan tablosu (chi/sqlite/sqlc/golang-migrate/resmi SDK'lar/mcp-go, Wails, shadcn/Zustand,
> WebSocket) tarihseldir → [arsiv/04-TEKNOLOJI-ILK-PLAN.md](arsiv/04-TEKNOLOJI-ILK-PLAN.md).
> Depolama 2026-06-15'te SQLite'tan dosya sistemine taşındı (`08-DEPOLAMA.md`).

## Uygulanan Durum (2026-09-28) — Planlanan vs Gerçek

Başlangıç planı ile güncel uygulamanın karşılaştırması:

| İhtiyaç | Plan | **Gerçekte** | Not |
|---------|------|--------------|-----|
| HTTP router | go-chi/chi | **stdlib `net/http` ServeMux** | Go 1.22+ method+path pattern → bağımlılık gerekmedi |
| Depolama | modernc.org/sqlite | **dosya sistemi (JSON/JSONL, DB yok)** | bellek-içi maps + atomik diske yazma; bkz. `08-DEPOLAMA.md` |
| Migration | golang-migrate | **SQL migration yok; dosya göçleri var** | eski oturum biçimi, artifact içerikleri ve sağlayıcı ayarları kodla dönüştürülür (`store_load.go`, `artifact_migrate.go`, `settings/provider_migrate.go`) |
| SQL üretimi | sqlc | **elle yazılmış store** | `internal/db/store_*.go` (artık SQL değil, dosya I/O) |
| Anthropic | resmi SDK | **ince HTTP istemci (SDK yok)** | Tam kontrol; ayrıca **claude-cli** (anahtarsız), **minimax-anthropic**, **openrouter**, **zai**, **deepseek**, **deepseek-anthropic** (güncel kind listesi: `internal/providers/kind_*.go`) |
| Web dağıtımı | ayrı statik sunum | **`go:embed all:dist`** (`internal/web/embed.go`) | `internal/web/dist/` derleme anında binary'ye gömülür; tek çalıştırılabilir dosya, CDN/statik sunucu gerekmez |
| Zamanlama | robfig/cron | ✅ **robfig/cron/v3** | Workspace başına scheduler |
| WebSocket/streaming | coder/websocket | **UI için SSE**, monitor için **coder/websocket** | Sohbet akışı SSE kullanır; `internal/tools/monitor_source_ws.go` harici WebSocket kaynaklarını izler (bkz. `85-MONITOR-SOZLESMESI.md`) |
| Frontend bileşen | shadcn/ui | **kendi Tailwind v4 bileşenleri** | Bileşen framework'ü yok; yalnız `lucide-react` (ikon) + `@fontsource-variable/inter`·`jetbrains-mono` (font) + `vis-network` v10.1.0 + `vis-data` (Harita ekranı) + `@xyflow/react` (flow canvas) eklendi |
| Tema | tek koyu tema | **token-tabanlı + 10 renk ailesi × 2 mod** | `var(--color-*)` semantic token seti; preset `<html>` inline style'a basılır (`frontend/src/shared/lib/themePresets.ts` — 20 preset; varsayılan `violet-dark`); paylaşılan UI primitifleri `shared/components/Button.tsx`; kategorik palet `shared/lib/palette.ts`; backend `settings.ThemePreset` ile kalıcı |
| Frontend state | Zustand/TanStack | **düz React `useState`** | Yeterli; ileride eklenebilir |
| UUID / log / şifreleme | google/uuid · slog · crypto/aes | ✅ hepsi kullanıldı | — |
| MCP istemci | mark3labs/mcp-go | **SDK'sız elle JSON-RPC 2.0** (stdio + Streamable HTTP) | Bağımlılıksız felsefe; `internal/mcp/manager.go` `DialStdio`/`DialHTTP`. Deprecated **SSE** taşıması bilinçli olarak desteklenmez (http'ye yönlendirir) |
| OTel (gözlemlenebilirlik) | otel | ⏳ ileride | Henüz eklenmedi |

> İlke: bağımlılığı ancak gerçekten gerektiğinde ekle. Depolama dosya sistemine taşındıktan sonra `modernc.org/sqlite` + ~8 dolaylı bağımlılık kaldırıldı. Doğrudan bağımlılıklar `go.mod` içinde tutulur: `google/uuid`, `robfig/cron/v3`, native pencere için `jchv/go-webview2`, WebSocket monitor için `coder/websocket` ve Windows süreç/dosya işlemleri için `golang.org/x/sys`.

## Masaüstü Kabuk

Wails planı iptal edildi (2026-06-22); native pencere CGO'suz WebView2 ile
`cmd/tionharness-desktop` altındadır (`jchv/go-webview2`, bkz. `32-NATIVE-PENCERE.md`).
Uygulama aynı zamanda saf web servisi (`localhost`) olarak çalışır.

## Karar: CGO Yok İlkesi

Çapraz derlemeyi kolaylaştırmak için **CGO gerektiren kütüphanelerden kaçınılır**:
- Depolama tamamen **stdlib** (`os`/`encoding/json`) üzerine; hiçbir veritabanı bağımlılığı yok (eski `modernc.org/sqlite` de saf Go idi, ama artık tümüyle kaldırıldı).
- Bu sayede `GOOS=darwin go build` gibi tek komutla diğer platformlara derlenir.

## Test Stratejisi

- Birim test: stdlib `testing`; yerel CGO kapalı ortamda `-race` koşmaz, Linux CI ise `-race` kullanır.
- Teslim kapısı: Git Bash içinde `scripts/test.sh full`; frontend/site derlemeleri ayrı doğrulanır.
- Provider arayüzü mock'lanabilir (interface tabanlı tasarım)
- Depolama testi: `internal/db/filestore_test.go` — geçici dizinde round-trip (create→reopen→reload).
- Provider HTTP: `internal/providers/transport_test.go` (`postJSON`) + `minimax_test.go` (`httptest` ile `Complete`).
- Runtime tunables: `internal/agent/tunables_test.go` (get/set + eşzamanlı erişim).
- Rota kaydı: `internal/api/server_test.go` (yinelenen/bozuk pattern panik regresyon koruması).
- MCP canlı testi: `internal/mcp/live_test.go`.

## Go hata yollarıyla uyumluluk

2026-09-29 doğrulamasında Go 1.27'nin bazı JSON hata yollarını
`Graph.nodes.0.branches.0` biçiminde ürettiği görüldü. Araç şeması hata açıklamaları,
önceki `Node.nodes.branches` biçimiyle birlikte tanınır; ortak çözümleme
`internal/tools/graph_error_path.go` ve regresyon testlerindedir. Bu uyumluluk
minimum Go sürümünü değiştirmez. Kişisel araç kurulumu, yerel yedekler ve bekleyen
yönetici işlemleri [tarihsel bakım raporunda](arsiv/HARICI-ARAC-GUNCELLEMELERI-2026-09-29.md)
korunur; ortak kurulum gereksinimi sayılmaz.
