# SwarmGo

> [SwarmClaw](https://github.com/swarmclawai/swarmclaw) projesinin **Go** ile, kendi UI/UX tasarımıyla yeniden yazımı.

Açık kaynaklı, kendi sunucunda barındırılan **çoklu-ajan (multi-agent) AI çalışma ortamı** ve kontrol düzlemi. Tek binary, **dosya-tabanlı depolama** (JSON/JSONL, veritabanı yok — bkz. `_Docs/08-DEPOLAMA.md`), `claude-cli` ile **anahtarsız** çalışabilir.

## Özellikler

- 🧠 **Ajanlar** — soul/identity ile kişiselleştirilebilir; sağlayıcı başına seçilebilir (`anthropic`, `claude-cli`)
- 💬 **Sohbet** — çok-turlu, otomatik **bağlam sıkıştırma** (compaction) ile uzun oturumlarda da ucuz
- ⚙️ **Otonom runtime** — ajan başına heartbeat, exponential backoff, 10 hatada otomatik devre dışı
- 🗂 **Görevler** — kanban panosu, "şimdi çalıştır", run geçmişi
- ⏰ **Zamanlamalar** — `robfig/cron` ile workspace başına scheduler
- ⛁ **Hafıza** — belge + günlük + yansıma (reflection/dream cycle), sözcüksel recall (anahtarsız/çevrimdışı)
- 🛡 **Bütçe guardrail** — otonom çağrılar için ajan başına günlük çağrı/token limiti
- 🧩 **Workspace izolasyonu** — her workspace ayrı DB + ayrı runtime + ayrı scheduler (fiziksel ayrım)
- 🔌 **Araçlar + MCP** — yerleşik araçlar (saat/http/recall) + harici MCP sunucuları (SDK'sız stdio JSON-RPC); native tool-use döngüsü **ve** anahtarsız claude-cli MCP delegasyonu
- 🔀 **Orchestration** — çok-ajanlı akışlar (agent / branch / parallel node grafiği), şablon sistemi, restart-safe run state, görsel akış builder
- 🏷 **Otomatik başlık** — sohbet ilk turunda ve görev oluşturmada başlık prompt'tan otomatik üretilir, ⟳ ile yeniden üretilebilir
- ⚙️ **Ayarlar ekranı** — uygulama-geneli `settings.json` (Anthropic anahtarı AES-GCM şifreli); sağlayıcı/model, bağlam limitleri, otonomi duraklat, başlık modeli, açık/koyu/sistem tema + accent — hepsi **canlı** uygulanır
- 💎 **Zengin sohbet arayüzü** — markdown çıktı (GFM + syntax highlight), tool kullanım kartları, düşünme adımları, dosya satır değişimi (diff), tıklanabilir dosya yolları, inline görsel (External Agent benzeri render) — bkz. [Sohbet UX](_Docs/07-CHAT-UX.md)
- 📜 **Loglar** — uygulama + tüm workspace logları tek ekranda (canlı akış, seviye filtresi, arama)

## Hızlı Başlangıç (geliştirme)

```powershell
# Terminal 1 — backend (:8090 — :8080 unity-mcp ile çakışır)
$env:SWARMGO_ADDR=":8090"; go run ./cmd/swarmgo

# Terminal 2 — frontend
cd frontend; npm install; npm run dev   # http://localhost:5173 (vite proxy → :8090)
```

Sağlık kontrolü: `curl http://localhost:8090/health`

> Tüm `/api/*` uçları `X-Workspace-Id` header'ına göre çalışır; yoksa Varsayılan workspace kullanılır.

## Ortam Değişkenleri

| Değişken | Açıklama | Varsayılan |
|----------|----------|-----------|
| `SWARMGO_ADDR` | HTTP dinleme adresi | `:8080` |
| `SWARMGO_DATA_DIR` | Kalıcı durum dizini | `~/.swarmgo` |
| `SWARMGO_MAX_CONTEXT_TOKENS` | Bağlam sıkıştırma eşiği | `12000` |
| `SWARMGO_KEEP_RECENT_MSGS` | Sıkıştırmada korunan son mesaj sayısı | `8` |
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

## Proje Yapısı

```
SwarmGo/
├── _Docs/                  # Plan ve tasarım dokümanları (Türkçe)
├── cmd/swarmgo/            # Giriş noktası (Manager + API server + graceful shutdown)
├── internal/
│   ├── config/             # env + AES-GCM secret
│   ├── db/                 # Dosya-tabanlı store (JSON/JSONL, DB yok) — bellek-içi + atomik diske yazma
│   ├── providers/          # anthropic + claude-cli (anahtarsız) + registry
│   ├── agent/              # runtime, worker, executor, scheduler, reflector, budget, titler, toolloop, trace (aktivite izi), tunables, flow
│   ├── memory/             # lexical recall (cosine) + Store
│   ├── conversation/       # token-bütçeli compaction
│   ├── orchestration/      # akış graf motoru (agent/branch/parallel node)
│   ├── mcp/                # SDK'sız stdio JSON-RPC MCP istemcisi + manager
│   ├── tools/              # built-in + MCP birleşik araç kayıt defteri
│   ├── settings/           # uygulama-geneli ayarlar (şifreli settings.json deposu)
│   ├── workspace/          # workspace başına DB + runtime + scheduler
│   └── api/                # HTTP handler'ları (stdlib ServeMux)
├── frontend/               # Vite + React + TS + Tailwind v4
└── go.mod
```

## Geliştirme İlkeleri

- Kod ve yorumlar **İngilizce**, dokümanlar **Türkçe**.
- CGO yok → kolay çapraz derleme (Windows / macOS / Linux).
- Modüler: her sorumluluk ayrı pakette / ayrı dosyada.

## Durum

✅ **Faz 0–8 tamamlandı** (İskelet · DB/Config · Provider+Chat · Web UI · Agent Runtime · Workspace İzolasyonu · Tasks+Schedules · Memory · Sağlamlaştırma · Tool-use+MCP · Orchestration) **+ ara özellikler** (otomatik başlık · uygulama-geneli Ayarlar ekranı · zengin sohbet arayüzü).
➡️ **Sıradaki: Faz 9 — Wails paketleme.** (Connectors fazı 2026-06-16'da kapsamdan çıkarıldı.) Detaylar için bkz. [İlerleme](_Docs/05-ILERLEME.md).

> Not: Faz 8 (Tool-use + MCP) kullanıcı talebiyle Faz 7'den (Orchestration) önce tamamlandı.
