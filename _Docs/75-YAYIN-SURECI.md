# 75 — Yayın Süreci (release pipeline)

> **Özet (2026-10-03):** Kamusal yayını **GitHub Actions
> sahiplenir**: `.github/workflows/release.yml`. Derleme betiği
> `scripts/build-release.sh`; feed'i GitHub Pages'e taşıyan workflow
> `.github/workflows/pages.yml`.
>
> Kamusal hatta binary'ler GitHub Release asset'i, `latest.json` GitHub Pages'tedir.
> Gitea ayrı, secret ile yapılandırılmış kendi barındırılan feed'e rsync ile yayın
> yapar; yalnız doğrulama hattı değildir. `deploy/release-host/` depo içindeki
> **yerel önizleme** birimidir; uzak yayın hedefi onunla özdeş kabul edilmez.
>
> **Sürüm notları üretilir, elle yazılmaz (2026-09-22):** `cmd/changelog`,
> `<önceki sürüm etiketi>..<etiket>` aralığındaki conventional commit'leri tipe
> göre gruplayıp `_Docs/CHANGELOG.md` (arşiv, en yeni başta) ve
> `_Docs/release.json` (yalnız en son yayın, duyuru fan-out'unun girdisi)
> üretir; aynı çıktı GitHub Release gövdesi olur. Ayrıntı: "Changelog üretimi".
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
        ├── gate: bash scripts/ci.sh release
        │        kırmızıysa burada durur, hiçbir şey yayımlanmaz
        ├── bash scripts/build-release.sh --skip-install <version>
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

`bash scripts/build-release.sh <version>` beş hedefi `CGO_ENABLED=0` ile çapraz derler
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

## Changelog üretimi (conventional commits)

Sürüm notları elle yazılmaz: `cmd/changelog`, `<önceki sürüm etiketi>..<etiket>`
aralığındaki commit'leri **conventional commit** tipine göre gruplayıp üretir.
`release-please`/`changesets` gerekmedi — commit disiplini zaten var, ve bu araç
depoya tek bir bağımlılık eklemeden aynı işi yapıyor.

```bash
go run ./cmd/changelog render v1.2.3   # Print the Markdown section
go run ./cmd/changelog json   v1.2.3   # Print release.json
go run ./cmd/changelog write  v1.2.3   # Prepend CHANGELOG.md and write release.json
```

Kurallar:

- **Aralık** `<önceki sürüm etiketi>..<etiket>`. "Önceki", `v<semver>` biçimindeki
  **ve etiketin kendi soyunda bulunan** (`--merged`) en yakın etikettir. Depodaki
  `before-rename` gibi işaret etiketleri bir yayın sınırı **değildir** ve
  atlanır — aksi halde bir changelog aralığı sessizce kırpılırdı. İlk yayının
  öncesi yoktur ve tüm geçmişi kapsar; bu bir hata değildir.
- **Bölüm sırası** okuyucunun önce ne istediğine göre sabit: BREAKING CHANGES,
  Features, Bug Fixes, Performance, Refactoring, Documentation, Tests, Build, CI,
  Style, Chores, Reverts, Other.
- **Bozan değişiklikler** hem en üstteki BREAKING CHANGES bloğunda hem de kendi
  tür bölümünde görünür: üstteki blok "ne bozulacak", tür bölümü "ne değişti"
  sorusunu yanıtlar; kopyayı atmak, kategoriye göre okuyandan değişikliği gizlerdi.
  `!` işareti de `BREAKING CHANGE:` footer'ı da tanınır.
- **Tanınmayan tip veya conventional olmayan konu satırı düşürülmez**, `Other`
  altında listelenir. Depo bazı yerlerde bu kuraldan eskidir ve sessizce commit
  kaybetmektense görünür olması yeğlenir.
- **Merge commit'leri hariç** (`--no-merges`): kendi başına bir değişiklik
  taşımazlar, "Merge branch 'x'" konuları `Other`'ı gürültüyle doldururdu.
- Satırlar `**scope:** konu (kısa hash)` biçiminde. Hash olmadan bir changelog
  satırından commit'e geri dönülemez.
- `write` **idempotent**tir: aynı etiket için yeniden koşmak bölümü ikinci kez
  eklemez, yani yeniden denenen bir yayın job'ı changelog'u bozmaz.

`release.json` her zaman **yalnız en son** yayını anlatır; arşiv `CHANGELOG.md`'nin
kendisidir. Bu ayrım bilinçli: duyuru fan-out'u (`_Docs/86`) tek bir sürümü
duyurur, geçmişi değil.

### Hatta nereye bağlı

`release.yml` içinde iki yerde koşar:

1. **Publish GitHub Release öncesi** — `render` çıktısı `body_path` ile Release
   gövdesi olur.
2. **Publish feed to Pages içinde** — `write`, `latest.json` ile aynı commit'te
   `_Docs/CHANGELOG.md` + `_Docs/release.json` dosyalarını default branch'e
   yazar. Etiket checkout'unda yalnız `render` kullanılır; izlenen changelog
   dosyaları **default branch checkout'undan sonra** `write` ile üretilir.
   Böylece dal değiştirirken etiket aşamasında oluşturulmuş bir dosya kaybolmaz.

## Kamusal GitHub hattının izinleri

GitHub hattı ek kullanıcı secret'ı istemez. Release oluşturma ve feed commit'i için
otomatik `GITHUB_TOKEN`, Pages akışını açıkça tetiklemek için `actions: write`
kullanılır. Release workflow'u `contents: write`; Pages workflow'u `pages: write`
ve `id-token: write` izinlerini talep eder. `RELEASE_*` SSH/rsync ayarları yalnız
aşağıdaki ayrı Gitea hattına aittir.

## Depo ayarları (bir kez yapılır)

1. **Settings → Pages → Build and deployment → Source = GitHub Actions.**
   (`Deploy from a branch` seçiliyse `pages.yml` deploy adımı hata verir.)
2. **Settings → Pages → Custom domain = `tionharness.com`**, ardından
   **Enforce HTTPS** işaretlenir. Alan adı `website/public/CNAME` dosyasında da
   durur; her Pages deploy'u onu çıktının köküne kopyalar, böylece ayar deploy
   sırasında sıfırlanmaz.
3. **Alan adı sağlayıcının DNS panelinde** kayıtlar:

   | Host | Tip | Değer |
   |------|-----|-------|
   | `@` (apex) | A | `185.199.108.153` |
   | `@` (apex) | A | `185.199.109.153` |
   | `@` (apex) | A | `185.199.110.153` |
   | `@` (apex) | A | `185.199.111.153` |
   | `www` | CNAME | `<kullanici>.github.io` |

   Apex için A kayıtları (dört adet, GitHub Pages'in anycast havuzu), `www` için
   CNAME. Sağlayıcının varsayılan olarak eklediği çakışan apex A / `www` CNAME
   kayıtları **silinmelidir**, yoksa alan adı doğrulaması takılır.
   Eski `dl.` kaydına artık gerek yoktur.
4. **Settings → Actions → General → Workflow permissions**: `Read and write
   permissions` (feed commit'i ve release oluşturma bunu gerektirir).

## Gitea tarafı: kendi barındırılan sürüm sunucusuna yayın

`.gitea/workflows/release.yml` de `v*` etiketiyle tetiklenir ve şu sırayı izler:
kapı (`bash scripts/ci.sh release`: backend + frontend kurulum/testleri) →
`bash scripts/build-release.sh --skip-install "$VERSION"` →
`latest.json` doğrulaması → rsync ile yayın. **Yayın en son adımdır**: kapı veya
derleme kırılırsa runner'dan hiçbir dosya çıkmaz, sunucudaki feed eski sürümü
göstermeye devam eder.

Hedef tamamen secret ile parametriktir; dosyada hiçbir host/adres/anahtar gömülü
değildir:

| Secret | Zorunlu | Anlamı |
|--------|---------|--------|
| `RELEASE_HOST` | evet | rsync/ssh hedef host |
| `RELEASE_USER` | evet | ssh kullanıcısı |
| `RELEASE_PATH` | evet | sunucudaki kök dizin (feed + `v<sürüm>/` buraya yazılır) |
| `RELEASE_PORT` | hayır | ssh portu (varsayılan `22`) |
| `RELEASE_SSH_KEY` | evet | özel anahtar (PEM içeriği) |
| `RELEASE_SSH_KNOWN_HOSTS` | evet | sabitlenmiş host anahtarı (`StrictHostKeyChecking=yes`) |
| `RELEASE_FEED_BASE` | evet | `latest.json` içindeki URL'lerin kök adresi |
| `RELEASE_ARTIFACT_BASE` | hayır | arşivler ayrı bir originde duruyorsa |

Bu, GitHub Pages feed'inden **ayrı** bir feed'dir: yalnız `RELEASE_PATH` altına
yazar. `RELEASE_FEED_BASE`'i Pages alan adına yönlendirmeyin — o zaman iki hat aynı
`latest.json` üzerinde yarışır. Gerçek hedef runner secret'larıyla seçilir;
yerel Docker volume'ünün veya belirli bir VPS'in kullanıldığı kaynak dosyasından
varsayılmaz. Arşivler yayınlandıktan sonra kökteki feed yenilenir.

## Yerel önizleme (üretim DEĞİL)

`deploy/release-host/` altındaki Caddy birimi artık yalnızca bir **yerel önizleme**
aracıdır: feed + indirme yerleşimini uzak sunucuya ihtiyaç duymadan denemeye yarar.
Kamusal üretimde arşivler GitHub Release asset'i, feed Pages'tedir; Gitea'nın
ayrı uzak yayını kendi `RELEASE_*` hedefini kullanır.

```bash
# 1) Sürüm çıktısını üret (depo kökünde)
bash scripts/build-release.sh 1.2.3

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
FEED_BASE=http://localhost:8080 bash scripts/build-release.sh 1.2.3
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
  `{ state, current, latest, updateAvailable, notesUrl, releasedAt, checkedAt,
  downloadUrl, downloadFile }`.
  `state` üç sonucu ayırır: `ok` (feed okundu), `skipped` (ldflags'siz `dev`
  derlemesi — kontrol hiç yapılmaz), `unknown` (feed erişilemez/bozuk).
- **Damgalanmamış `dev` derlemesi hiç feed'e çıkmaz.** Geliştiriciye güncelleme
  uyarısı gösterilmez.
- **Hata yutulur (bilinçli):** erişilemeyen/bozuk feed loglanır, sonuç `unknown`
  olur; kullanıcıya hata gösterilmez ve açılış bloklanmaz. Frontend tarafında da
  istek hatası `console.error` ile loglanır, şerit yalnızca görünmez.
- **Artifact seçimi** (`internal/api/updateartifact.go`): `latest.json`'daki
  `artifacts` listesinden sunucunun kendi `runtime.GOOS`/`GOARCH` çiftine uyan
  giriş seçilir → `downloadUrl` doğrudan dosya, `downloadFile` dosya adı. Uyan
  giriş yoksa (veya girişin `url`'i boşsa) `downloadUrl` jenerik
  `<FeedURL()>/releases` sayfasıdır ve `downloadFile` boş kalır — başka
  platformun binary'si asla verilmez.
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
| `.gitea/workflows/release.yml` | Etiket → kapı → derleme → `latest.json` doğrulama → rsync (kendi barındırılan sürüm sunucusu, `RELEASE_*` secret'ları) |
| `scripts/build-release.sh` | Çapraz derleme, arşivleme, `SHA256SUMS`, `latest.json` (`FEED_BASE` + `ARTIFACT_BASE`) |
| `scripts/ci.sh` | Ortak `backend` / `frontend` / `release` doğrulama kapıları; CI örnekleri `bash scripts/ci.sh <mode>` kullanır |
| `cmd/changelog` | Conventional commit'lerden changelog üretir (`render` / `json` / `write`) |
| `internal/changelog` | Commit ayrıştırma, tür gruplama, aralık çözümü, Markdown + JSON render |
| `_Docs/CHANGELOG.md` | Yayın geçmişi; her yayında en yeni bölüm **başa** eklenir (üretilir, elle yazılmaz) |
| `_Docs/release.json` | **Yalnız en son** yayının makine-okunur hali — duyuru fan-out'unun girdisi |
| `scripts/install.sh` | Linux/macOS/Git Bash kurulum script'i (feed + sha256) |
| `scripts/install.ps1` | Windows PowerShell kurulum script'i (feed + sha256) |
| `deploy/release-host/sync-release.sh` | Yerel önizleme yayını (üretimde kullanılmaz) |
| `internal/api/updatecheck.go` | Uygulama içi sürüm kontrolü (feed okuma, önbellek) |
| `internal/api/semver.go` | Bağımlılıksız semver karşılaştırma (prerelease dahil) |
| `internal/api/updateartifact.go` | Platforma uyan artifact seçimi + `/releases` fallback |
| `frontend/src/app/UpdateBanner.tsx` | "Yeni sürüm mevcut" şeridi (yalnız link) |
| `deploy/release-host/docker-compose.yml` | Caddy statik indirme + tanıtım sitesi (yalnız yerel önizleme) |
| `_Docs/72-TANITIM-SITESI.md` | `www` tarafında sunulan tanıtım sitesi |
