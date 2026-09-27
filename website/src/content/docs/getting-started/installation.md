---
title: Kurulum
description: Kaynaktan derleme ve ilk çalıştırma.
order: 2
---

Kaynaktan derleme, yayımlanmış binary bulunmasına bağlı olmayan kurulum yoludur.
Go 1.26.4+ ve Node.js 20.19+ (20.x) veya 22.12+ gerekir; geliştirme ortamında
Node.js 24 kullanılır. Git ve npm komutları erişilebilir olmalıdır.

## Windows

PowerShell içinde:

```powershell
git clone https://github.com/bilal-arikan/tionharness
cd tionharness
.\scripts\build.ps1
$env:TIONHARNESS_ADDR="127.0.0.1:8095"
.\tionharness.exe
```

Derleme, frontend çıktısını `internal/web/dist/` altına yazar ve Go uygulamasına
gömer. `http://127.0.0.1:8095` adresini açın. Kendi penceresinde çalışan sürüm için
`build.ps1 -Desktop` kullanın; üretilen `tionharness-desktop.exe` WebView2 runtime ister.

## Linux ve macOS

```bash
git clone https://github.com/bilal-arikan/tionharness
cd tionharness
cd frontend && npm ci && npm run build && cd ..
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o tionharness ./cmd/tionharness
TIONHARNESS_ADDR=127.0.0.1:8095 ./tionharness
```

## Doğrulama

`http://127.0.0.1:8095/health` adresi başarılı yanıt vermeli; kök adres arayüzü
sunmalıdır. UI olmadan yalnız Go derlemesi yaptıysanız frontend'i derleyip binary'yi
yeniden üretin. Sunucuyu terminalde Ctrl+C ile kapatın.

Veriler varsayılan `~/.tionharness` dizininde saklanır. Farklı bir dizinle denemek
ve dinleme adresini değiştirmek için [yapılandırma](/docs/reference/configuration)
rehberine bakın.
