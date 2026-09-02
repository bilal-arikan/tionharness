# TionHarness

> Kendi makinende çalışan, çok-ajanlı (multi-agent) AI runtime'ı ve kontrol düzlemi.

Birden fazla otonom AI ajanını çalıştırmak; onlara görev dağıtmak, akış (flow) içinde
zincirlemek, zamanlanmış işler koşturmak ve hepsini tek bir arayüzden izlemek için
self-hosted bir çalışma ortamı. **Tek binary**, **veritabanı yok** (JSON/JSONL dosya
depolama) ve başlamak için **API anahtarı gerekmez** — `claude-cli` ya da `codex-cli`
gibi zaten giriş yapılmış bir abonelik CLI'ı yeterlidir.

- Site: <https://tionharness.com>
- Dokümantasyon: <https://tionharness.com/docs>
- Depo: <https://github.com/bilal-arikan/tionharness>
- Lisans: [Apache-2.0](LICENSE)

## Hızlı Başlangıç

Henüz etiketlenmiş bir sürüm (release) yok; kurulum yolu **kaynaktan derlemektir**.
Gereksinimler: Go 1.26+, Node 20+ ve giriş yapılmış bir sağlayıcı CLI'ı (ör. `claude`).

**Windows**

```powershell
git clone https://github.com/bilal-arikan/tionharness
cd tionharness

# UI'yı internal/web/dist'e derler, sonra tek exe'ye gömer
.\scripts\build.ps1

$env:TIONHARNESS_ADDR="127.0.0.1:8095"
.\tionharness.exe            # http://127.0.0.1:8095
```

**Linux / macOS**

```bash
git clone https://github.com/bilal-arikan/tionharness
cd tionharness

cd frontend && npm ci && npm run build && cd ..
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o tionharness ./cmd/tionharness

TIONHARNESS_ADDR=127.0.0.1:8095 ./tionharness
```

Geliştirme modunda (backend + Vite hot reload, tek komut):

```powershell
.\scripts\dev.ps1              # yerel ağa açık, tarayıcıyı açar
.\scripts\dev.ps1 -Loopback    # yalnız 127.0.0.1, güvenlik duvarı sormaz
```

Windows'ta tarayıcı yerine kendi penceresinde çalışan sürüm için
`.\scripts\build.ps1 -Desktop` → `tionharness-desktop.exe` (WebView2, CGO yok).

## Mimari

- **Backend:** Go, stdlib `net/http` üstünde HTTP API + SSE. CGO yok, çapraz derleme tek komut.
- **Frontend:** Vite + React + TypeScript + Tailwind. `internal/web/dist` içine derlenir ve
  `//go:embed` ile binary'ye gömülür — üretimde ayrı bir Vite sunucusu gerekmez, API ve UI
  aynı porttan servis edilir.
- **Depolama:** Veritabanı yok. Her şey dosya sisteminde JSON/JSONL olarak durur; bellek-içi
  store diske atomik yazar.
- **Veri dizini:** `~/.tionharness` (`TIONHARNESS_DATA_DIR` ile değiştirilir). İçinde
  uygulama-geneli `settings.json` ve `providers.json`, `logs/`, `skills/`, `market/`,
  `backups/`, sağlayıcı CLI login evleri (`provider-homes/`) ve `workspaces/<WS-id>/`
  altında workspace başına izole store bulunur.

Kod düzeni: `cmd/tionharness` (giriş noktası), `cmd/tionharness-desktop` (native pencere),
`internal/*` (alt sistemler), `frontend/` (uygulama arayüzü), `website/` (statik tanıtım +
dokümantasyon sitesi, Astro — Go modülünün dışındadır).

## Temel Kavramlar

- **Ajan (agent)** — kalıcı kimliği, prompt'u ve araç erişimi olan otonom AI varlığı. Ajan
  başına sağlayıcı örneği, model ve düşünme (thinking) seviyesi seçilir.
- **Oturum (session)** — bir ajanla yürütülen çok-turlu konuşma; mesaj geçmişi, araç adımları
  ve token kullanımı burada tutulur, uzun oturumlarda bağlam otomatik sıkıştırılır.
- **Workspace** — fiziksel izolasyon birimi: kendi store'u, runtime'ı ve scheduler'ı olan ayrı
  bir çalışma alanı. Tüm `/api/*` uçları `X-Workspace-Id` başlığına göre çalışır.
- **Görevler (kanban)** — "Görevler" ekranındaki pano; kart başına ajan, öncelik, etiket ve
  bağımlılık; facet filtreleri, gruplama ekseni ve workspace başına kayıtlı görünümler.
- **Akış (flow)** — çok-ajanlı graf: agent / branch / parallel / loop / subflow düğümleri,
  yeniden başlatmaya dayanıklı çalışma durumu ve görsel akış builder'ı.
- **Zamanlama (schedule)** — cron ifadeleriyle workspace başına periyodik çalıştırma.
- **Otomasyon (automation)** — olay tetikli kurallar (etiketli oturumun bitmesi, kart durumu
  değişmesi gibi) bir ajanı veya akışı çalıştırır. Zamanlamalarla birlikte "Otomasyon"
  ekranında yönetilir.
- **Rota (trajectory)** — bir koordinatör ağacının ilan edilen planı (reçetenin `phases:`
  bloğu ya da ajanın `trajectory` aracı) ile gözlenen gerçeklerin (worker'lar, akış koşuları,
  otomasyon ateşlemeleri, insan kapıları) tek grafta buluşması. Rota hiçbir şeyi çalıştırmaz;
  runtime yazar, "Rota" ekranı ve ajan okur. Faz kapıları (artifact / verdict / human), faz
  bitişi ve rota sonu tetikli otomasyonlar, koşu özeti, LLM'siz haftalık küratör ve yalnız
  öneri üreten reçete optimizer'ı bunun üstüne kuruludur.
- **Skill** — ajanın talep üzerine yüklediği yeniden kullanılabilir talimat paketi; bağlamı
  şişirmemek için tam metin yalnız gerektiğinde okunur.
- **Araçlar & MCP** — yerleşik araçlar (dosya oku/yaz/düzenle, glob/grep, shell [opsiyonel],
  `todo_write`, `ask_user`, artifact, self-management) ve harici MCP sunucuları (SDK'sız
  stdio JSON-RPC); araçlar tembel (lazy) yüklenir.

## Sağlayıcılar

11 sağlayıcı türü (provider kind) yerleşiktir; her türden birden çok **örnek** tanımlanabilir
ve ajanlar tek tek örneklere bağlanır:

`claude-cli` · `codex-cli` · `anthropic` · `minimax` · `minimax-anthropic` · `openrouter` ·
`zai` · `deepseek` · `deepseek-anthropic` · `anthropic-compat` · `openai-compat`

`claude-cli` ve `codex-cli` anahtarsızdır — makinede giriş yapılmış CLI'ı kullanır. Yeni CLI
örnekleri `<dataDir>/provider-homes/<instance-id>` altında kendi izole login evini alır.

## Arayüz

Panel, Sohbet, Ajanlar, Ağ, Harita, Rota, Görevler, Otomasyon, Akışlar, Artifactlar, Skills,
Araçlar & MCP, Market, Bütçe, Loglar ve İçgörü ekranları. Tema: açık/koyu/sistem +
6 renk paleti (Violet, Blue, Emerald, Rose, Amber, Nord) — her biri açık ve koyu varyantıyla,
hepsi canlı uygulanır. Arayüz dili, ajanın yanıt dilinden bağımsız olarak ayarlanır.

## Kapsam ve Sınırlar

Bunlar bilinçli tasarım kararlarıdır; kurmadan önce okuyun.

- **Barındırılan bir SaaS değil.** Hesap, kiracı (tenant) veya faturalandırma yoktur.
  Binary'yi siz çalıştırırsınız; veriniz çalıştığı makineden çıkmaz.
- **Kimlik doğrulama katmanı yok.** HTTP API'de auth yoktur ve CORS wildcard'dır. Loopback'e
  veya bir Tailscale adresine bağlayın; **doğrudan internete açmayın.**
- **Sandbox değil.** Dosya ve shell araçları tasarım gereği tüm dosya sistemine erişir. Tek
  koruma izin modudur (`auto` / `ask` / `read-only`) — bilinçli seçin.
- **Model sağlayıcısı değil.** Zaten erişiminiz olan modelleri orkestre eder; kendi CLI
  girişinizi veya API anahtarınızı getirmeniz gerekir.

## Geliştirme

```bash
export TIONHARNESS_ENABLE_SHELL=1  # yoksa shell aracı testleri skip'e düşer
go test ./... -count=1             # backend

cd frontend && npm test            # vitest
```

Frontend Prettier ile formatlanır (`frontend/.prettierrc.json`): `npm run format` /
`npm run format:check`. Pre-commit hook stage'lenmiş dosyaları otomatik formatlar; klon başına
bir kez `git config core.hooksPath .githooks`.

İlkeler: kod ve yorumlar İngilizce, dokümanlar Türkçe; CGO yok; her sorumluluk ayrı pakette.

## Dokümantasyon

Kullanıcı dokümantasyonu <https://tionharness.com/docs> adresindedir.

Derin tasarım ve plan dokümanları [`_Docs/`](_Docs/) klasöründedir. `_Docs` **iç
dokümantasyondur ve Türkçe yazılır**; sürümlenmiş bir kullanıcı kılavuzu değil, projenin
kendi mimari/karar defteridir. Başlangıç noktaları:
[Genel Bakış](_Docs/00-GENEL-BAKIS.md), [Mimari](_Docs/01-MIMARI.md),
[Depolama](_Docs/08-DEPOLAMA.md) ve canlı durum için
[İlerleme](_Docs/05-ILERLEME.md).

Depoda çalışan ajanlar için kurallar: [CLAUDE.md](CLAUDE.md).

## Lisans

TionHarness [Apache License 2.0](LICENSE) altında lisanslanmıştır. Gömülen ve dağıtılan
üçüncü-taraf bileşenlerin lisansları için bkz.
[THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md).
