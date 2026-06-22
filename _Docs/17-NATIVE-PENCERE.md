# SwarmGo — Native Masaüstü Penceresi (Seçenek 2)

> Durum: **UYGULANDI ✅ (2026-06-22)**. `swarmgo-desktop.exe` çift tıkla → tarayıcı
> sekmesi yerine **kendi WebView2 penceresinde** açılır. Başsız `swarmgo.exe` aynen durur.
>
> Gerçekleşen mimari aşağıdaki planla birebir: ayrı `internal/app` boot paketi (paylaşılan),
> `cmd/swarmgo-desktop` (Windows-only, `jchv/go-webview2`, CGO'suz), `:0` boş port,
> `waitForHealth`, WebView2-yok → tarayıcı fallback, `scripts/build.ps1 -Desktop` (`-H windowsgui`).
> Canlı doğrulandı: sunucu rastgele portta boot, `/health`+`/` 200, pencere açıldı.

## Amaç ve kısıt

Bugün tek binary UI'yı HTTP ile sunuyor ama görüntüleme tarayıcıda. Bu plan, gömülü UI'yı
**native bir pencerede** (sistem WebView motoru) gösterir — Electron/ayrı çalışma zamanı yok.

**Felsefe kısıtı:** Mevcut `cmd/swarmgo` (başsız sunucu) **saf-Go, CGO'suz, çapraz-derlenebilir**
kalmalı. Native pencere bunu bozmamalı.

## Anahtar bulgu — Windows'ta CGO gerekmez

| Platform | Kütüphane | CGO? | Çalışma zamanı bağımlılığı |
|----------|-----------|------|----------------------------|
| **Windows** | [`jchv/go-webview2`](https://github.com/jchv/go-webview2) | ❌ **Hayır** (saf Go) | WebView2 Runtime (Win11'de **yerleşik**) |
| macOS | `webview/webview_go` | ✅ Evet (Cocoa/WebKit) | Sistemde mevcut |
| Linux | `webview/webview_go` | ✅ Evet (GTK/WebKitGTK) | `libwebkit2gtk` paketi |

Sen Win11'desin → **CGO'suz, ek kurulumsuz** native pencere mümkün. macOS/Linux ileride
CGO'lu klasik webview ile eklenebilir (opsiyonel, build-tag'li).

## Mimari karar — ayrı binary + build tag (mevcut sunucuyu kirletme)

Başsız sunucu binary'sine dokunmadan, **ikinci bir giriş noktası** ekliyoruz:

```
cmd/
├── swarmgo/            # mevcut başsız sunucu (DEĞİŞMEZ, saf-Go, çapraz-derleme)
│   └── main.go
└── swarmgo-desktop/    # YENİ: native pencere sarmalayıcı (Windows-only, build-tag)
    └── main.go
```

`swarmgo-desktop`:
1. Sunucuyu **127.0.0.1:0** (OS'un seçtiği boş port) üzerinde başlatır.
2. Seçilen gerçek portu okur.
3. `webview2`'de bir pencere açar, `http://127.0.0.1:<port>`'a yönlendirir.
4. Pencere kapanınca sunucuyu graceful shutdown eder ve süreç biter.

Böylece sunucu mantığı **tek kaynaktan** paylaşılır; desktop yalnız "başlat + pencere + kapat".

## Ön koşul refactor — boot dizisini paylaşılabilir yap

`cmd/swarmgo/main.go` şu an tüm boot'u inline yapıyor (config → secret → settings →
registry → tunables → manager → server → ListenAndServe). İki giriş noktasının paylaşması için
bu dizi **`internal/app`** paketine çıkarılır:

```go
// internal/app/app.go
package app

// Bootstrap tüm alt sistemleri kurar ve dinlemeye hazır bir App döner.
// addr "127.0.0.1:0" verilirse OS boş port seçer; App.Addr() gerçek adresi verir.
func Bootstrap(cfg *config.Config, logger *slog.Logger) (*App, error)

func (a *App) Addr() string          // net.Listener.Addr() — gerçek port (0 ise çözülen)
func (a *App) Serve() error          // bloklar (mevcut go func gövdesi)
func (a *App) Shutdown(ctx) error    // graceful
```

- `Bootstrap` `net.Listen("tcp", cfg.Addr)` ile **önce listener** açar (port 0 desteği için),
  `http.Server{...}` + `IdleTimeout` (mevcut) korunur, `srv.Serve(ln)` kullanır.
- `cmd/swarmgo/main.go` ~100 satırdan ~15 satıra iner: `app.Bootstrap` + signal + `Serve`.
- **Davranış-korumalı**: mevcut başsız akış birebir aynı kalır (sadece taşındı).

## Yeni dosyalar

```
internal/app/app.go              # paylaşılan boot (Bootstrap/Serve/Shutdown/Addr)
cmd/swarmgo-desktop/main.go      # //go:build windows  — webview sarmalayıcı
cmd/swarmgo-desktop/doc.go       # //go:build !windows — boş stub + "yalnız Windows" notu
```

### `cmd/swarmgo-desktop/main.go` (taslak)

```go
//go:build windows

package main

func main() {
    logger := ...                       // mevcut slog kurulumu
    cfg, _ := config.Load()
    cfg.Addr = "127.0.0.1:0"            // boş port
    application, err := app.Bootstrap(cfg, logger)
    // ... hata → MessageBox + çık
    go application.Serve()
    url := "http://" + application.Addr()
    waitForHealth(url, 5*time.Second)   // /health 200 olana dek kısa poll

    w := webview2.NewWithOptions(webview2.WebViewOptions{
        Debug: false,
        WindowOptions: webview2.WindowOptions{
            Title:  "SwarmGo",
            Width:  1280, Height: 800,
            IconId: 0,                    // exe'ye gömülü ikon (opsiyonel)
            Center: true,
        },
    })
    if w == nil {
        // WebView2 Runtime yok → kullanıcıya indirme linki göster
    }
    defer w.Destroy()
    w.Navigate(url)
    w.Run()                              // bloklar; pencere kapanınca döner

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    application.Shutdown(ctx)
}
```

## Bağımlılık

- `go get github.com/jchv/go-webview2` → `go.mod`'a tek satır (saf Go; `golang.org/x/sys` getirir).
- Mevcut `cmd/swarmgo` build'i **etkilenmez** (yeni paket yalnız desktop binary'sinde import edilir).
- `go.mod` minimal bağımlılık ilkesi: bu paket yalnız desktop hedefinde derlenir; başsız
  sunucu/CI build'i hâlâ yalnız `uuid`+`cron`+`x/sys` ile çalışır.

## Build script değişikliği (`scripts/build.ps1`)

Yeni `-Desktop` bayrağı:

```powershell
.\scripts\build.ps1            # swarmgo.exe (başsız sunucu, mevcut)
.\scripts\build.ps1 -Desktop   # swarmgo-desktop.exe (native pencere, UI gömülü)
```

- `-Desktop`: UI build (aynı) + `go build -ldflags "-H windowsgui -s -w" -o swarmgo-desktop.exe ./cmd/swarmgo-desktop`
- **`-H windowsgui`**: konsol penceresi açılmasını engeller (çift tıkla → yalnız uygulama penceresi).

## Pencere ikonu (opsiyonel, hoş dokunuş)

`-H windowsgui` ile exe ikonu için `.syso` kaynağı gerekir (örn. `rsrc`/`go-winres`).
İlk sürümde atlanabilir; ikon istenirse `go-winres` ile `cmd/swarmgo-desktop/winres/`
eklenir (build-time, çalışma zamanı bağımlılığı değil).

## Yaşam döngüsü / kenar durumları

| Durum | Davranış |
|-------|----------|
| WebView2 Runtime yok (nadir, eski Win10) | `NewWithOptions` nil → MessageBox: "WebView2 Runtime gerekli" + Microsoft indirme linki; tarayıcı fallback'i aç |
| Port 0 → çözülen port | `listener.Addr()` ile okunur; webview ona yönlendirilir |
| Pencere kapatıldı | `w.Run()` döner → `Shutdown` → süreç biter (orphan sunucu kalmaz) |
| Sunucu boot hatası | MessageBox + exit (pencere açılmadan) |
| Başsız sunucu hâlâ isteniyor | `swarmgo.exe` aynen durur; iki dağıtım yan yana |

## Çapraz platform yol haritası (sonraki, opsiyonel)

- macOS/Linux için `webview/webview_go` (CGO) ile `cmd/swarmgo-desktop/main_unix.go`
  (`//go:build darwin || linux`). CGO + platform toolchain gerektirir; ayrı bir görev.
- Bu plan **Windows-first**; başsız binary tüm platformlarda CGO'suz kalmaya devam eder.

## Riskler ve ödünleşmeler

1. **`-H windowsgui` + log**: konsol olmayınca stdout logları görünmez. Çözüm: logbuf
   zaten UI "Loglar" ekranında; ayrıca dosyaya log opsiyonu (`SWARMGO_LOG_FILE`) eklenebilir.
2. **WebView2 sürümü**: çok eski Win10'larda runtime olmayabilir → fallback ele alınır (yukarıda).
3. **Bağımlılık yüzeyi**: `go-webview2` + `x/sys` eklenir; yalnız desktop hedefinde, kabul edilebilir.
4. **İki binary**: dağıtımda iki exe (`swarmgo.exe` server, `swarmgo-desktop.exe` masaüstü).
   İstenirse ileride tek exe + `--server` flag'iyle birleştirilebilir (ayrı iş).

## Doğrulama planı

1. `go build ./...` + `go vet` (tüm hedefler) yeşil; başsız `swarmgo.exe` davranışı değişmedi
   (smoke: `/health`, `/` HTML 200 — mevcut testler).
2. `swarmgo-desktop.exe` çift tıkla → pencere açılır, UI yüklenir, sohbet/akış çalışır.
3. Pencereyi kapat → süreç sonlanır (Görev Yöneticisi'nde artık process yok).
4. Port çakışması testi: iki desktop instance aynı anda (her biri farklı port 0 alır).

## Uygulama adımları (özet sıra)

1. `internal/app` paketi: boot dizisini taşı (davranış-korumalı refactor) + `cmd/swarmgo` onu kullansın.
2. `go get github.com/jchv/go-webview2`.
3. `cmd/swarmgo-desktop/main.go` (`//go:build windows`) + `!windows` stub.
4. `waitForHealth` helper (kısa poll) + WebView2-yok fallback (MessageBox/tarayıcı).
5. `scripts/build.ps1` `-Desktop` bayrağı + `-H windowsgui`.
6. (Opsiyonel) `go-winres` ile pencere/exe ikonu.
7. Build + canlı doğrulama (yukarıdaki 4 madde).
8. README "Native Masaüstü" bölümü + `_Docs/05-ILERLEME.md` girdisi.

## Tahmini efor

- Çekirdek (1–5): ~1 oturum, orta büyüklük. Refactor (`internal/app`) en dikkatli kısım
  (davranış-korumalı olmalı). Webview sarmalayıcı küçük.
- İkon + çapraz platform: ayrı, opsiyonel turlar.
