# TionHarness — Native Masaüstü Penceresi (Seçenek 2)

> Durum: **UYGULANDI ✅ (2026-06-22)**. `tionharness-desktop.exe` çift tıkla → tarayıcı
> sekmesi yerine **kendi WebView2 penceresinde** açılır. Başsız `tionharness.exe` aynen durur.
>
> Gerçekleşen mimari aşağıdaki planla birebir: ayrı `internal/app` boot paketi (paylaşılan),
> `cmd/tionharness-desktop` (Windows-only, `jchv/go-webview2`, CGO'suz), `:0` boş port,
> `waitForHealth`, WebView2-yok → tarayıcı fallback, `scripts/build.ps1 -Desktop` (`-H windowsgui`).
> Canlı doğrulandı: sunucu rastgele portta boot, `/health`+`/` 200, pencere açıldı.

## Amaç ve kısıt

Bugün tek binary UI'yı HTTP ile sunuyor ama görüntüleme tarayıcıda. Bu plan, gömülü UI'yı
**native bir pencerede** (sistem WebView motoru) gösterir — Electron/ayrı çalışma zamanı yok.

**Felsefe kısıtı:** Mevcut `cmd/tionharness` (başsız sunucu) **saf-Go, CGO'suz, çapraz-derlenebilir**
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
├── tionharness/            # mevcut başsız sunucu (DEĞİŞMEZ, saf-Go, çapraz-derleme)
│   └── main.go
└── tionharness-desktop/    # YENİ: native pencere sarmalayıcı (Windows-only, build-tag)
    └── main.go
```

`tionharness-desktop`:
1. Sunucuyu **127.0.0.1:0** (OS'un seçtiği boş port) üzerinde başlatır.
2. Seçilen gerçek portu okur.
3. `webview2`'de bir pencere açar, `http://127.0.0.1:<port>`'a yönlendirir.
4. Pencere kapanınca sunucuyu graceful shutdown eder ve süreç biter.

Böylece sunucu mantığı **tek kaynaktan** paylaşılır; desktop yalnız "başlat + pencere + kapat".

## Ön koşul refactor — boot dizisini paylaşılabilir yap

`cmd/tionharness/main.go` şu an tüm boot'u inline yapıyor (config → secret → settings →
registry → tunables → manager → server → ListenAndServe). İki giriş noktasının paylaşması için
bu dizi **`internal/app`** paketine çıkarılır:

```go
// internal/app/app.go
package app

// Bootstrap tüm alt sistemleri kurar ve dinlemeye hazır bir App döner.
// addr "127.0.0.1:0" verilirse OS boş port seçer; App.Addr() gerçek adresi verir.
func Bootstrap(cfg *config.Config, logs *logbuf.Buffer, logger *slog.Logger) (*App, error)

func (a *App) Addr() string          // net.Listener.Addr() — gerçek port (0 ise çözülen)
func (a *App) Serve() error          // bloklar (mevcut go func gövdesi)
func (a *App) Shutdown(ctx) error    // graceful
```

- `Bootstrap` `net.Listen("tcp", cfg.Addr)` ile **önce listener** açar (port 0 desteği için),
  `http.Server{...}` + `IdleTimeout` (mevcut) korunur, `srv.Serve(ln)` kullanır.
- `cmd/tionharness/main.go` ~100 satırdan ~15 satıra iner: `app.Bootstrap` + signal + `Serve`.
- **Davranış-korumalı**: mevcut başsız akış birebir aynı kalır (sadece taşındı).

## Yeni dosyalar

```
internal/app/app.go              # paylaşılan boot (Bootstrap/Serve/Shutdown/Addr)
cmd/tionharness-desktop/main.go      # //go:build windows  — webview sarmalayıcı
cmd/tionharness-desktop/stub_other.go # //go:build !windows — boş stub + "yalnız Windows" notu
cmd/tionharness-desktop/window_windows.go, openwindow_windows.go, titlebar_windows.go, dpi_windows.go  # Windows-özel pencere/DPI/titlebar
```

### `cmd/tionharness-desktop/main.go` (taslak)

```go
//go:build windows

package main

func main() {
    logger := ...                       // mevcut slog kurulumu
    cfg, _ := config.Load()
    cfg.Addr = "127.0.0.1:0"            // boş port
    application, err := app.Bootstrap(cfg, logs, logger)
    // ... hata → MessageBox + çık
    go application.Serve()
    url := "http://" + application.Addr()
    waitForHealth(url, 5*time.Second)   // /health 200 olana dek kısa poll

    w := webview2.NewWithOptions(webview2.WebViewOptions{
        Debug: false,
        WindowOptions: webview2.WindowOptions{
            Title:  "TionHarness",
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
- Mevcut `cmd/tionharness` build'i **etkilenmez** (yeni paket yalnız desktop binary'sinde import edilir).
- `go.mod` minimal bağımlılık ilkesi: bu paket yalnız desktop hedefinde derlenir; başsız
  sunucu/CI build'i hâlâ yalnız `uuid`+`cron`+`x/sys` ile çalışır.

## Build script değişikliği (`scripts/build.ps1`)

Yeni `-Desktop` bayrağı:

```powershell
.\scripts\build.ps1            # tionharness.exe (başsız sunucu, mevcut)
.\scripts\build.ps1 -Desktop   # tionharness-desktop.exe (native pencere, UI gömülü)
```

- `-Desktop`: UI build (aynı) + `go build -ldflags "-H windowsgui -s -w" -o tionharness-desktop.exe ./cmd/tionharness-desktop`
- **`-H windowsgui`**: konsol penceresi açılmasını engeller (çift tıkla → yalnız uygulama penceresi).

## Pencere ikonu (opsiyonel, hoş dokunuş)

`-H windowsgui` ile exe ikonu için `.syso` kaynağı gerekir (örn. `rsrc`/`go-winres`).
İlk sürümde atlanabilir; ikon istenirse `go-winres` ile `cmd/tionharness-desktop/winres/`
eklenir (build-time, çalışma zamanı bağımlılığı değil).

## Yaşam döngüsü / kenar durumları

| Durum | Davranış |
|-------|----------|
| WebView2 Runtime yok (nadir, eski Win10) | `NewWithOptions` nil → MessageBox: "WebView2 Runtime gerekli" + Microsoft indirme linki; tarayıcı fallback'i aç |
| Port 0 → çözülen port | `listener.Addr()` ile okunur; webview ona yönlendirilir |
| Pencere kapatıldı | `w.Run()` döner → `Shutdown` → süreç biter (orphan sunucu kalmaz) |
| Sunucu boot hatası | MessageBox + exit (pencere açılmadan) |
| Başsız sunucu hâlâ isteniyor | `tionharness.exe` aynen durur; iki dağıtım yan yana |

## Başlık çubuğu tema uyumu ✅ (2026-06-23)

Native başlık çubuğu (caption + küçült/büyüt/kapat butonları + kenarlık) uygulama temasına
boyanır. **`cmd/tionharness-desktop/titlebar_windows.go`** (`//go:build windows`, salt `syscall`,
`dwmapi.dll`):

- `DWMWA_USE_IMMERSIVE_DARK_MODE` (20) — koyu/açık frame (Win10 1809+).
- `DWMWA_CAPTION_COLOR` (35) / `DWMWA_TEXT_COLOR` (36) / `DWMWA_BORDER_COLOR` (34) — palet
  renkleri (Win11 22000+). Desteklenmeyen build'lerde sessizce yok sayılır.
- `presetTitleColors` haritası 8 curated paletin `bg/text/border`'ını tutar (`themePresets.ts`
  ile elle senkron — başlık çubuğu yalnız bu üç token'a ihtiyaç duyar). Hex → COLORREF `0x00BBGGRR`.
- `app.App.Appearance()` çözülen preset/theme/accent'i sızıntısız verir.
- **Canlı:** `watchTitleBar` (1.5sn poll) tema değişince `w.Dispatch` ile yeniden uygular.

Bilinmeyen/legacy tema → yalnız dark/light frame (caption rengi atlanır), asla kırılmaz.

## Konsol penceresi yanıp sönmesi düzeltildi ✅ (2026-06-23)

**Belirti:** `-H windowsgui` ile konsolsuz derlenen desktop binary çalışırken ekranda ara ara
bir terminal penceresi açılıp kapanıyordu. **Sebep:** konsolsuz GUI süreci bir konsol alt-süreci
(claude CLI, PowerShell shell aracı/hook, git, MCP stdio sunucusu) başlattığında Windows o çocuk
için varsayılan olarak yeni bir konsol penceresi açar. Otonom ajan turları/heartbeat/auto-title/
scheduler periyodik olarak `claude-cli`'ye shell-out yaptığından "ara ara" görünüyordu.

**Çözüm:** yeni **`internal/proc`** paketi — `Hide(cmd)` Windows'ta `CREATE_NO_WINDOW`
(`0x08000000`) + `HideWindow` set eder (`hide_windows.go`), diğer platformlarda no-op
(`hide_other.go`). Konsol açan **tüm** `exec.Command` çağrılarına eklendi: `providers/claudecli.go`
(ana suçlu), `tools/builtin_shell.go`, `agent/hooks.go`, `agent/worktree.go` (git worktree),
`mcp/client.go` (stdio sunucu), `api/git.go`, `api/workdir_context.go`, `api/workspaces.go`
(klasör seçici PowerShell — dialog yine görünür, yalnız konsol gizlenir). `explorer.exe` çağrıları
zaten GUI olduğundan dokunulmadı. ✅ `go build ./...`/`vet` + `go test ./internal/...` (305) yeşil.
Not: başsız `tionharness.exe` zaten konsollu olduğundan etkilenmezdi; bayrak orada da zararsız.

## Konsol gizleme merkezileştirildi — proc.Command factory ✅ (2026-06-23)

Önceki düzeltmede her `exec.Command` sitesine ayrı ayrı `proc.Hide(cmd)` ekleniyordu. Artık
`internal/proc` bir **fabrika** sunuyor: `Command(name, args...)` / `CommandContext(ctx, name,
args...)` — `exec.Command*`'ı sarıp `Hide`'ı baştan uygular (`command.go`). Konsol açan tüm
siteler `exec.Command*` yerine `proc.Command*` kullanır → gizleme kuralı **tek yerde**, site başına
**tek satır**, ve `exec.Command`'ı doğrudan çağırmadığın sürece gizlemeyi **unutmak imkânsız**.
Dönüştürülen siteler: `providers/claudecli.go`, `tools/builtin_shell.go`, `agent/hooks.go`,
`agent/worktree.go` (3 git çağrısı), `mcp/client.go`, `api/git.go`, `workdir_context.go`,
`workspaces.go`. `explorer.exe` siteleri (GUI, yanıp sönmez) `exec` olarak kaldı. Artık kullanılmayan
`os/exec` importları temizlendi. ✅ `go build ./...`/`vet` + e2e-harici testler yeşil.

## Bulanık metin düzeltildi — DPI farkındalığı ✅ (2026-06-23)

**Belirti:** Pencerede tüm metinler hafif bulanık (tarayıcıda net). **Sebep:** Yüksek-DPI
ekranlarda (ölçek >%100) DPI-aware işaretlenmemiş pencereyi Windows 96 DPI'da çizip bitmap
olarak büyütür → bulanıklık. **Çözüm:** `cmd/tionharness-desktop/dpi_windows.go` (salt `syscall`,
`user32.dll`): `setDPIAware()` **pencere oluşturulmadan önce** (main'in ilk satırı)
`SetProcessDpiAwarenessContext(PER_MONITOR_AWARE_V2 = -4)` çağırır; eski Windows'ta
`SetProcessDPIAware` fallback. WebView2 host sürecin DPI farkındalığını izlediğinden artık
gerçek piksel yoğunluğunda **keskin** render eder. DPI farkındalığı süreçte yalnız bir kez
ayarlanabildiğinden çağrı en başta yapılır (ilk çağrı kazanır). ✅ build yeşil.

## "Path aç" + "Klasör seç" butonları düzeltildi ✅ (2026-06-23)

Masaüstünde iki ayrı kök sebep:

1. **Klasör seç (workspace oluşturma):** `/api/pick-folder` PowerShell FolderBrowserDialog'u
   `proc.Command` ile başlatıyordu; `proc.Command`'in `HideWindow` (`SW_HIDE`) bayrağı **dialogu da
   gizliyordu** (ShowDialog modal olarak bloklar ama görünmez → "açılmıyor"). Düzeltme: yeni
   `proc.HideConsole` (yalnız `CREATE_NO_WINDOW`, GUI penceresini gizlemez) + düz `exec.CommandContext`.
   Canlı doğrulandı (UIAutomation): `#32770` "Klasöre Gözat" dialogu görünür (count 1).
2. **Path aç (Explorer reveal — ajan/artifact/oturum/skill/prompt/ws-config):**
   `exec.CommandContext(r.Context(), "explorer.exe", ...).Start()` ateşle-unut bir launch'ı **istek
   context'ine** bağlıyordu; handler dönünce context iptal olup explorer **açılmadan öldürülüyordu**
   (yarış). Düzeltme: `exec.Command(...)` (context'ten ayrık) → explorer handler dönse de yaşar.

`proc.Hide` (HideWindow+CREATE_NO_WINDOW) yalnız saf konsol çocukları (git/claude/sh) için;
GUI dialog açan konsol çocuğu için `proc.HideConsole`.

## Çapraz platform yol haritası (sonraki, opsiyonel)

- macOS/Linux için `webview/webview_go` (CGO) ile `cmd/tionharness-desktop/main_unix.go`
  (`//go:build darwin || linux`). CGO + platform toolchain gerektirir; ayrı bir görev.
- Bu plan **Windows-first**; başsız binary tüm platformlarda CGO'suz kalmaya devam eder.

## Riskler ve ödünleşmeler

1. **`-H windowsgui` + log**: konsol olmayınca stdout logları görünmez. Çözüm: logbuf
   zaten UI "Loglar" ekranında; ayrıca dosyaya log opsiyonu (`TIONHARNESS_LOG_FILE`) eklenebilir.
2. **WebView2 sürümü**: çok eski Win10'larda runtime olmayabilir → fallback ele alınır (yukarıda).
3. **Bağımlılık yüzeyi**: `go-webview2` + `x/sys` eklenir; yalnız desktop hedefinde, kabul edilebilir.
4. **İki binary**: dağıtımda iki exe (`tionharness.exe` server, `tionharness-desktop.exe` masaüstü).
   İstenirse ileride tek exe + `--server` flag'iyle birleştirilebilir (ayrı iş).

## Doğrulama planı

1. `go build ./...` + `go vet` (tüm hedefler) yeşil; başsız `tionharness.exe` davranışı değişmedi
   (smoke: `/health`, `/` HTML 200 — mevcut testler).
2. `tionharness-desktop.exe` çift tıkla → pencere açılır, UI yüklenir, sohbet/akış çalışır.
3. Pencereyi kapat → süreç sonlanır (Görev Yöneticisi'nde artık process yok).
4. Port çakışması testi: iki desktop instance aynı anda (her biri farklı port 0 alır).

## Uygulama adımları (özet sıra)

1. `internal/app` paketi: boot dizisini taşı (davranış-korumalı refactor) + `cmd/tionharness` onu kullansın.
2. `go get github.com/jchv/go-webview2`.
3. `cmd/tionharness-desktop/main.go` (`//go:build windows`) + `!windows` stub.
4. `waitForHealth` helper (kısa poll) + WebView2-yok fallback (MessageBox/tarayıcı).
5. `scripts/build.ps1` `-Desktop` bayrağı + `-H windowsgui`.
6. (Opsiyonel) `go-winres` ile pencere/exe ikonu.
7. Build + canlı doğrulama (yukarıdaki 4 madde).
8. README "Native Masaüstü" bölümü + `_Docs/05-ILERLEME.md` girdisi.

## Tahmini efor

- Çekirdek (1–5): ~1 oturum, orta büyüklük. Refactor (`internal/app`) en dikkatli kısım
  (davranış-korumalı olmalı). Webview sarmalayıcı küçük.
- İkon + çapraz platform: ayrı, opsiyonel turlar.
