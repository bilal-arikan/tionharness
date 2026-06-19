# SwarmGo — tek binary üretim derlemesi.
# Frontend'i derler (Vite → internal/web/dist), ardından UI'yı gömen tek bir
# Go binary'si üretir. Sonuç: swarmgo.exe (çift tıkla → hem API hem UI tek portta).
#
# Kullanım:  .\scripts\build.ps1            # swarmgo.exe üretir
#            .\scripts\build.ps1 -SkipUI    # yalnız backend (mevcut dist'i gömer)

param(
    [switch]$SkipUI,
    [string]$Output = "swarmgo.exe"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

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

Write-Host "==> Backend derleniyor (UI gömülü tek binary)..." -ForegroundColor Cyan
go build -trimpath -ldflags "-s -w" -o $Output ./cmd/swarmgo

$size = "{0:N1} MB" -f ((Get-Item $Output).Length / 1MB)
Write-Host "==> Tamam: $root\$Output ($size)" -ForegroundColor Green
Write-Host "    Çalıştır:  `$env:SWARMGO_ADDR='127.0.0.1:8095'; .\$Output   → http://127.0.0.1:8095" -ForegroundColor Green
