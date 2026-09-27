---
title: Yapılandırma
description: Ortam değişkenleri, ayar dosyaları ve kimlik doğrulama.
order: 1
---

Sunucunun dinleme adresi ortam değişkeninden gelir; `settings.json` içinde `port`
ayarı yoktur. Genel tercihler arayüzden yönetilir ve veri dizinine kaydedilir.

## Başlangıç değişkenleri

| Değişken | Varsayılan / davranış |
|---|---|
| `TIONHARNESS_ADDR` | `127.0.0.1:8080` |
| `TIONHARNESS_DATA_DIR` | Kullanıcının ev dizininde `.tionharness` |
| `TIONHARNESS_WORKSPACE_DIR` | Veri dizini altında `workspace`; çalışma kökü varsayılanı |
| `TIONHARNESS_API_AUTH_TOKEN` | Boş; ayarlanırsa bearer doğrulaması etkinleşir |
| `CREDENTIAL_SECRET` | Ayarlı değilse veri dizinindeki `credential-secret` kullanılır/üretilir; açık değer base64 kodlanmış 32 bayt olmalıdır |

Windows PowerShell örneği:

```powershell
$env:TIONHARNESS_ADDR = "127.0.0.1:8095"
$env:TIONHARNESS_DATA_DIR = Join-Path $env:TEMP "tionharness-trial"
.\tionharness.exe
```

`scripts/dev.ps1` farklı varsayılan kullanır: backend `0.0.0.0:8090`, frontend
5173. Yalnız yerel erişim için `-Loopback` verin.

## Ayar dosyaları

`settings.json` uygulama-geneli tercihleri, `providers.json` sağlayıcı örneklerini
tutar. `theme`, `themePreset`, `language` ve `uiLanguage` ayrı alanlardır; boş
`uiLanguage`, ajan yanıt dilini izler. Workspace ayarları `ws-settings.json`
içindedir. Dosyaları elle değiştirecekseniz çalışan uygulamayı önce kapatın.

JSON dosyalarının tamamı şifreli değildir; hassas anahtar alanları şifrelenir.
Şifrelenmiş kayıtları taşırken `credential-secret` veya aynı `CREDENTIAL_SECRET`
değeri gerekir. CLI login evleri `provider-homes/<instance-id>` altında ayrıca tutulur.

## HTTP kimlik doğrulaması

Token ayarlanmadığında erişim kimliksizdir. Token ayarlanırsa `/api/*` isteklerinde
`Authorization: Bearer <token>` gerekir; `/health` ve CORS ön kontrolü muaftır.
Paketlenmiş arayüz token göndermediği için bu ayar UI erişimini de engeller.
`ACCESS_KEY` eski ve etkisiz bir değişkendir; bu korumayı açmaz.

Normal yerel kullanımda loopback adresini koruyun. Yapılandırılmış auth tek başına
çok kullanıcılı hesap veya yetkilendirme sistemi oluşturmaz.
