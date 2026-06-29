# SwarmGo - development run (backend + frontend together).
#
# Starts the Go backend (go run ./cmd/swarmgo on 127.0.0.1:8090) and the Vite
# dev server (npm run dev on :5173, proxies /api to 8090) side by side.
# Closing this window or pressing Ctrl+C kills BOTH process trees so no orphan
# node/go server is left listening.
#
# NOTE: keep this file ASCII-only. Windows PowerShell 5.1 mis-decodes a UTF-8
# (no BOM) script as ANSI, which corrupts non-ASCII chars (em-dash, Turkish
# letters) and breaks string parsing inside Start-Job.
#
# Usage:  .\scripts\dev.ps1                 # backend + frontend, open browser at :5173
#         .\scripts\dev.ps1 -NoBrowser      # don't auto-open the browser
#         .\scripts\dev.ps1 -BackendOnly    # only the Go backend
#         .\scripts\dev.ps1 -FrontendOnly   # only the Vite dev server
#         .\scripts\dev.ps1 -Port 8091      # override backend port

param(
    [int]$Port = 8090,
    [switch]$NoBrowser,
    [switch]$BackendOnly,
    [switch]$FrontendOnly
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

# Track launched processes so the finally block can tear them (and their
# children: go->compiled exe, npm->node) down on exit.
$procs = @()

function Stop-Tree($p) {
    if ($null -eq $p) { return }
    try {
        if (-not $p.HasExited) {
            # /T kills the whole tree (go run's child binary, npm's node), /F forces it.
            taskkill /PID $p.Id /T /F 2>$null | Out-Null
        }
    } catch { }
}

try {
    if (-not $FrontendOnly) {
        Write-Host "==> Backend baslatiliyor: go run ./cmd/swarmgo  (127.0.0.1:$Port)" -ForegroundColor Cyan
        $env:SWARMGO_ADDR = "127.0.0.1:$Port"
        # Gated features (see SKILL.md / Ortam Notlari): shell + self-management.
        $env:SWARMGO_ENABLE_SHELL = "1"
        $env:SWARMGO_ENABLE_SELFMANAGE = "1"
        $backend = Start-Process -FilePath "go" -ArgumentList "run", "./cmd/swarmgo" `
            -WorkingDirectory $root -NoNewWindow -PassThru
        $procs += $backend
    }

    if (-not $BackendOnly) {
        $fe = Join-Path $root "frontend"
        if (-not (Test-Path (Join-Path $fe "node_modules"))) {
            Write-Host "==> node_modules yok, npm install calisiyor..." -ForegroundColor Yellow
            Push-Location $fe
            npm install
            Pop-Location
        }
        Write-Host "==> Frontend baslatiliyor: npm run dev  (http://127.0.0.1:5173)" -ForegroundColor Cyan
        # npm.cmd: on Windows npm is a batch shim; call the .cmd directly.
        $frontend = Start-Process -FilePath "npm.cmd" -ArgumentList "run", "dev" `
            -WorkingDirectory $fe -NoNewWindow -PassThru
        $procs += $frontend
    }

    if (-not $NoBrowser -and -not $BackendOnly) {
        # Give Vite a moment to come up, then open the browser.
        Start-Sleep -Seconds 3
        Start-Process "http://127.0.0.1:5173"
    }

    Write-Host ""
    Write-Host "==> Calisiyor. Durdurmak icin Ctrl+C (ya da pencereyi kapat) -> ikisi de kapanir." -ForegroundColor Green
    Write-Host ""

    # Wait until one of them exits; if one dies, take the others down too.
    while ($true) {
        Start-Sleep -Milliseconds 500
        foreach ($p in $procs) {
            if ($p.HasExited) {
                Write-Host "==> Bir surec sonlandi (PID $($p.Id)), digerleri kapatiliyor..." -ForegroundColor Yellow
                throw "child-exited"
            }
        }
    }
}
finally {
    Write-Host "==> Temizlik: surecler sonlandiriliyor..." -ForegroundColor Cyan
    foreach ($p in $procs) { Stop-Tree $p }
    Write-Host "==> Kapandi." -ForegroundColor Green
}
