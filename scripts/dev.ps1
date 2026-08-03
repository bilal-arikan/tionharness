# TionSwarm - development run (backend + frontend together).
#
# Starts the Go backend (go run ./cmd/tionswarm on 127.0.0.1:8090) and the Vite
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
#     New-NetFirewallRule -DisplayName 'TionSwarm Dev 5173' -Direction Inbound -LocalPort 5173 -Protocol TCP -Action Allow -Profile Private
#     New-NetFirewallRule -DisplayName 'TionSwarm Dev 8090' -Direction Inbound -LocalPort 8090 -Protocol TCP -Action Allow -Profile Private
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
# Diagnostics: _devlogs\lifecycle.log records every launch/kill across runs, and
# _devlogs\backend-stderr-<stamp>.log captures the backend's STDERR (Go runtime
# fatals / panic traces; STDOUT stays live on the console). Empty captures are
# deleted on exit, so a surviving stderr file always means something went wrong.
# If the backend disappears and lifecycle.log shows NO cleanup entry for it, the
# kill came from outside this script (external taskkill, window force-close).
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

# Dev-run diagnostics (_devlogs/, gitignored via *.log).
#
# WHY: the Go backend writes slog to STDOUT (mirrored to ~/.tionswarm/logs), but
# Go runtime fatals -- "fatal error: out of memory", panic traces -- go to STDERR,
# which used to be unredirected and therefore died with the console window. On
# 2026-08-01 a backend death at 05:26:55 left NO evidence anywhere: no graceful
# "shutting down" log line, no Windows WER report, no crash dump. It could not be
# decided whether the process was force-killed from outside or hit a runtime
# fatal. Two captures close that gap:
#   * backend-stderr-<stamp>.log -- the crash text itself.
#   * lifecycle.log              -- who killed what, appended across runs. A
#     taskkill /T /F leaves no trace in the app log, so WITHOUT this line an
#     external kill and our own cleanup look identical afterwards. If the backend
#     vanishes and lifecycle.log has no "cleanup" entry, the kill came from
#     outside this script.
# Only STDERR is redirected: STDOUT stays on the console so the live log is
# unchanged. The frontend is deliberately left alone (Vite reports build/TS
# errors interactively; capturing it would hide them).
$logDir = Join-Path $root "_devlogs"
if (-not (Test-Path $logDir)) { New-Item -ItemType Directory -Path $logDir | Out-Null }
$runStamp = Get-Date -Format "yyyyMMdd-HHmmss"
$backendErrLog = Join-Path $logDir "backend-stderr-$runStamp.log"
$lifecycleLog = Join-Path $logDir "lifecycle.log"

function Write-Lifecycle($msg) {
    $line = "{0} {1}" -f (Get-Date -Format "yyyy-MM-ddTHH:mm:ss.fffzzz"), $msg
    try { Add-Content -Path $lifecycleLog -Value $line -Encoding UTF8 } catch { }
}

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
# children: go->compiled exe, npm->node) down on exit. Each entry carries the
# label + stderr capture path so cleanup can report which one died.
$procs = @()

function Add-Proc($proc, $label, $errLog) {
    # Touching .Handle keeps the process handle open, which is what makes
    # .ExitCode readable after the process dies (a Start-Process -PassThru object
    # otherwise reports $null). The exit code is a real diagnostic signal here:
    # a Go runtime fatal exits 2, an outside taskkill /F exits 1.
    try { $null = $proc.Handle } catch { }
    $script:procs += [pscustomobject]@{ Proc = $proc; Label = $label; Err = $errLog }
}

function Get-ExitCodeSafe($p) {
    # ExitCode throws while the process is still alive, and a Start-Process
    # -PassThru object can also hand back $null once it has exited (the handle is
    # not always retained). Both mean "unknown" -- never print a blank code.
    try {
        $c = $p.ExitCode
        if ($null -eq $c) { return "?" }
        return $c
    } catch { return "?" }
}

function Stop-Tree($p, $label) {
    if ($null -eq $p) { return }
    try {
        if ($p.HasExited) {
            Write-Lifecycle "$label already exited on its own (pid=$($p.Id) exit=$(Get-ExitCodeSafe $p)) -- not killed by dev.ps1"
            return
        }
        # /T kills the whole tree (go run's child binary, npm's node), /F forces it.
        # Logged BEFORE the kill so the record survives even if taskkill takes us
        # down mid-teardown -- this is the line that proves the death was ours.
        Write-Lifecycle "dev.ps1 cleanup: force-killing tree $label (pid=$($p.Id), taskkill /T /F)"
        taskkill /PID $p.Id /T /F 2>$null | Out-Null
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
        Write-Lifecycle "dev.ps1 pre-flight: force-killing orphan on port $pt (pid=$procId name=$name, taskkill /T /F)"
        taskkill /PID $procId /T /F 2>$null | Out-Null
    }
    # Give the OS a moment to release the socket before we rebind.
    Start-Sleep -Milliseconds 400
}

try {
    Write-Lifecycle "dev.ps1 start (bind=${bindHost}:$Port backendOnly=$BackendOnly frontendOnly=$FrontendOnly)"
    if (-not $FrontendOnly) {
        # Pre-flight: clear any orphan still holding the backend port.
        Free-Port $Port "Backend"
        Write-Host "==> Backend baslatiliyor: go run ./cmd/tionswarm  (${bindHost}:$Port)" -ForegroundColor Cyan
        $env:TIONSWARM_ADDR = "${bindHost}:$Port"
        # Gated feature (see SKILL.md / Ortam Notlari): the built-in shell.
        # (Self-management is ALWAYS installed since 2026-07-01 -- no env gate.)
        $env:TIONSWARM_ENABLE_SHELL = "1"
        # -RedirectStandardError: keeps Go runtime fatals (OOM, panic traces) on
        # disk. STDOUT is intentionally NOT redirected -- slog keeps streaming to
        # this console live.
        $backend = Start-Process -FilePath "go" -ArgumentList "run", "./cmd/tionswarm" `
            -WorkingDirectory $root -NoNewWindow -PassThru `
            -RedirectStandardError $backendErrLog
        Add-Proc $backend "backend" $backendErrLog
        Write-Lifecycle "backend launched (pid=$($backend.Id) stderr=$backendErrLog)"
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
        Add-Proc $frontend "frontend" $null
        Write-Lifecycle "frontend launched (pid=$($frontend.Id))"
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
        foreach ($e in $procs) {
            if ($e.Proc.HasExited) {
                $code = Get-ExitCodeSafe $e.Proc
                Write-Host "==> $($e.Label) sonlandi (PID $($e.Proc.Id), exit $code), digerleri kapatiliyor..." -ForegroundColor Yellow
                Write-Lifecycle "$($e.Label) exited on its own (pid=$($e.Proc.Id) exit=$code)"
                throw "child-exited"
            }
        }
    }
}
finally {
    Write-Host "==> Temizlik: surecler sonlandiriliyor..." -ForegroundColor Cyan
    Write-Lifecycle "dev.ps1 cleanup started (Ctrl+C, window close or child exit)"
    foreach ($e in $procs) { Stop-Tree $e.Proc $e.Label }

    # Surface captured stderr right here: a runtime fatal is worthless if nobody
    # reads it. An EMPTY capture is deleted, so a surviving file always means the
    # process wrote something to stderr.
    foreach ($e in $procs) {
        if (-not $e.Err) { continue }
        if (-not (Test-Path $e.Err)) { continue }
        $len = (Get-Item $e.Err).Length
        if ($len -eq 0) { Remove-Item $e.Err -Force -ErrorAction SilentlyContinue; continue }
        Write-Lifecycle "$($e.Label) stderr captured: $($e.Err) ($len bytes)"
        Write-Host ""
        Write-Host "==> $($e.Label) STDERR yakalandi ($len bayt): $($e.Err)" -ForegroundColor Red
        Get-Content $e.Err -Tail 40 | ForEach-Object { Write-Host "    $_" -ForegroundColor DarkYellow }
        Write-Host ""
    }

    # Keep only the 10 newest captures so _devlogs does not grow without bound.
    Get-ChildItem $logDir -Filter "backend-stderr-*.log" -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTime -Descending |
        Select-Object -Skip 10 |
        Remove-Item -Force -ErrorAction SilentlyContinue

    Write-Lifecycle "dev.ps1 cleanup finished"
    Write-Host "==> Kapandi." -ForegroundColor Green
}
