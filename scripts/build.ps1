# TionHarness — tek binary üretim derlemesi.
# Frontend'i derler (Vite → internal/web/dist), ardından UI'yı gömen tek bir
# Go binary'si üretir.
#
# Kullanım:  .\scripts\build.ps1            # tionharness.exe (başsız sunucu + gömülü UI, tarayıcıda açılır)
#            .\scripts\build.ps1 -Desktop   # tionharness-desktop.exe (native WebView2 penceresi, Windows)
#            .\scripts\build.ps1 -SkipUI    # UI build'ini atla (mevcut dist'i gömer)

param(
    [switch]$SkipUI,
    [switch]$Desktop,
    [string]$Output
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
$buildCommit = (git rev-parse --short HEAD).Trim()
$buildTime = [DateTime]::UtcNow.ToString("yyyy-MM-ddTHH:mm:ssZ")
$versionFlags = "-X github.com/bilal-arikan/tionharness/internal/api.BuildCommit=$buildCommit -X github.com/bilal-arikan/tionharness/internal/api.BuildDate=$buildTime"

if (-not $SkipUI) {
    Write-Host "==> Frontend derleniyor (npm run build)..." -ForegroundColor Cyan
    Push-Location "$root\frontend"
    if (-not (Test-Path "node_modules")) {
        Write-Host "    node_modules yok, npm install çalışıyor..." -ForegroundColor Yellow
        npm install
    }
    npm run build
    Pop-Location
    # emptyOutDir wipes the tracked placeholder; restore it so a fresh checkout
    # (without a build) still compiles `//go:embed all:dist`.
    $keep = "$root\internal\web\dist\.gitkeep"
    if (-not (Test-Path $keep)) {
        "Frontend build placeholder. Real assets are emitted here by 'npm run build' (vite outDir) and embedded via go:embed. Do not delete this file." |
            Out-File -FilePath $keep -Encoding utf8 -NoNewline
    }
}

if ($Desktop) {
    if (-not $Output) { $Output = "tionharness-desktop.exe" }
    Write-Host "==> Native masaüstü uygulaması derleniyor (WebView2 penceresi, UI gömülü)..." -ForegroundColor Cyan
    # -H windowsgui: çift tıkla → konsol penceresi açılmaz, yalnız uygulama penceresi.
    go build -trimpath -ldflags "-H windowsgui -s -w $versionFlags" -o $Output ./cmd/tionharness-desktop
    $hint = "Çift tıkla → kendi penceresinde açılır (tarayıcı gerekmez)."
} else {
    if (-not $Output) { $Output = "tionharness.exe" }
    Write-Host "==> Başsız sunucu derleniyor (UI gömülü tek binary)..." -ForegroundColor Cyan
    go build -trimpath -ldflags "-s -w $versionFlags" -o $Output ./cmd/tionharness
    $hint = "Çalıştır:  `$env:TIONHARNESS_ADDR='127.0.0.1:8095'; .\$Output   → http://127.0.0.1:8095"
}

$size = "{0:N1} MB" -f ((Get-Item $Output).Length / 1MB)
Write-Host "==> Tamam: $root\$Output ($size)" -ForegroundColor Green
Write-Host "    $hint" -ForegroundColor Green
