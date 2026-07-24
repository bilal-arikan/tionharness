# Serve TionSwarm over your Tailscale network with automatic HTTPS.
#
# WHY: browsers only allow the microphone (getUserMedia / Web Speech) in a
# "secure context" (HTTPS or localhost). Tailscale Serve provisions a real
# Let's Encrypt certificate for your <machine>.<tailnet>.ts.net name and
# reverse-proxies it to the local TionSwarm port, so phones on the tailnet reach
# TionSwarm over https:// and the mic works. Traffic stays INSIDE the tailnet
# (this is `serve`, NOT `funnel`) -> the no-auth backend is never public.
#
# PREREQUISITE (one-time, in a browser):
#   Enable "HTTPS Certificates" in the tailnet admin console:
#   https://login.tailscale.com/admin/dns
#   (MagicDNS is already on if machine names resolve.)
#
# USAGE:
#   .\scripts\tailscale-serve.ps1                 # build+run backend, then serve
#   .\scripts\tailscale-serve.ps1 -NoBackend      # only (re)configure serve
#   .\scripts\tailscale-serve.ps1 -Reset          # tear down the serve config
#   .\scripts\tailscale-serve.ps1 -Port 8080

param(
  [int]$Port = 8080,
  [switch]$NoBackend,
  [switch]$Reset
)

$ErrorActionPreference = 'Stop'
$tailscale = 'C:\Program Files\Tailscale\tailscale.exe'
if (-not (Test-Path $tailscale)) { $tailscale = 'tailscale' }

if ($Reset) {
  & $tailscale serve reset
  Write-Host 'Tailscale serve config reset.'
  exit 0
}

# Report whether HTTPS certs are enabled (empty CertDomains => not enabled yet).
$j = & $tailscale status --json | ConvertFrom-Json
$name = ($j.Self.DNSName).TrimEnd('.')
if (-not $j.CertDomains -or @($j.CertDomains).Count -eq 0) {
  Write-Host 'WARNING: HTTPS Certificates are NOT enabled for this tailnet.' -ForegroundColor Yellow
  Write-Host '         Enable it first at https://login.tailscale.com/admin/dns'
  Write-Host '         (serve will fail to fetch a certificate until then).'
  Write-Host ''
}

# Start the single-binary backend on loopback unless told not to.
if (-not $NoBackend) {
  $root = Split-Path -Parent $PSScriptRoot
  $exe = Join-Path $root 'bin\tionswarm.exe'
  if (-not (Test-Path $exe)) {
    Write-Host "Building backend ($exe) ..."
    Push-Location $root
    & go build -o $exe ./cmd/tionswarm
    Pop-Location
  }
  # Bind to loopback only; Tailscale Serve is the sole front door.
  $env:TIONSWARM_ADDR = "127.0.0.1:$Port"
  Write-Host "Starting TionSwarm on $env:TIONSWARM_ADDR ..."
  Start-Process -FilePath $exe -WindowStyle Hidden
  Start-Sleep -Seconds 3
}

# Map the tailnet HTTPS front door -> local TionSwarm port.
& $tailscale serve --bg $Port
Write-Host ''
& $tailscale serve status
Write-Host ''
Write-Host "Open from any device on the tailnet (Tailscale on):" -ForegroundColor Green
Write-Host "  https://$name/"
