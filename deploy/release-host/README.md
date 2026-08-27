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

## VPS'e taşıma (ARTIK GEÇERLİ DEĞİL)

Aşağıdaki adımlar yalnızca tarihsel referanstır; mevcut yayın mimarisi VPS
kullanmaz. Kendi barındırma altyapınıza taşımak isterseniz izlenecek yol budur:

1. `deploy/release-host/` dizinini VPS'e kopyalayın.
2. `.env.example` dosyasını `.env` olarak kopyalayıp üretim profilindeki site adreslerini ve `80`/`443` portlarını ayarlayın.
3. VPS güvenlik duvarında TCP 80 ve 443 portlarını açın.
4. DNS sağlayıcısı DNS yönetiminde `dl`, `www` ve apex alan adları için VPS IP adresini gösteren A kayıtları ekleyin.
5. `docker compose up -d` çalıştırın. Caddy sertifikaları otomatik alır ve yeniler.

`caddy_data` named volume kalıcı tutulmalıdır. Bu volume silinirse sertifikalar yeniden alınır ve sık tekrarda Let's Encrypt hız sınırlarına takılabilirsiniz.
