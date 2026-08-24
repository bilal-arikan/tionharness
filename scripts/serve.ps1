# serve.ps1 -- Clean build + run of the single binary (backend only, no Vite).
#
# Why not just `go run`? `go run ./cmd/tionharness` can reuse a STALE cached link
# artifact and launch a binary that PREDATES your latest source edits (observed
# 2026-07-12: a fix compiled fine via `go build` but `go run` kept serving an old
# exe). This script does an explicit `go build -o bin\tionharness.exe` -- which
# forces a fresh link that always picks up current source -- then runs THAT binary
# directly, so what you run is guaranteed current.
#
# Serves the embedded SPA (single binary) on http://<host>:<Port>. Use dev.ps1
# instead when you want the Vite dev server (:5173) + hot reload.
#
# Usage:
#   .\scripts\serve.ps1                 # 0.0.0.0:8090 (LAN-reachable)
#   .\scripts\serve.ps1 -Loopback       # 127.0.0.1 only
#   .\scripts\serve.ps1 -Port 8091
#   .\scripts\serve.ps1 -Clean          # also wipe the Go build cache first (paranoid)
#   .\scripts\serve.ps1 -NoKillPort     # abort instead of killing a port squatter
#
# ASCII-only on purpose (WinPS 5.1 mis-decodes a BOM-less UTF-8 script -> parse breaks).

param(
    [int]$Port = 8090,
    [switch]$Loopback,
    [switch]$Clean,
    [switch]$NoKillPort
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$bindHost = if ($Loopback) { "127.0.0.1" } else { "0.0.0.0" }
$bin = Join-Path $root "bin\tionharness.exe"

# Free-Port clears a leftover listener on $pt (an orphaned prior run holding the
# port) so the new backend can bind. With -NoKillPort we abort with a clear message.
function Free-Port($pt, $label) {
    $conns = Get-NetTCPConnection -LocalPort $pt -State Listen -ErrorAction SilentlyContinue
    if (-not $conns) { return }
    $owners = $conns | Select-Object -ExpandProperty OwningProcess -Unique
    foreach ($procId in $owners) {
        $proc = Get-Process -Id $procId -ErrorAction SilentlyContinue
        $name = if ($proc) { $proc.ProcessName } else { "unknown" }
        if ($NoKillPort) {
            Write-Host "==> ERROR: $label port $pt busy (PID $procId / $name). -NoKillPort set, aborting." -ForegroundColor Red
            throw "port-busy"
        }
        Write-Host "==> $label port $pt busy (PID $procId / $name) -> killing orphan..." -ForegroundColor Yellow
        taskkill /PID $procId /T /F 2>$null | Out-Null
    }
    Start-Sleep -Milliseconds 400
}

# 1) Clean build (explicit link -> always current source).
if ($Clean) {
    Write-Host "==> go clean -cache" -ForegroundColor Yellow
    & go clean -cache
}
New-Item -ItemType Directory -Force -Path (Join-Path $root "bin") | Out-Null
Write-Host "==> Building: go build -o bin\tionharness.exe ./cmd/tionharness" -ForegroundColor Cyan
& go build -o $bin ./cmd/tionharness
if ($LASTEXITCODE -ne 0) { throw "build failed (exit $LASTEXITCODE)" }
$mb = [math]::Round((Get-Item $bin).Length / 1MB, 1)
Write-Host "==> Build OK: $bin ($mb MB)" -ForegroundColor Green

# 2) Free the port and set env (gated feature matches dev.ps1).
Free-Port $Port "Backend"
$env:TIONHARNESS_ADDR = "${bindHost}:$Port"
$env:TIONHARNESS_ENABLE_SHELL = "1"

# 3) Run in the foreground -- Ctrl+C stops it (single binary, no children to reap).
Write-Host "==> Running: $bin  (${bindHost}:$Port)  -- Ctrl+C to stop" -ForegroundColor Cyan
& $bin
