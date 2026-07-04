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
# By default the servers bind to 0.0.0.0 so the UI + API are reachable from other
# devices on the local network (phone, laptop) at http://<this-machine-LAN-IP>:5173.
# The backend has NO auth and CORS is wildcard -- only expose it on a trusted LAN.
# Pass -Loopback to bind 127.0.0.1 only (old behaviour, no firewall prompt).
#
# TROUBLESHOOTING -- "firewall allowed but phone still can't reach it":
#   The usual cause is the Wi-Fi network being classified as PUBLIC. Windows
#   Firewall blocks inbound on Public, and the go run temp exe gets a NEW path on
#   every compile so a program-based allow rule goes stale. Fix (elevated shell):
#     Set-NetConnectionProfile -InterfaceAlias 'Wi-Fi' -NetworkCategory Private
#     New-NetFirewallRule -DisplayName 'SwarmGo Dev 5173' -Direction Inbound -LocalPort 5173 -Protocol TCP -Action Allow -Profile Private
#     New-NetFirewallRule -DisplayName 'SwarmGo Dev 8090' -Direction Inbound -LocalPort 8090 -Protocol TCP -Action Allow -Profile Private
#   Port-based rules survive recompiles; the phone hits Vite (:5173, node), which
#   proxies /api to the backend (:8090).
#
#   If the PC self-test (Invoke-WebRequest to the LAN IP) returns 200 but the phone
#   STILL fails, the PC side is fine -- the router is the problem. Diagnose from the
#   phone with adb: `adb shell ping -c3 <PC-LAN-IP>`. "Destination Host Unreachable"
#   while the gateway pings fine == router AP/client isolation (peers can't ARP each
#   other). Fix: disable "AP Isolation"/"Client Isolation" on the router, OR bypass
#   it entirely with Tailscale (browse http://<PC-tailscale-100.x.y.z>:5173 -- Vite
#   binds 0.0.0.0 so it also listens on the Tailscale interface).
#
# Usage:  .\scripts\dev.ps1                 # backend + frontend on LAN, open browser at LAN IP
#         .\scripts\dev.ps1 -Loopback       # bind 127.0.0.1 only (local-only, no network access)
#         .\scripts\dev.ps1 -NoBrowser      # don't auto-open the browser
#         .\scripts\dev.ps1 -BackendOnly    # only the Go backend
#         .\scripts\dev.ps1 -FrontendOnly   # only the Vite dev server
#         .\scripts\dev.ps1 -Port 8091      # override backend port
#         .\scripts\dev.ps1 -NoKillPort     # don't kill an orphan on the port, abort instead
#
# Pre-flight: before binding, a leftover listener on the backend port (8090) or
# the Vite port (5173) -- typically a go/node child orphaned when a prior run was
# killed from outside its finally block -- is detected and killed so the port is
# reusable. Pass -NoKillPort to abort with a message instead of killing it.

param(
    [int]$Port = 8090,
    [switch]$Loopback,
    [switch]$NoBrowser,
    [switch]$BackendOnly,
    [switch]$FrontendOnly,
    [switch]$NoKillPort
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

# Get-LanIP returns the primary IPv4 address of the adapter that owns the default
# route (the one other LAN devices can reach). Falls back to 127.0.0.1 if none is
# found (e.g. offline). Used for the bind address hint and the browser URL.
function Get-LanIP {
    try {
        $ip = Get-NetIPConfiguration |
            Where-Object { $_.IPv4DefaultGateway -ne $null -and $_.NetAdapter.Status -eq "Up" } |
            ForEach-Object { $_.IPv4Address.IPAddress } |
            Select-Object -First 1
        if ($ip) { return $ip }
    } catch { }
    return "127.0.0.1"
}

# Bind host: 0.0.0.0 exposes the servers on every interface (LAN-reachable); with
# -Loopback we keep the old 127.0.0.1-only behaviour (no Windows Firewall prompt).
$bindHost = if ($Loopback) { "127.0.0.1" } else { "0.0.0.0" }
$lanIP = if ($Loopback) { "127.0.0.1" } else { Get-LanIP }

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

# Free-Port clears a leftover listener on $pt before we try to bind it. A prior
# run killed from outside (window closed, task ended) can orphan the go/node
# child still holding the port; without this the new backend dies on bind and
# the script collapses with a generic "child exited" message. We identify the
# owning PID, report what it is, and (unless -NoKillPort) kill its tree so the
# port is reusable. With -NoKillPort we abort early with a clear message instead.
function Free-Port($pt, $label) {
    $conns = Get-NetTCPConnection -LocalPort $pt -State Listen -ErrorAction SilentlyContinue
    if (-not $conns) { return }
    $owners = $conns | Select-Object -ExpandProperty OwningProcess -Unique
    foreach ($procId in $owners) {
        $proc = Get-Process -Id $procId -ErrorAction SilentlyContinue
        $name = if ($proc) { $proc.ProcessName } else { "bilinmeyen" }
        if ($NoKillPort) {
            Write-Host "==> HATA: $label portu $pt dolu (PID $procId / $name). -NoKillPort acik, cikiliyor." -ForegroundColor Red
            throw "port-busy"
        }
        Write-Host "==> $label portu $pt dolu (PID $procId / $name) -> orphan temizleniyor..." -ForegroundColor Yellow
        taskkill /PID $procId /T /F 2>$null | Out-Null
    }
    # Give the OS a moment to release the socket before we rebind.
    Start-Sleep -Milliseconds 400
}

try {
    if (-not $FrontendOnly) {
        # Pre-flight: clear any orphan still holding the backend port.
        Free-Port $Port "Backend"
        Write-Host "==> Backend baslatiliyor: go run ./cmd/swarmgo  (${bindHost}:$Port)" -ForegroundColor Cyan
        $env:SWARMGO_ADDR = "${bindHost}:$Port"
        # Gated features (see SKILL.md / Ortam Notlari): shell + self-management.
        $env:SWARMGO_ENABLE_SHELL = "1"
        $env:SWARMGO_ENABLE_SELFMANAGE = "1"
        $backend = Start-Process -FilePath "go" -ArgumentList "run", "./cmd/swarmgo" `
            -WorkingDirectory $root -NoNewWindow -PassThru
        $procs += $backend
    }

    # Wait for the backend to actually serve before starting Vite. `go run` may
    # spend ~10-20s COMPILING on a cold cache; during that the port does not exist
    # yet, so starting Vite now would spam "ECONNREFUSED 127.0.0.1:8090" until the
    # binary finally launches. Polling /health first makes Vite start clean.
    if (-not $FrontendOnly -and -not $BackendOnly) {
        Write-Host "==> Backend derleniyor/hazirlaniyor, /health bekleniyor (ilk derleme uzun surebilir)..." -ForegroundColor Cyan
        $healthUrl = "http://127.0.0.1:$Port/health"
        $deadline = (Get-Date).AddSeconds(120)
        $ready = $false
        while ((Get-Date) -lt $deadline) {
            if ($backend.HasExited) { throw "backend saglikli olmadan sonlandi" }
            try {
                $r = Invoke-WebRequest -Uri $healthUrl -UseBasicParsing -TimeoutSec 2
                if ($r.StatusCode -eq 200) { $ready = $true; break }
            } catch { }
            Start-Sleep -Milliseconds 500
        }
        if ($ready) {
            Write-Host "==> Backend hazir." -ForegroundColor Green
        } else {
            Write-Host "==> Backend zaman asimina ugradi; frontend yine de baslatiliyor." -ForegroundColor Yellow
        }
    }

    if (-not $BackendOnly) {
        # Pre-flight: clear any orphan still holding the Vite dev port (5173).
        Free-Port 5173 "Frontend"
        $fe = Join-Path $root "frontend"
        if (-not (Test-Path (Join-Path $fe "node_modules"))) {
            Write-Host "==> node_modules yok, npm install calisiyor..." -ForegroundColor Yellow
            Push-Location $fe
            npm install
            Pop-Location
        }
        # In network mode pass --host 0.0.0.0 so Vite listens on every interface
        # (default is localhost-only). "--" forwards the flag through npm to vite.
        $viteArgs = @("run", "dev")
        if (-not $Loopback) { $viteArgs += @("--", "--host", "0.0.0.0") }
        Write-Host "==> Frontend baslatiliyor: npm run dev  (http://${lanIP}:5173)" -ForegroundColor Cyan
        # npm.cmd: on Windows npm is a batch shim; call the .cmd directly.
        $frontend = Start-Process -FilePath "npm.cmd" -ArgumentList $viteArgs `
            -WorkingDirectory $fe -NoNewWindow -PassThru
        $procs += $frontend
    }

    if (-not $NoBrowser -and -not $BackendOnly) {
        # Give Vite a moment to come up, then open the browser at the LAN IP so the
        # same URL works from other devices too.
        Start-Sleep -Seconds 3
        Start-Process "http://${lanIP}:5173"
    }

    Write-Host ""
    Write-Host "==> Calisiyor. Durdurmak icin Ctrl+C (ya da pencereyi kapat) -> ikisi de kapanir." -ForegroundColor Green
    if (-not $Loopback) {
        Write-Host "==> UI (bu makine):   http://127.0.0.1:5173" -ForegroundColor Green
        Write-Host "==> UI (yerel agdan): http://${lanIP}:5173  <- telefon/diger cihazlar" -ForegroundColor Green
        Write-Host "==> API auth YOK, CORS wildcard -> yalnizca guvenilir agda ac." -ForegroundColor Yellow
        Write-Host "==> Ilk calistirmada Windows Guvenlik Duvari izin sorabilir (Ozel ag icin izin ver)." -ForegroundColor Yellow
    } else {
        Write-Host "==> UI: http://127.0.0.1:5173  (yalnizca bu makine / -Loopback)" -ForegroundColor Green
    }
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
