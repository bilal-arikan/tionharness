# TionHarness — Native Masaüstü Penceresi (Seçenek 2)

> Durum: **UYGULANDI ✅ (2026-06-22)**. `tionharness-desktop.exe` çift tıkla → tarayıcı
> sekmesi yerine **kendi WebView2 penceresinde** açılır. Başsız `tionharness.exe` aynen durur.
>
> Gerçekleşen mimari arşivdeki planla birebir: ayrı `internal/app` boot paketi (paylaşılan),
> `cmd/tionharness-desktop` (Windows-only, `jchv/go-webview2`, CGO'suz), `:0` boş port,
> `waitForHealth`, WebView2-yok → tarayıcı fallback, `scripts/build.ps1 -Desktop` (`-H windowsgui`).
> Canlı doğrulandı: sunucu rastgele portta boot, `/health`+`/` 200, pencere açıldı.

## Mimari (özet)

Başsız `cmd/tionharness` saf-Go/CGO'suz kalır; native pencere ayrı giriş noktasıdır:
`cmd/tionharness-desktop` (Windows-only, `jchv/go-webview2`, CGO'suz). Paylaşılan boot
`internal/app` (`Bootstrap`/`Serve`/`Shutdown`/`Addr`); masaüstü sunucuyu `127.0.0.1:0`
üzerinde başlatır, `waitForHealth` sonrası WebView2 penceresini açar, pencere kapanınca
graceful shutdown yapar. Derleme: `scripts/build.ps1 -Desktop` (`-H windowsgui`).

> Plan gövdesi (CGO analizi, refactor taslağı, `main.go` taslağı, bağımlılık, build script,
> doğrulama/uygulama adımları, efor) → [arsiv/32-NATIVE-PENCERE-PLAN.md](arsiv/32-NATIVE-PENCERE-PLAN.md).
> İkon artık `goversioninfo` ile üretilen `.syso` dosyalarıyla gömülür (aşağı bak).

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
için varsayılan olarak yeni bir konsol penceresi açar. Otonom ajan turları/auto-title/
scheduler periyodik olarak `claude-cli`'ye shell-out yaptığından "ara ara" görünüyordu.

**Çözüm:** yeni **`internal/proc`** paketi — `Hide(cmd)` Windows'ta `CREATE_NO_WINDOW`
(`0x08000000`) + `HideWindow` set eder (`hide_windows.go`), diğer platformlarda no-op
(`hide_other.go`). Konsol açan **tüm** `exec.Command` çağrılarına eklendi: `providers/claudecli.go`
(ana suçlu), `tools/builtin_shell.go`, `agent/hooks.go`,
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
`mcp/client.go`, `api/git.go`, `workdir_context.go`,
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

### Üçüncü yol: `proc.HideNested` / `proc.CommandContextNested` (torun doğuran çocuklar)

**Ne zaman:** alt proses KENDİ torun proseslerini doğuruyorsa — ör. `claude-cli` / `codex-cli`
turlarının başlattığı MCP stdio alt prosesleri veya turun kabuğa düşerek çalıştırdığı build
aracı. Çağrı yerleri: `internal/providers/claudecli.go`, `claudecli_session.go`, `codexcli.go`.

**Neden CREATE_NO_WINDOW yok:** `CREATE_NO_WINDOW` çocuğa **hiç konsol vermez**. Konsolsuz bir
ebeveyn, açık bir konsol bayrağı olmadan çocuk doğurduğunda Windows'un miras alacağı konsol
olmadığı için toruna **yepyeni ve GÖRÜNÜR** bir konsol tahsis eder — gizli bir CLI turundaki her
MCP araç çağrısında kullanıcının gördüğü terminal parlaması budur. Tek başına `HideWindow`
(`STARTF_USESHOWWINDOW` + `SW_HIDE`) çocuğa konsolu yine tahsis eder ama anında gizler; torunlar
da kendi konsollarını açmak yerine bu mevcut gizli konsolu miras alır.

**Ayrım korunmalı:** torun doğurmayan çağrılar (`ProbeAuth`, `cli_preflight.go`, `git.go`) eski
`proc.CommandContext` ile kalır — onlarda `CREATE_NO_WINDOW` doğru davranıştır.

## Kural testle zorunlu kılındı — kalan tüm çağrı yerleri ✅ (2026-08-28)

Merkezileştirme yapıldıktan sonra da yeni yazılan kodda düz `exec.Command*` sızmaya devam etti;
her sızıntı, paketli masaüstü uygulamasında yanıp sönen bir terminal demekti. Kalan sekiz çağrı
yeri `proc` üzerinden geçirildi:

| Dosya | Süreç | Kullanılan yardımcı |
|-------|-------|----------------------|
| `internal/agent/rtk_optimizer.go` | `rtk rewrite` | `proc.CommandContext` |
| `internal/agent/shell_optimizer.go` | `sqz compress` | `proc.CommandContext` |
| `internal/agent/capabilities.go` | codebase-memory `index_repository` | `proc.Command` |
| `internal/agent/handoff.go` | `git` | `proc.CommandContext` |
| `internal/api/external_tools_maint.go` | harici araç bakımı | `proc.CommandContext` |
| `internal/tools/builtin_codebase_search.go` | codebase-memory CLI | `proc.CommandContext` |
| `internal/worktree/git.go` (`ExecRunner`) | `git` | `proc.CommandContext` |
| `internal/stt/stt.go` | ffmpeg / whisper | `proc.CommandContext` |
| `cmd/tionharness-desktop/main.go` (`openBrowser`) | `rundll32` | `proc.HideConsole` |

İlk ikisi en görünür olanlardı: `rtk` ve `sqz` her shell aracı çağrısında koşar, yani her komutta
bir konsol yanıp sönüyordu.

**Regresyon kapısı:** `internal/proc/console_policy_test.go` (`TestNoRawExecCommand`) depo
ağacını tarar ve `exec.Command`/`exec.CommandContext` içerip `proc.Command*` / `proc.Hide*`
yardımcılarının hiçbirine dokunmayan her `.go` dosyasında FAIL verir. Taramadan hariç olanlar:
`internal/proc` (gizleme katmanının kendisi), `dist/`, `frontend/`, `website/`, `node_modules`,
`*_test.go`. Bilinçli istisna için dosyaya `exec-console-exempt` işaretçisi + gerekçe yorumu
konur — şu an tek kullanıcısı `cmd/tionharness-desktop/openwindow_windows.go` (GUI çocuğu;
`SW_HIDE` onun WebView2 penceresini gizlerdi).

## Çapraz platform yol haritası (sonraki, opsiyonel)

- macOS/Linux için `webview/webview_go` (CGO) ile `cmd/tionharness-desktop/main_unix.go`
  (`//go:build darwin || linux`). CGO + platform toolchain gerektirir; ayrı bir görev.
- Bu plan **Windows-first**; başsız binary tüm platformlarda CGO'suz kalmaya devam eder.

## Riskler ve ödünleşmeler

1. **`-H windowsgui` + log**: konsol olmayınca stdout logları görünmez. Çözüm: logbuf
   zaten UI "Loglar" ekranında; ayrıca `app.SetupLogging()` logları koşulsuz olarak
   data dizini altındaki dosyaya da yazar (`internal/app/app.go`, `config.LogFilePath()`).
2. **WebView2 sürümü**: çok eski Win10'larda runtime olmayabilir → fallback ele alınır (yukarıda).
3. **Bağımlılık yüzeyi**: `go-webview2` + `x/sys` eklenir; yalnız desktop hedefinde, kabul edilebilir.
4. **İki binary**: dağıtımda iki exe (`tionharness.exe` server, `tionharness-desktop.exe` masaüstü).
   İstenirse ileride tek exe + `--server` flag'iyle birleştirilebilir (ayrı iş).

## Uygulama ikonu (Windows exe kaynağı)

`build/windows/icon.ico` — "TH" monogramı, marka moru (`#863bff`), yuvarlatılmış rozet.
`frontend/public/favicon.svg` ve `website/public/favicon.svg` ile aynı biçim; ikon
`build/windows/make-icon.py` (Pillow) ile aynı rect'lerden rasterize edilir
(7 boy: 16→256).

İkon ve sürüm bilgisi exe'ye Windows kaynak nesnesi (`.syso`) olarak gömülür:

| Dosya | Kapsam |
|-------|--------|
| `cmd/tionharness/rsrc_windows_amd64.syso` | başsız sunucu, x64 |
| `cmd/tionharness/rsrc_windows_arm64.syso` | başsız sunucu, arm64 |
| `cmd/tionharness-desktop/rsrc_windows_*.syso` | native pencere derlemesi |

Bu dosyalar **depoya işlenmiştir** (`.gitignore` içinde `!cmd/*/rsrc_windows_*.syso`
istisnası), böylece düz `go build` bile markalı bir exe üretir — derleme zamanında
ek araç gerekmez. Go bunları dosya adındaki `_windows_<arch>` son ekine göre yalnız
ilgili hedefte bağlar; Linux/macOS derlemeleri etkilenmez.

Yeniden üretmek (ikon veya sürüm değişirse):

```bash
go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
for pkg in tionharness tionharness-desktop; do
  ~/go/bin/goversioninfo.exe -icon=build/windows/icon.ico -64 \
    -o cmd/$pkg/rsrc_windows_amd64.syso build/windows/versioninfo.json
  ~/go/bin/goversioninfo.exe -icon=build/windows/icon.ico -64 -arm=true \
    -o cmd/$pkg/rsrc_windows_arm64.syso build/windows/versioninfo.json
done
```
