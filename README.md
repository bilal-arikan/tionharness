# TionHarness

> **Go** ile, kendi UI/UX tasarımıyla yazılmış çok-ajanlı AI runtime'ı.

Açık kaynaklı, kendi sunucunda barındırılan **çoklu-ajan (multi-agent) AI çalışma ortamı** ve kontrol düzlemi. Tek binary, **dosya-tabanlı depolama** (JSON/JSONL, veritabanı yok — bkz. `_Docs/08-DEPOLAMA.md`), `claude-cli` ile **anahtarsız** çalışabilir.

## Özellikler

- 🧠 **Ajanlar** — soul/identity ile kişiselleştirilebilir; ajan başına sağlayıcı + model seçilebilir (`claude-cli`, `anthropic`, `minimax`), thinking seviyesi (off/low/medium/high)
- 💬 **Sohbet** — çok-turlu, otomatik **bağlam sıkıştırma** (compaction) ile uzun oturumlarda da ucuz
- ⚙️ **Otonom runtime** — zamanlama (cron) ve tek seferlik self-wake ile ajanlar kendi kendine ilerler
- 🗂 **Görevler** — kanban panosu, "şimdi çalıştır", run geçmişi; **görünüm katmanı**: facet filtreleri (öncelik/etiket/ajan/tarih/bağımlılık), gruplama ekseni (durum · ajan · öncelik · etiket · tarih — sürükleme eksenin alanını yazar) ve workspace başına kayıtlı görünümler — bkz. [Board Görünümleri](_Docs/67-BOARD-GORUNUMLERI.md)
- ⏰ **Zamanlamalar** — `robfig/cron` ile workspace başına scheduler
- ⛁ **Hafıza** — belge + günlük + yansıma (reflection/dream cycle), sözcüksel recall (anahtarsız/çevrimdışı)
- 🛡 **Bütçe guardrail** — otonom çağrılar için ajan başına günlük çağrı/token limiti
- 🧩 **Workspace izolasyonu** — her workspace ayrı DB + ayrı runtime + ayrı scheduler (fiziksel ayrım)
- 🔌 **Araçlar + MCP** — yerleşik araçlar (saat/http/recall, dosya oku-yaz-düzenle/glob/grep, shell [opsiyonel], todo_write, ask_user, artifact) + harici MCP sunucuları (SDK'sız stdio JSON-RPC); native tool-use döngüsü **ve** anahtarsız claude-cli MCP delegasyonu
- 🔀 **Orchestration** — çok-ajanlı akışlar (agent / branch / parallel node grafiği), şablon sistemi, restart-safe run state, görsel akış builder
- 🏷 **Otomatik başlık** — sohbet ilk turunda ve görev oluşturmada başlık prompt'tan otomatik üretilir, ⟳ ile yeniden üretilebilir
- ⚙️ **Ayarlar ekranı** — uygulama-geneli `settings.json` (Anthropic anahtarı AES-GCM şifreli); sağlayıcı/model, bağlam limitleri, otonomi duraklat, başlık modeli, açık/koyu/sistem tema + accent + **8 hazır tema paleti** (Gece Moru, Arduvaz, Zümrüt, Gül, Kehribar, Nord, Gün Işığı, Solarized) — hepsi **canlı** uygulanır
- 💎 **Zengin sohbet arayüzü** — markdown çıktı (GFM + syntax highlight), tool kullanım kartları, düşünme adımları, dosya satır değişimi (diff), tıklanabilir dosya yolları, inline görsel (External Agent benzeri render) — bkz. [Sohbet UX](_Docs/07-CHAT-UX.md)
- 📜 **Loglar** — uygulama + tüm workspace logları tek ekranda (canlı akış, seviye filtresi, arama)

## Hızlı Başlangıç (geliştirme)

```powershell
# Terminal 1 — backend (127.0.0.1:8090 — :8080 unity-mcp ile çakışır)
# Loopback adresi Windows Güvenlik Duvarı'nın her derlemede "izin ver" sormasını önler.
$env:TIONHARNESS_ADDR="127.0.0.1:8090"; go run ./cmd/tionharness

# Terminal 2 — frontend
cd frontend; npm install; npm run dev   # http://localhost:5173 (vite proxy → :8090)
```

Sağlık kontrolü: `curl http://localhost:8090/health`

**Tek komutla:** `.\scripts\dev.ps1` backend + frontend'i birlikte başlatır ve tarayıcıyı açar.
Varsayılan olarak **yerel ağa açar** (`0.0.0.0`) → aynı ağdaki telefon/laptop `http://<makine-LAN-IP>:5173`
adresinden erişir. Yalnız bu makineyle sınırlamak için `.\scripts\dev.ps1 -Loopback` (Windows Güvenlik
Duvarı sormaz). ⚠ Backend'de auth yok + CORS wildcard — yalnız güvenilir ağda aç.

> Tüm `/api/*` uçları `X-Workspace-Id` header'ına göre çalışır; yoksa Varsayılan workspace kullanılır.

## Tek Binary (üretim)

Frontend, `//go:embed` ile binary'ye gömülür → tek `tionharness.exe` hem API'yi hem UI'yı
aynı porttan sunar (ayrı Vite sunucusu gerekmez).

```powershell
# Hepsi bir arada: UI build (vite → internal/web/dist) + UI gömülü go build
.\scripts\build.ps1            # → tionharness.exe (~12 MB)

# Çalıştır
$env:TIONHARNESS_ADDR="127.0.0.1:8095"; .\tionharness.exe   # → http://127.0.0.1:8095 (UI + API)
```

Manuel (script'siz):

```powershell
cd frontend; npm run build; cd ..          # internal/web/dist'e üretir
go build -trimpath -ldflags "-s -w" -o tionharness.exe ./cmd/tionharness
```

> Frontend build edilmemişse (`internal/web/dist` yalnız placeholder içerir) binary yine
> derlenir; UI sunulmaz, log "frontend not bundled" der ve dev (Vite proxy) akışı kullanılır.
> Rota önceliği: `/api/*`, `/health`, `/mcp/*` her zaman önce; `/` ve bilinmeyen yollar SPA
> kabuğuna (index.html) düşer.

## Native Masaüstü Uygulaması (Windows)

Tarayıcı yerine **kendi penceresinde** açılan sürüm. WebView2 (Windows 11'de yerleşik)
kullanır; saf Go, CGO yok. Boş bir loopback portunda sunucuyu başlatır, UI'yı pencerede gösterir.

```powershell
.\scripts\build.ps1 -Desktop   # → tionharness-desktop.exe (~12 MB)
.\tionharness-desktop.exe          # çift tıkla → kendi penceresinde açılır, tarayıcı gerekmez
```

Detay ve tasarım: [_Docs/32-NATIVE-PENCERE.md](_Docs/32-NATIVE-PENCERE.md). WebView2 runtime
yoksa (nadir, eski Win10) otomatik olarak varsayılan tarayıcıya düşer. Başsız `tionharness.exe`
sürümü değişmeden durur; iki dağıtım yan yana kullanılabilir.

## Ortam Değişkenleri

| Değişken | Açıklama | Varsayılan |
|----------|----------|-----------|
| `TIONHARNESS_ADDR` | HTTP dinleme adresi (loopback varsayılan; ağa açmak için `0.0.0.0:8090`) | `127.0.0.1:8080` |
| `TIONHARNESS_DATA_DIR` | Kalıcı durum dizini | `~/.tionharness` |
| `TIONHARNESS_MAX_CONTEXT_TOKENS` | Bağlam sıkıştırma eşiği | `12000` |
| `TIONHARNESS_KEEP_RECENT_MSGS` | Sıkıştırmada korunan son mesaj sayısı | `8` |
| `CREDENTIAL_SECRET` | AES-GCM şifreleme anahtarı | otomatik üretim |
| `ANTHROPIC_API_KEY` | `anthropic` sağlayıcı için (claude-cli'da gerekmez) | — |

> Bu değerlerin çoğu artık **Ayarlar ekranından** da düzenlenebilir; uygulama-geneli `settings.json` (data dizininde, Anthropic anahtarı AES-GCM şifreli) içinde saklanır. İlk boot'ta env değerleri store'a migrate edilir, sonrasında store önceliklidir.

## Dokümantasyon

Tüm plan ve tasarım dokümanları [`_Docs/`](_Docs/) klasöründedir:

- [Genel Bakış](_Docs/00-GENEL-BAKIS.md)
- [Mimari](_Docs/01-MIMARI.md)
- [Veri Modeli](_Docs/02-VERI-MODELI.md)
- [Yol Haritası](_Docs/03-YOL-HARITASI.md)
- [Teknoloji Seçimleri](_Docs/04-TEKNOLOJI-SECIMLERI.md)
- [İlerleme Takibi](_Docs/05-ILERLEME.md) — **canlı durum burada**
- [Workspace İzolasyonu](_Docs/06-WORKSPACES.md)
- [Sohbet UX](_Docs/07-CHAT-UX.md)
- [Depolama](_Docs/08-DEPOLAMA.md)
- [Claude Agent SDK Paritesi (ADR)](_Docs/09-CLAUDE-AGENT-SDK.md)
- [Kavramsal Tasarım Notları](_Docs/10-KAVRAMSAL-TASARIM-NOTLARI.md)
- [Interaction MCP](_Docs/11-INTERACTION-MCP.md)
- [Loglama Sistemi](_Docs/12-LOGLAMA.md)
- [Tanıtım Sitesi](_Docs/72-TANITIM-SITESI.md)

## Proje Yapısı

```
TionHarness/
├── _Docs/                  # Plan ve tasarım dokümanları (Türkçe)
├── cmd/tionharness/            # Giriş noktası (Manager + API server + graceful shutdown)
├── internal/
│   ├── config/             # env + AES-GCM secret
│   ├── db/                 # Dosya-tabanlı store (JSON/JSONL, DB yok) — bellek-içi + atomik diske yazma
│   ├── providers/          # anthropic + claude-cli (anahtarsız) + minimax (OpenAI-uyumlu) + catalog + registry
│   ├── agent/              # runtime, worker, executor, scheduler, reflector, budget, titler, summarizer (/özet komutları), prompts (prompt görüntüleyici), toolloop, trace (aktivite izi), tunables, flow
│   ├── memory/             # lexical recall (cosine) + Store
│   ├── conversation/       # token-bütçeli compaction
│   ├── orchestration/      # akış graf motoru (agent/branch/parallel node)
│   ├── mcp/                # SDK'sız stdio JSON-RPC MCP istemcisi + manager
│   ├── tools/              # built-in + MCP birleşik araç kayıt defteri
│   ├── settings/           # uygulama-geneli ayarlar (şifreli settings.json deposu)
│   ├── workspace/          # workspace başına DB + runtime + scheduler
│   └── api/                # HTTP handler'ları (stdlib ServeMux)
├── frontend/               # Vite + React + TS + Tailwind v4 (uygulama arayüzü)
├── website/                # Statik tanıtım sitesi (Astro) — Go modülü dışı
└── go.mod
```

## Tanıtım Sitesi (`website/`)

Projeyi anlatan statik tek-sayfa site: **Astro 5 + Tailwind v4**, İngilizce, sıfır runtime
framework. Uygulamayla hiçbir bağı yoktur — Go modülünün dışındadır, `go:embed` ağacına
girmez, binary'yi büyütmez.

```powershell
cd website
npm install
npm run dev      # http://localhost:4321
npm run build    # -> website/dist/ (statik çıktı)
```

Public repo, release binary'leri, docs sitesi ve lisans henüz yok; bunların tamamı
`website/src/site.config.ts` içinde `null` olarak durur ve site bunları **ölü link yerine**
"Coming soon" olarak gösterir. Repo yayına alınınca yalnız o dosya doldurulur.

Ürün ekran görüntüleri repoda tutulmaz, çalışan bir örnekten üretilir:

```powershell
.\scripts\dev.ps1                    # önce uygulamayı başlat
.\scripts\shots.ps1 -InstallDeps     # Playwright ile website/public/shots/'a çeker
```

Detay: [Tanıtım Sitesi](_Docs/72-TANITIM-SITESI.md) · [website/README.md](website/README.md)

## Geliştirme İlkeleri

- Kod ve yorumlar **İngilizce**, dokümanlar **Türkçe**.
- CGO yok → kolay çapraz derleme (Windows / macOS / Linux).
- Modüler: her sorumluluk ayrı pakette / ayrı dosyada.

## Durum

✅ **Faz 0–8 tamamlandı** (İskelet · DB/Config · Provider+Chat · Web UI · Agent Runtime · Workspace İzolasyonu · Tasks+Schedules · Memory · Sağlamlaştırma · Tool-use+MCP · Orchestration) **+ ara özellikler** (otomatik başlık · uygulama-geneli Ayarlar ekranı · zengin sohbet arayüzü).
➡️ **Sıradaki: Faz 9 — Wails paketleme.** (Connectors fazı 2026-06-16'da kapsamdan çıkarıldı.) Detaylar için bkz. [İlerleme](_Docs/05-ILERLEME.md).

> Not: Faz 8 (Tool-use + MCP) kullanıcı talebiyle Faz 7'den (Orchestration) önce tamamlandı.
