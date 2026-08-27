# 75 — Yayın Süreci (release pipeline)

> **Durum: CI hattı UYGULANDI (2026-08-27).** Etiket atıldığında derleyip yayımlayan
> Gitea workflow'u: `.gitea/workflows/release.yml`. Derleme betiği
> `scripts/build-release.sh`, indirme sunucusu `deploy/release-host/`.
>
> Not: `_Docs/73-*` numarası lokalizasyona ait olduğu için bu doküman **75** numarasını
> aldı.

## Akış

```
git tag v1.2.3 → push
        │
        ▼
  .gitea/workflows/release.yml
        │
        ├── job: test         (ci.yml'nin aynısı — go vet + go test ./... -race)
        │        │  kırmızıysa burada durur, hiçbir şey yayımlanmaz
        │        ▼
        └── job: release      (needs: test)
                 ├── scripts/build-release.sh <version>
                 │      → dist/release/<version>/  (arşivler + SHA256SUMS + latest.json)
                 ├── publish  → rsync  <RELEASE_PATH>/v<version>/
                 └── promote  → rsync  <RELEASE_PATH>/latest.json   (EN SON)
```

Sıralama tesadüf değil: `latest.json` **en son** yüklenir. Feed bir sürümü ancak
dosyaları indirilebilir hale geldikten sonra duyurur; ters sırada istemciler henüz var
olmayan arşivleri indirmeye çalışırdı.

Kapı (gate) `needs: test` ile kurulur. `test` job'ı kırmızıysa `release` job'ı hiç
başlamaz — derleme de yayın da olmaz.

### Tetikleyiciler

| Tetikleyici | Sürüm nereden gelir |
|-------------|---------------------|
| `push` → `tags: ['v*']` | `gitea.ref_name` (baştaki `v` soyulur) |
| `workflow_dispatch` | `version` girdisi (elle yeniden çalıştırma) |

Her ikisinde de sürüm boşsa job hata verip durur; sessizce yanlış sürüm üretmez.

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

Betik `FEED_BASE` ortam değişkenini okur (varsayılan `https://dl.tionharness.com`) ve
bu değeri hem `latest.json` içindeki indirme adreslerine hem de binary'ye
(`internal/api.FeedBaseURL` ldflag) gömer. Workflow bu değeri `FEED_BASE` secret'ından
alır; secret yoksa varsayılana düşer.

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
      "url": "https://dl.tionharness.com/v1.2.3/tionharness_1.2.3_linux_amd64.tar.gz",
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
| `artifacts[].url` | `<FEED_BASE>/v<version>/<file>` — tam indirme adresi |
| `artifacts[].sha256` | Arşivin SHA-256 özeti (`SHA256SUMS` ile aynı değer) |
| `artifacts[].size` | Arşiv boyutu (bayt, sayı) |

Dosyanın iki kopyası sunulur: sürüme sabitlenmiş `v<version>/latest.json` ve istemcilerin
yokladığı kök `latest.json`. Kök kopya her yayında üzerine yazılır.

## Gereken secret'lar

| Secret | Zorunlu | Anlam |
|--------|---------|-------|
| `RELEASE_HOST` | hayır | Yayın hedefinin adresi. **Boşsa yayın adımı atlanır.** |
| `RELEASE_USER` | hayır | SSH kullanıcısı (varsayılan `hermes`) |
| `RELEASE_PATH` | hayır | Sunucudaki `dl` kökü (varsayılan `/srv/dl`) |
| `RELEASE_SSH_KEY` | `RELEASE_HOST` doluysa evet | rsync/ssh için özel anahtar |
| `FEED_BASE` | hayır | Feed kök adresi (varsayılan `https://dl.tionharness.com`) |

Secret adları ve SSH kurulumu `deploy.yml`'deki `DEPLOY_SSH_KEY` deseninin aynısıdır:
anahtar `~/.ssh/release_key` dosyasına yazılır, `chmod 600` verilir ve
`ssh -o StrictHostKeyChecking=no` ile kullanılır.

### VPS yokken davranış

Şu an ortada VPS yok; yığın yerel Docker'da çalışıyor. Bu yüzden `RELEASE_HOST`
**boş** kabul edilir ve workflow şöyle davranır:

- `test` ve derleme adımları normal çalışır — sürüm çıktısı gerçekten üretilir.
- `Prepare SSH`, `Ensure rsync` ve `Publish to download host` adımları erken çıkar ve
  log'a neden atlandığını yazar: `RELEASE_HOST is empty - publish SKIPPED.`
- Job **yeşil** biter. Hattı kırmaz, ama "yayımladım" gibi de davranmaz.

Secret'lar tanımlandığı anda aynı workflow, dosya değişikliği gerekmeden yayımlamaya
başlar.

### Bilinen eksik: workflow artifact yüklemesi

`ci.yml` ve `deploy.yml`'de hiçbir artifact yükleme action'ı (`actions/upload-artifact`
vb.) kullanılmıyor; bu Gitea kurulumunda çözüleceği doğrulanmamış bir action'ı
uydurmamak için adım **eklenmedi**. Çıktılar şimdilik yalnızca job'ın çalışma dizininde
(`dist/release/<version>/`) durur ve `Summary` adımı `SHA256SUMS` içeriğini log'a basar.
Runner'da artifact desteği doğrulandığında `release` job'ının sonuna tek bir yükleme
adımı eklenmesi yeterlidir.

## Yerel Docker ile test etme

Yayın sunucusu `deploy/release-host/` altındaki Caddy birimidir (`dl` ve `www` statik
siteleri). VPS'e dokunmadan tüm akışı yerelde denemek için:

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

- Feed adresi varsayılan `https://dl.tionharness.com`; `install.sh` bunu
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
| `.gitea/workflows/release.yml` | Etiket → test → derleme → yayın hattı |
| `scripts/build-release.sh` | Çapraz derleme, arşivleme, `SHA256SUMS`, `latest.json` |
| `scripts/install.sh` | Linux/macOS/Git Bash kurulum script'i (feed + sha256) |
| `scripts/install.ps1` | Windows PowerShell kurulum script'i (feed + sha256) |
| `deploy/release-host/sync-release.sh` | Yerel/manuel yayın (aynı yerleşim) |
| `internal/api/updatecheck.go` | Uygulama içi sürüm kontrolü (feed okuma, önbellek) |
| `internal/api/semver.go` | Bağımlılıksız semver karşılaştırma (prerelease dahil) |
| `frontend/src/app/UpdateBanner.tsx` | "Yeni sürüm mevcut" şeridi (yalnız link) |
| `deploy/release-host/docker-compose.yml` | Caddy statik indirme + tanıtım sitesi |
| `_Docs/72-TANITIM-SITESI.md` | `www` tarafında sunulan tanıtım sitesi |
