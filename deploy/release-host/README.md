# TionHarness sürüm sunucusu — YEREL ÖNİZLEME

> **Bu birim üretimde kullanılmaz.** Üretimde ortada VPS yoktur: binary'ler GitHub
> Release asset'i, `latest.json` ise GitHub Pages üzerinden
> `https://tionharness.com/latest.json` adresinde sunulur
> (`.github/workflows/release.yml` + `pages.yml`, ayrıntı:
> `_Docs\75-YAYIN-SURECI.md`). Aşağıdaki Caddy birimi yalnızca feed + indirme
> yerleşimini **yerelde** denemek içindir.

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

## Sürüm yayımlama

Önce depo kökünde sürüm çıktısını üretin:

```bash
./scripts/build-release.sh <version>
```

Bu betik başka çalışma kapsamında hazırlanıyor ve henüz mevcut olmayabilir. Ardından bu dizinde çıktıyı yayımlayın:

```bash
./sync-release.sh <version>
```

Kontrol:

```bash
curl http://localhost:8080/latest.json
```

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

## VPS'e taşıma (ARTIK GEÇERLİ DEĞİL)

Aşağıdaki adımlar yalnızca tarihsel referanstır; mevcut yayın mimarisi VPS
kullanmaz. Kendi barındırma altyapınıza taşımak isterseniz izlenecek yol budur:

1. `deploy/release-host/` dizinini VPS'e kopyalayın.
2. `.env.example` dosyasını `.env` olarak kopyalayıp üretim profilindeki site adreslerini ve `80`/`443` portlarını ayarlayın.
3. VPS güvenlik duvarında TCP 80 ve 443 portlarını açın.
4. DNS sağlayıcınızın yönetim panelinde `dl`, `www` ve apex alan adları için VPS IP adresini gösteren A kayıtları ekleyin.
5. `docker compose up -d` çalıştırın. Caddy sertifikaları otomatik alır ve yeniler.

`caddy_data` named volume kalıcı tutulmalıdır. Bu volume silinirse sertifikalar yeniden alınır ve sık tekrarda Let's Encrypt hız sınırlarına takılabilirsiniz.
