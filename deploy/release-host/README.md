# TionHarness sürüm sunucusu — YEREL ÖNİZLEME

Bu Docker Compose birimi feed ve indirme yerleşimini **yerelde** denemek içindir.
Kamusal yayın GitHub Release dosyaları ve GitHub Pages üzerindeki
`https://tionharness.com/latest.json` adresidir. Gitea'nın `RELEASE_*` secret'larıyla
yönettiği kendi barındırılan feed ayrı bir yayın hedefidir; buradaki yerel örnek
ayarlar onu yapılandırmaz. Ayrıntı:
[`_Docs/75-YAYIN-SURECI.md`](../../_Docs/75-YAYIN-SURECI.md).

Bu Docker Compose birimi, Caddy ile iki statik site sunar:

- `dl`: sürüm dosyaları ve istemcilerin yokladığı `latest.json`
- `www`: TionHarness tanıtım sitesinin statik çıktısı

## Yerelde çalıştırma

```bash
cd deploy/release-host
cp .env.example .env
docker compose up -d
```

İndirme sitesi `http://localhost:8080`, WWW sitesi `http://localhost:8081` adresindedir. WWW için host portu `8081`, container portu `81` ile eşlenir.
Geliştirme backend'inin `8090` portu bu önizleme sunucusundan ayrıdır.

## Sürüm yayımlama

Önce depo kökünde yerel indirme adreslerini taşıyan çıktıyı üretin:

```bash
FEED_BASE=http://localhost:8080 ./scripts/build-release.sh 1.2.3
```

Ardından bu dizinde çıktıyı yayımlayın:

```bash
./sync-release.sh 1.2.3
```

Kontrol:

```bash
curl http://localhost:8080/latest.json
```

`build-release.sh` bağımsız çalışırken frontend bağımlılıklarını kurar ve UI'yı
derler. CI aynı kurulumun tekrarlanmaması için `--skip-install` kullanır.

## www sitesini yayımlama

`{$WWW_SITE}` bloğu `/srv/www` dizinini sunar. Bu dizin boşken 8081 portu
`404` döner — tanıtım sitesinin çıktısını oraya kopyalamak gerekir. Kaynak,
depodaki `website/` (Astro) projesidir; `astro build` çıktıyı `website/dist`
altına yazar.

```bash
cd website
npm ci          # ilk kurulumda
npm run build   # -> website/dist
cd ../deploy/release-host
./sync-www.sh
```

`sync-www.sh` varsayılan olarak **yerel** modda çalışır: `website/dist`
içeriğini bu dizindeki `srv/www` altına kopyalar (`.gitkeep` korunur, eski
dosyalar silinir). Compose birimi bu dizini salt-okunur bağladığı için
Caddy'yi yeniden başlatmaya gerek yoktur.

Kontrol:

```bash
curl -I http://localhost:8081/        # 200 beklenir (önceden 404)
```

### Uzak (self-hosted) yayım

`RELEASE_HOST` tanımlıysa betik rsync + ssh ile uzak sunucuya yayım yapar.
Ortam değişkenleri `.gitea/workflows/release.yml` içindeki yayım adımıyla aynı
adları taşır:

| Değişken | Zorunlu | Açıklama |
|----------|---------|----------|
| `RELEASE_HOST` | — | Boşsa yerel mod. Doluysa uzak mod açılır. |
| `RELEASE_USER` | uzak modda evet | SSH kullanıcısı |
| `RELEASE_WWW_PATH` | uzak modda evet | Sunucudaki **www** kökü |
| `RELEASE_PORT` | hayır | SSH portu (varsayılan `22`) |
| `RELEASE_SSH_KEY` | hayır | Özel anahtar dosyası (`ssh -i`) |

`RELEASE_WWW_PATH` bilerek `RELEASE_PATH`'ten ayrıdır: `RELEASE_PATH` indirme
kökünü gösterir ve buraya verilirse `rsync --delete` tüm sürüm dosyalarını
siler. Zorunlu bir değişken boşsa betik hata yazıp `exit 1` ile durur; sessizce
devam etmez.

Docker önizlemesinin `.env.example` dosyası yalnızca yerel portları tanımlar.
Uzak yayın ayarları bu yerel profilden bağımsızdır; yukarıdaki ortam değişkenleri
ve Gitea yayın rehberi kullanılır. Kamusal siteye ait DNS ve VPS taşıma adımları
bu yerel önizlemenin kapsamına girmez.
