# TionHarness - serve over your Tailscale network with automatic HTTPS.
#
# Runs like dev.ps1: a FOREGROUND long-running process. Backend output streams to
# this terminal; pressing Ctrl+C (or the backend exiting on its own) tears the
# process tree down and, unless -KeepServe, removes the Tailscale serve config,
# printing a clear "stopped" line so you always see it end from the terminal.
#
# WHY HTTPS: browsers only allow the microphone (getUserMedia / Web Speech) in a
# "secure context". Tailscale Serve gives your <machine>.<tailnet>.ts.net name a
# real Let's Encrypt certificate and reverse-proxies it to the local port, so
# phones on the tailnet reach TionHarness over https:// and the mic works. Traffic
# stays INSIDE the tailnet (this is `serve`, NOT `funnel`) -> the no-auth backend
# is never public.
#
# PREREQUISITE (one-time, in a browser): enable "HTTPS Certificates" in the
# tailnet admin console: https://login.tailscale.com/admin/dns
#
# NOTE: keep this file ASCII-only (Windows PowerShell 5.1 mis-decodes UTF-8).
#
# Usage:  .\scripts\tailscale-serve.ps1                 # build if needed, run, serve
#         .\scripts\tailscale-serve.ps1 -Build          # force a rebuild first
#         .\scripts\tailscale-serve.ps1 -Port 5174      # override the loopback port
#         .\scripts\tailscale-serve.ps1 -KeepServe      # leave serve config on exit
#         .\scripts\tailscale-serve.ps1 -Reset          # just tear down the serve config
#         .\scripts\tailscale-serve.ps1 -NoKillPort     # abort if the port is busy

param(
    # Loopback port for the single binary; Tailscale Serve proxies https:// to it.
    # 5174 avoids clashes with dev.ps1 (Vite 5173 / backend 8090) and unity-mcp (8080).
    [int]$Port = 5174,
    [switch]$Build,
    [switch]$KeepServe,
    [switch]$Reset,
    [switch]$NoKillPort
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$tailscale = "C:\Program Files\Tailscale\tailscale.exe"
if (-not (Test-Path $tailscale)) { $tailscale = "tailscale" }
$exe = Join-Path $root "bin\tionharness.exe"

if ($Reset) {
    & $tailscale serve reset
    Write-Host "==> Tailscale serve config sifirlandi." -ForegroundColor Green
    exit 0
}

# Stop-Tree kills a process AND its children (taskkill /T), so no orphan backend
# is left holding the port after an outside kill.
function Stop-Tree($p) {
    if ($null -eq $p) { return }
    try {
        if (-not $p.HasExited) { taskkill /PID $p.Id /T /F 2>$null | Out-Null }
    } catch { }
}

# Free-Port clears a leftover listener on the port (e.g. a detached backend from a
# previous run) so the new one can bind. -NoKillPort aborts instead.
function Free-Port($pt, $label) {
    $conns = Get-NetTCPConnection -LocalPort $pt -State Listen -ErrorAction SilentlyContinue
    if (-not $conns) { return }
    $owners = $conns | Select-Object -ExpandProperty OwningProcess -Unique
    foreach ($procId in $owners) {
        $proc = Get-Process -Id $procId -ErrorAction SilentlyContinue
        $pname = if ($proc) { $proc.ProcessName } else { "bilinmeyen" }
        if ($NoKillPort) {
            Write-Host "==> HATA: port $pt dolu (PID $procId / $pname). -NoKillPort acik, cikiliyor." -ForegroundColor Red
            throw "port-busy"
        }
        Write-Host "==> $label portu $pt dolu (PID $procId / $pname) -> temizleniyor..." -ForegroundColor Yellow
        taskkill /PID $procId /T /F 2>$null | Out-Null
    }
    Start-Sleep -Milliseconds 400
}

# Tailnet identity + HTTPS-cert readiness (empty CertDomains => not enabled yet).
$j = & $tailscale status --json | ConvertFrom-Json
$name = ($j.Self.DNSName).TrimEnd('.')
if (-not $j.CertDomains -or @($j.CertDomains).Count -eq 0) {
    Write-Host "==> UYARI: Bu tailnet'te HTTPS Sertifikalari KAPALI." -ForegroundColor Yellow
    Write-Host "    Once ac: https://login.tailscale.com/admin/dns (HTTPS Certificates)" -ForegroundColor Yellow
    Write-Host "    (Acik degilse serve sertifika alamaz.)" -ForegroundColor Yellow
    Write-Host ""
}

# Build the single binary if missing or when -Build is passed.
if ($Build -or -not (Test-Path $exe)) {
    Write-Host "==> Binary derleniyor: go build -o bin\tionharness.exe ./cmd/tionharness" -ForegroundColor Cyan
    & go build -o $exe ./cmd/tionharness
    if ($LASTEXITCODE -ne 0) { throw "go build basarisiz" }
}

$backend = $null
try {
    Free-Port $Port "Backend"

    Write-Host "==> Backend baslatiliyor: bin\tionharness.exe  (127.0.0.1:$Port)" -ForegroundColor Cyan
    $env:TIONHARNESS_ADDR = "127.0.0.1:$Port"
    # Match dev.ps1: enable the built-in shell tool (agents run commands).
    $env:TIONHARNESS_ENABLE_SHELL = "1"
    $backend = Start-Process -FilePath $exe -NoNewWindow -PassThru
    Start-Sleep -Milliseconds 300
    if ($backend.HasExited) { throw "backend aninda sonlandi (exit $($backend.ExitCode))" }

    # Wait for /health before advertising via serve (workspace open can take ~10s).
    Write-Host "==> /health bekleniyor..." -ForegroundColor Cyan
    $healthUrl = "http://127.0.0.1:$Port/health"
    $deadline = (Get-Date).AddSeconds(120)
    $ready = $false
    while ((Get-Date) -lt $deadline) {
        if ($backend.HasExited) { throw "backend saglikli olmadan sonlandi (exit $($backend.ExitCode))" }
        try {
            $r = Invoke-WebRequest -Uri $healthUrl -UseBasicParsing -TimeoutSec 2
            if ($r.StatusCode -eq 200) { $ready = $true; break }
        } catch { }
        Start-Sleep -Milliseconds 500
    }
    if ($ready) { Write-Host "==> Backend hazir." -ForegroundColor Green }
    else { Write-Host "==> /health zaman asimi; yine de serve kuruluyor." -ForegroundColor Yellow }

    # Point the tailnet HTTPS front door at the local port.
    Write-Host "==> Tailscale serve ayarlaniyor -> 127.0.0.1:$Port" -ForegroundColor Cyan
    & $tailscale serve --bg $Port | Out-Null

    Write-Host ""
    Write-Host "==> Calisiyor. Durdurmak icin Ctrl+C." -ForegroundColor Green
    Write-Host "==> Bu makine:        http://localhost:$Port" -ForegroundColor Green
    Write-Host "==> Tailnet (telefon): https://$name/  (Tailscale acik)" -ForegroundColor Green
    Write-Host "==> serve = tailnet-only (public DEGIL)." -ForegroundColor Yellow
    Write-Host ""

    # Block in the foreground until the backend exits or Ctrl+C interrupts us.
    while ($true) {
        Start-Sleep -Milliseconds 500
        if ($backend.HasExited) {
            Write-Host "==> Backend kendi kendine sonlandi (exit $($backend.ExitCode))." -ForegroundColor Yellow
            break
        }
    }
}
finally {
    Write-Host "==> Temizlik: backend sonlandiriliyor..." -ForegroundColor Cyan
    Stop-Tree $backend
    if (-not $KeepServe) {
        try { & $tailscale serve reset } catch { }
        Write-Host "==> Tailscale serve config kaldirildi (-KeepServe ile birakabilirsin)." -ForegroundColor Cyan
    }
    Write-Host "==> Kapandi." -ForegroundColor Green
}
