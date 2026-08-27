<#
.SYNOPSIS
  TionHarness installer for Windows.

.DESCRIPTION
  Reads latest.json from the release feed, downloads the artifact that matches
  this machine, verifies its sha256 and installs the binary.

.PARAMETER FeedUrl
  Release feed base URL. Defaults to $env:TIONHARNESS_FEED_URL when set,
  otherwise https://tionharness.com

.EXAMPLE
  powershell -NoProfile -File scripts/install.ps1
#>
[CmdletBinding()]
param(
    [string]$FeedUrl
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

function Fail([string]$Message) {
    [Console]::Error.WriteLine("install: $Message")
    exit 1
}

if (-not $FeedUrl) {
    if ($env:TIONHARNESS_FEED_URL) { $FeedUrl = $env:TIONHARNESS_FEED_URL }
    else { $FeedUrl = 'https://tionharness.com' }
}
$FeedUrl = $FeedUrl.TrimEnd('/')

# --- platform detection -------------------------------------------------------

if ($IsLinux) { $hostOs = 'linux' }
elseif ($IsMacOS) { $hostOs = 'darwin' }
else { $hostOs = 'windows' }

switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture) {
    'X64' { $hostArch = 'amd64' }
    'Arm64' { $hostArch = 'arm64' }
    default { Fail "unsupported architecture: $([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture)" }
}

$binaryName = if ($hostOs -eq 'windows') { 'tionharness.exe' } else { 'tionharness' }

# --- temp workspace -----------------------------------------------------------

$workDir = Join-Path ([System.IO.Path]::GetTempPath()) ("tionharness-install-" + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $workDir -Force | Out-Null

try {
    # --- feed -----------------------------------------------------------------

    $feedFile = Join-Path $workDir 'latest.json'
    Write-Host "Fetching $FeedUrl/latest.json"
    try {
        Invoke-WebRequest -Uri "$FeedUrl/latest.json" -OutFile $feedFile -UseBasicParsing
    } catch {
        Fail "failed to download $FeedUrl/latest.json : $($_.Exception.Message)"
    }

    try {
        $feed = Get-Content -Raw -LiteralPath $feedFile | ConvertFrom-Json
    } catch {
        Fail "malformed feed at $FeedUrl/latest.json : $($_.Exception.Message)"
    }

    if (-not $feed.version) { Fail "malformed feed: no version field in $FeedUrl/latest.json" }
    $version = $feed.version

    $artifact = @($feed.artifacts | Where-Object { $_.os -eq $hostOs -and $_.arch -eq $hostArch }) | Select-Object -First 1
    if (-not $artifact) {
        Fail "no release artifact for $hostOs/$hostArch in $FeedUrl/latest.json (version $version)"
    }
    foreach ($field in 'file', 'url', 'sha256') {
        if (-not $artifact.$field) { Fail "malformed artifact entry for $hostOs/$hostArch : no $field field" }
    }

    # --- download and verify --------------------------------------------------

    $archive = Join-Path $workDir $artifact.file
    Write-Host "Downloading $($artifact.url)"
    try {
        Invoke-WebRequest -Uri $artifact.url -OutFile $archive -UseBasicParsing
    } catch {
        Fail "failed to download $($artifact.url) : $($_.Exception.Message)"
    }

    $actual = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    $expected = ([string]$artifact.sha256).ToLowerInvariant()
    if ($actual -ne $expected) {
        Remove-Item -LiteralPath $archive -Force -ErrorAction SilentlyContinue
        Write-Host "install: checksum mismatch for $($artifact.file)"
        Write-Host "install:   expected $expected"
        Write-Host "install:   actual   $actual"
        Fail 'checksum verification failed; the download was deleted and nothing was installed'
    }
    Write-Host "Checksum OK ($expected)"

    # --- extract --------------------------------------------------------------

    $extractDir = Join-Path $workDir 'extract'
    New-Item -ItemType Directory -Path $extractDir -Force | Out-Null

    if ($artifact.file -like '*.zip') {
        Expand-Archive -LiteralPath $archive -DestinationPath $extractDir -Force
    } elseif ($artifact.file -like '*.tar.gz' -or $artifact.file -like '*.tgz') {
        if (-not (Get-Command tar -ErrorAction SilentlyContinue)) {
            Fail "required tool not found: tar (needed to extract $($artifact.file))"
        }
        & tar -xzf $archive -C $extractDir
        if ($LASTEXITCODE -ne 0) { Fail "failed to extract $($artifact.file)" }
    } else {
        Fail "unsupported archive format: $($artifact.file)"
    }

    $extracted = Join-Path $extractDir $binaryName
    if (-not (Test-Path -LiteralPath $extracted)) {
        Fail "archive $($artifact.file) does not contain $binaryName"
    }

    # --- install --------------------------------------------------------------

    if ($env:TIONHARNESS_INSTALL_DIR) {
        $installDir = $env:TIONHARNESS_INSTALL_DIR
    } else {
        $installDir = Join-Path $env:LOCALAPPDATA 'Programs\TionHarness'
    }
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null

    $target = Join-Path $installDir $binaryName
    Copy-Item -LiteralPath $extracted -Destination $target -Force

    Write-Host "Installed tionharness $version to $target"

    $onPath = ($env:PATH -split [System.IO.Path]::PathSeparator | Where-Object { $_ } |
        Where-Object { $_.TrimEnd('\', '/') -ieq $installDir.TrimEnd('\', '/') })
    if (-not $onPath) {
        Write-Host ''
        Write-Host "$installDir is not on your PATH. Add it for your user account by running:"
        Write-Host ''
        Write-Host "    setx PATH `"$installDir;`$env:PATH`""
        Write-Host ''
        Write-Host 'then open a new terminal.'
    }
} finally {
    if (Test-Path -LiteralPath $workDir) {
        Remove-Item -LiteralPath $workDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
