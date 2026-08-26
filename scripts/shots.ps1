# shots.ps1 -- capture product screenshots for the promo site (_Docs\72).
#
# ASCII-only on purpose: WinPS 5.1 decodes BOM-less UTF-8 as ANSI and mangles
# the parse. Same rule as dev.ps1.
#
# Requires a RUNNING TionHarness instance; this script does not start one.
# Start it first with .\scripts\dev.ps1 (5173) or .\tionharness.exe (8095).
#
#   .\scripts\shots.ps1
#   .\scripts\shots.ps1 -BaseUrl http://127.0.0.1:8095 -Workspace WS1

[CmdletBinding()]
param(
    [string] $BaseUrl = 'http://127.0.0.1:5173',
    [string] $Workspace = '',
    # Installs playwright + the Chromium build if they are missing.
    [switch] $InstallDeps
)

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$siteDir = Join-Path $repoRoot 'website'
$script = Join-Path $siteDir 'scripts\shots.mjs'

if (-not (Test-Path $script)) {
    throw "shots.mjs not found at $script"
}

# 1. The app has to be up: the script drives real hash routes, not fixtures.
Write-Host "Checking $BaseUrl ..." -ForegroundColor Cyan
try {
    Invoke-WebRequest -Uri "$BaseUrl/api/workspaces" -UseBasicParsing -TimeoutSec 5 | Out-Null
}
catch {
    throw "Cannot reach $BaseUrl/api/workspaces. Start the app first (.\scripts\dev.ps1)."
}

# 2. Playwright is a heavy, optional dependency -- it is not in package.json so a
#    plain `npm install` for the site stays small. Install it on demand.
$playwrightDir = Join-Path $siteDir 'node_modules\playwright'
if (-not (Test-Path $playwrightDir)) {
    if (-not $InstallDeps) {
        throw "playwright is not installed. Re-run with -InstallDeps (downloads a Chromium build, ~150 MB)."
    }
    Write-Host 'Installing playwright ...' -ForegroundColor Cyan
    Push-Location $siteDir
    try {
        npm install -D playwright
        if ($LASTEXITCODE -ne 0) { throw "npm install -D playwright failed ($LASTEXITCODE)" }
        npx playwright install chromium
        if ($LASTEXITCODE -ne 0) { throw "playwright install chromium failed ($LASTEXITCODE)" }
    }
    finally {
        Pop-Location
    }
}

# 3. Shoot.
$nodeArgs = @($script, '--base', $BaseUrl)
if ($Workspace -ne '') { $nodeArgs += @('--workspace', $Workspace) }

Push-Location $siteDir
try {
    node @nodeArgs
    if ($LASTEXITCODE -ne 0) { throw "shots.mjs failed ($LASTEXITCODE)" }
}
finally {
    Pop-Location
}

Write-Host 'Done. Rebuild the site to pick the new shots up: cd website; npm run build' -ForegroundColor Green
