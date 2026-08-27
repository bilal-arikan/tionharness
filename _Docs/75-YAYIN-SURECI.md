# 75 — Yayın Süreci (release pipeline)

> **Durum: GitHub tabanlı hat UYGULANDI (2026-08-27).** Yayını **GitHub Actions
> sahiplenir**: `.github/workflows/release.yml`. Derleme betiği
> `scripts/build-release.sh`; feed'i GitHub Pages'e taşıyan workflow
> `.github/workflows/pages.yml`.
>
> **Ortada VPS yoktur.** Sıfır altyapı: binary'ler GitHub Release asset'i,
> `latest.json` GitHub Pages. `deploy/release-host/` artık yalnızca **yerel
> önizleme** aracıdır (aşağıya bakın), üretim bileşeni değildir.
>
> Not: `_Docs/73-*` numarası lokalizasyona ait olduğu için bu doküman **75** numarasını
> aldı.

## Akış

```
git tag v1.2.3 → push (GitHub)
        │
        ▼
  .github/workflows/release.yml   (tek job, permissions: contents: write)
        │
        ├── gate: go vet + go test ./... -race + frontend npm ci/test
        │        kırmızıysa burada durur, hiçbir şey yayımlanmaz
        ├── scripts/build-release.sh <version>
        │        FEED_BASE=https://tionharness.com
        │        ARTIFACT_BASE=https://github.com/bilal-arikan/tionharness/releases/download/v1.2.3
        │        → dist/release/1.2.3/  (arşivler + SHA256SUMS + latest.json)
        ├── GitHub Release oluştur + arşivleri ve SHA256SUMS'ı asset olarak yükle
        │        (softprops/action-gh-release@v2)
        └── EN SON: latest.json → website/public/latest.json, default branch'e commit
                 → .github/workflows/pages.yml → https://tionharness.com/latest.json
```

Sıralama tesadüf değil: feed **en son** yayımlanır. `latest.json` bir sürümü ancak
asset'leri gerçekten indirilebilir hale geldikten sonra duyurur; ters sırada istemciler
henüz var olmayan dosyalar için 404 alırdı.

**Neden asset, neden Pages değil:** her sürüm ~125 MB. GitHub Pages'in site başına
~1 GB boyut ve ayda ~100 GB bant genişliği yumuşak sınırı vardır; birkaç sürümde
dolar. Release asset'lerinde böyle bir sınır yoktur. Buna karşılık birkaç yüz baytlık
`latest.json` Pages için idealdir ve `tionharness.com` kökünden sabit bir adreste
sunulur.

### Tetikleyiciler

| Tetikleyici | Sürüm nereden gelir |
|-------------|---------------------|
| `push` → `tags: ['v*']` | `github.ref_name` (baştaki `v` soyulur) |
| `workflow_dispatch` | `version` girdisi (elle yeniden çalıştırma) |

Her ikisinde de sürüm boşsa job hata verip durur; sessizce yanlış sürüm üretmez.

### Feed commit'i CI'ı yeniden tetiklemez

Feed adımı default branch'e `chore(release): publish latest.json for v<version>
[skip ci]` mesajıyla commit atar. `[skip ci]` GitHub'da **tüm** push tetikli
workflow'ları susturur — `pages.yml` dahil. Bu yüzden release workflow'u, commit
gerçekten atıldıysa `pages.yml`'yi `gh workflow run pages.yml` ile **açıkça**
tetikler (`permissions: actions: write` bunun içindir). `latest.json` değişmediyse
commit de dispatch de yapılmaz.

## İki temel adres: `FEED_BASE` ve `ARTIFACT_BASE`

Tek bir statik sunucu varken feed ile arşivler aynı host'taydı. GitHub'da **iki ayrı
host**tur, dolayısıyla `scripts/build-release.sh` iki değişken okur:

| Değişken | Anlamı | Varsayılan |
|----------|--------|------------|
| `FEED_BASE` | Feed'in kökü. Binary'ye `internal/api.FeedBaseURL` ldflag'i olarak gömülür; uygulama `<FEED_BASE>/latest.json` adresini yoklar | `https://tionharness.com` |
| `ARTIFACT_BASE` | `latest.json` içindeki `artifacts[].url` ön eki — arşivlerin gerçekte durduğu yer | `${FEED_BASE}/v<version>` |

`ARTIFACT_BASE` varsayılanı **eski davranışın birebir aynısıdır**: verilmediğinde url
yine `<FEED_BASE>/v<version>/<file>` olur, yani tek-host yerleşimi ve yerel Docker
akışı hiç değişmeden çalışır. GitHub workflow'u bunu Release indirme köküyle ezer:

```
ARTIFACT_BASE=https://github.com/bilal-arikan/tionharness/releases/download/v1.2.3
→ url = https://github.com/.../releases/download/v1.2.3/tionharness_1.2.3_linux_amd64.tar.gz
```

## Üretilen çıktı

`scripts/build-release.sh <version>` beş hedefi `CGO_ENABLED=0` ile çapraz derler
(linux amd64/arm64, windows amd64, darwin arm64/amd64) ve şunları yazar:

```
dist/release/<version>/
  tionharness_<version>_linux_amd64.tar.gz
  tionharness_<version>_linux_arm64.tar.gz
  tionharness_<version>_windows_amd64.zip
  tionharness_<version>_darwin_arm64.tar.gz
  tionharness_<version>_darwin_amd64.tar.gz
  SHA256SUMS
  latest.json
```

Arşivler ve `SHA256SUMS` GitHub Release asset'i olarak yüklenir; `latest.json` asset
**değildir**, `website/public/latest.json` üzerinden Pages'ten sunulur.

## `latest.json` şeması

```json
{
  "version": "1.2.3",
  "released_at": "2026-08-27T12:00:00Z",
  "notes_url": "https://tionharness.com/releases/v1.2.3",
  "artifacts": [
    {
      "os": "linux",
      "arch": "amd64",
      "file": "tionharness_1.2.3_linux_amd64.tar.gz",
      "url": "https://github.com/bilal-arikan/tionharness/releases/download/v1.2.3/tionharness_1.2.3_linux_amd64.tar.gz",
      "sha256": "…",
      "size": 18234567
    }
  ]
}
```

| Alan | Anlam |
|------|-------|
| `version` | Baştaki `v` olmadan semver (`1.2.3`, ön-sürüm eki olabilir) |
| `released_at` | Derleme anının UTC zaman damgası (RFC 3339) |
| `notes_url` | Sürüm notları sayfası |
| `artifacts[].os` | `linux` \| `windows` \| `darwin` |
| `artifacts[].arch` | `amd64` \| `arm64` |
| `artifacts[].file` | Arşiv dosya adı |
| `artifacts[].url` | `<ARTIFACT_BASE>/<file>` — tam indirme adresi (üretimde GitHub Release asset'i) |
| `artifacts[].sha256` | Arşivin SHA-256 özeti (`SHA256SUMS` ile aynı değer) |
| `artifacts[].size` | Arşiv boyutu (bayt, sayı) |

Üretimde tek bir kopya sunulur: `https://tionharness.com/latest.json`. Kaynağı depodaki
`website/public/latest.json` dosyasıdır ve her yayında üzerine yazılır. (Yerel Docker
önizlemesinde ayrıca `v<version>/latest.json` kopyası da oluşur.)

## Gereken secret'lar — yok

GitHub hattı **hiçbir secret istemez**. Release oluşturma ve feed commit'i için
otomatik `GITHUB_TOKEN` yeterlidir; workflow bunu `permissions: contents: write`
(+ `pages.yml` tarafında `pages: write`, `id-token: write`) ile talep eder. SSH
anahtarı, rsync, `RELEASE_*` secret'ları ve VPS **kaldırılmıştır**.

## Depo ayarları (bir kez yapılır)

1. **Settings → Pages → Build and deployment → Source = GitHub Actions.**
   (`Deploy from a branch` seçiliyse `pages.yml` deploy adımı hata verir.)
2. **Settings → Pages → Custom domain = `tionharness.com`**, ardından
   **Enforce HTTPS** işaretlenir. Alan adı `website/public/CNAME` dosyasında da
   durur; her Pages deploy'u onu çıktının köküne kopyalar, böylece ayar deploy
   sırasında sıfırlanmaz.
3. **DNS sağlayıcısı DNS** kayıtları:

   | Host | Tip | Değer |
   |------|-----|-------|
   | `@` (apex) | A | `185.199.108.153` |
   | `@` (apex) | A | `185.199.109.153` |
   | `@` (apex) | A | `185.199.110.153` |
   | `@` (apex) | A | `185.199.111.153` |
   | `www` | CNAME | `<user>.github.io` |

   Apex için A kayıtları (dört adet, GitHub Pages'in anycast havuzu), `www` için
   CNAME. DNS sağlayıcısı'in varsayılan olarak eklediği çakışan apex A / `www` CNAME
   kayıtları **silinmelidir**, yoksa alan adı doğrulaması takılır.
   Eski `dl.` kaydına artık gerek yoktur.
4. **Settings → Actions → General → Workflow permissions**: `Read and write
   permissions` (feed commit'i ve release oluşturma bunu gerektirir).

## Gitea tarafı: yalnız doğrulama

`.gitea/workflows/release.yml` **hiçbir şey yayımlamaz**. Aynı `v*` etiketinde iki
hat birden yayımlasaydı `latest.json` üzerinde yarışırlardı. Gitea kopyası artık
sadece kapıyı (vet + test) ve `build-release.sh`'in temiz bir Linux runner'da tam
artifact setini gerçekten ürettiğini doğrular; rsync adımları ve `RELEASE_*`
secret'ları kaldırılmıştır. Dosya bilinçli olarak silinmedi — Gitea hâlâ ikinci bir
derleme doğrulaması sağlıyor.

## Yerel önizleme (üretim DEĞİL)

`deploy/release-host/` altındaki Caddy birimi artık yalnızca bir **yerel önizleme**
aracıdır: feed + indirme yerleşimini VPS'e ihtiyaç duymadan denemeye yarar. Üretimde
karşılığı yoktur; üretimde arşivler GitHub Release asset'i, feed ise Pages'tedir.

```bash
# 1) Sürüm çıktısını üret (depo kökünde)
./scripts/build-release.sh 1.2.3

# 2) Statik sunucuyu ayağa kaldır
cd deploy/release-host
cp .env.example .env
docker compose up -d

# 3) Çıktıyı srv/dl/ içine yayımla (v<version>/ + kök latest.json)
./sync-release.sh 1.2.3

# 4) Feed'i doğrula
curl http://localhost:8080/latest.json
curl -I http://localhost:8080/v1.2.3/tionharness_1.2.3_linux_amd64.tar.gz
```

`sync-release.sh`, workflow'un rsync adımlarıyla **aynı yerleşimi** üretir:
`srv/dl/v<version>/` + promote edilen `srv/dl/latest.json`. Yani yerelde doğru görünen
bir düzen, uzak sunucuda da doğrudur.

Farklı bir feed adresiyle denemek için derlemeyi `FEED_BASE` ile koştur:

```bash
FEED_BASE=http://localhost:8080 ./scripts/build-release.sh 1.2.3
```

Bu durumda `latest.json` içindeki `url` alanları yerel sunucuyu gösterir ve indirme
akışı uçtan uca test edilebilir.

## Uygulama içi sürüm kontrolü (banner)

Uygulama, aynı feed'i okuyup "yeni sürüm var" bilgisini üstte kapatılabilir bir
şerit olarak gösterir. **İndirme ve otomatik güncelleme YOKTUR**: banner yalnızca
sürüm notlarına link verir, binary'yi değiştirme kararı kullanıcıdadır.

- Backend: `internal/api/updatecheck.go` (`<FeedURL()>/latest.json` isteği,
  8 sn timeout, gerçek `User-Agent`) + `internal/api/semver.go` (bağımlılık
  eklemeden semver karşılaştırma; `0.0.1-test` < `0.0.1`).
- Uç nokta: `GET /api/version/update` →
  `{ state, current, latest, updateAvailable, notesUrl, releasedAt, checkedAt }`.
  `state` üç sonucu ayırır: `ok` (feed okundu), `skipped` (ldflags'siz `dev`
  derlemesi — kontrol hiç yapılmaz), `unknown` (feed erişilemez/bozuk).
- **Damgalanmamış `dev` derlemesi hiç feed'e çıkmaz.** Geliştiriciye güncelleme
  uyarısı gösterilmez.
- **Hata yutulur (bilinçli):** erişilemeyen/bozuk feed loglanır, sonuç `unknown`
  olur; kullanıcıya hata gösterilmez ve açılış bloklanmaz.
- Önbellek: sonuç bellekte tutulur, en fazla 24 saatte bir gerçek istek atılır;
  eşzamanlı çağrılar tek isteğe düşer. Açılıştan ~15 sn sonra bir kez arka planda
  kontrol edilir (`Server.StartUpdateCheck`, `internal/app/app.go`).
- Frontend: `frontend/src/app/UpdateBanner.tsx` (görsel) +
  `frontend/src/app/updateCheck.ts` (saf mantık). Kapatma **sürüm başına**
  saklanır (`localStorage: tionharness.updateDismissedVersion`), böylece 0.2.0'ı
  kapatmak sonraki 0.3.0'ı gizlemez.

Yerel feed'e karşı uçtan uca denemek için (yukarıdaki Docker adımlarından sonra):

```bash
TIONHARNESS_FEED_URL=http://localhost:8080 go test ./internal/api/ \
  -run TestUpdateCheckAgainstLiveFeed -count=1 -v \
  -ldflags "-X github.com/bilal-arikan/tionharness/internal/api.BuildVersion=0.0.0"
```

Aynı komutu `-ldflags` olmadan koşturmak `dev` davranışını doğrular (kontrol
atlanır, feed'e istek gitmez).

## Kurulum script'leri (son kullanıcı)

Feed'i tüketen iki kurulum script'i vardır: `scripts/install.sh` (Linux, macOS ve
Windows'ta Git Bash) ve `scripts/install.ps1` (Windows PowerShell). İkisi de
`<feed>/latest.json` dosyasını indirir, makinenin GOOS/GOARCH değerine uyan
artifact'ı seçer, indirir ve **sha256'yı doğrular**.

- Feed adresi varsayılan `https://tionharness.com`; `install.sh` bunu
  `TIONHARNESS_FEED_URL` ile, `install.ps1` ise `-FeedUrl` parametresi veya aynı
  ortam değişkeni ile geçersiz kılar.
- Kurulum dizini: `install.sh` → `TIONHARNESS_INSTALL_DIR` ya da
  `$HOME/.local/bin`; `install.ps1` → `$env:TIONHARNESS_INSTALL_DIR` ya da
  `$env:LOCALAPPDATA\Programs\TionHarness`. Dizin PATH'te değilse script yalnızca
  nasıl ekleneceğini yazdırır, kullanıcının shell rc dosyasına dokunmaz.
- **Checksum doğrulaması atlanamaz.** Uyuşmazlıkta indirilen arşiv silinir ve
  script sıfırdan farklı çıkış kodu döndürür; `--skip-checksum` benzeri bir kaçış
  yolu bilinçli olarak yoktur.
- Platforma uyan artifact yoksa script, tespit edilen os/arch'ı adıyla belirtip
  hata verir; başka bir platformun derlemesine düşmez.
- Geçici çalışma dizini hata durumunda da temizlenir (`trap` / `finally`).

Yerel feed ile denemek için (yukarıdaki Docker adımlarından sonra):

```bash
TIONHARNESS_FEED_URL=http://localhost:8080 \
  TIONHARNESS_INSTALL_DIR=/tmp/thinstall bash scripts/install.sh
```

```powershell
$env:TIONHARNESS_INSTALL_DIR = "$env:TEMP\thinstall"
powershell -NoProfile -File scripts/install.ps1 -FeedUrl http://localhost:8080
```

## İlgili dosyalar

| Dosya | Rolü |
|-------|------|
| `.github/workflows/release.yml` | Etiket → kapı → derleme → Release asset'leri → feed commit'i (**yayını sahiplenir**) |
| `.github/workflows/pages.yml` | `website/` derleyip GitHub Pages'e deploy eder; `latest.json` ve `CNAME` de bu deploy'la gider |
| `website/public/latest.json` | Yayımlanan feed'in kaynağı → `https://tionharness.com/latest.json` |
| `website/public/CNAME` | Pages custom domain (`tionharness.com`) |
| `.gitea/workflows/release.yml` | Yalnız doğrulama: kapı + derleme, **yayın yok** |
| `scripts/build-release.sh` | Çapraz derleme, arşivleme, `SHA256SUMS`, `latest.json` (`FEED_BASE` + `ARTIFACT_BASE`) |
| `scripts/install.sh` | Linux/macOS/Git Bash kurulum script'i (feed + sha256) |
| `scripts/install.ps1` | Windows PowerShell kurulum script'i (feed + sha256) |
| `deploy/release-host/sync-release.sh` | Yerel önizleme yayını (üretimde kullanılmaz) |
| `internal/api/updatecheck.go` | Uygulama içi sürüm kontrolü (feed okuma, önbellek) |
| `internal/api/semver.go` | Bağımlılıksız semver karşılaştırma (prerelease dahil) |
| `frontend/src/app/UpdateBanner.tsx` | "Yeni sürüm mevcut" şeridi (yalnız link) |
| `deploy/release-host/docker-compose.yml` | Caddy statik indirme + tanıtım sitesi (yalnız yerel önizleme) |
| `_Docs/72-TANITIM-SITESI.md` | `www` tarafında sunulan tanıtım sitesi |
