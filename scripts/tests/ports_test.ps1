# Exercise listener cleanup with synthetic processes; no live process is stopped.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "..\lib\ports.ps1")
$script:PortTestListeners = @()
$script:PortTestEvents = New-Object 'System.Collections.Generic.List[string]'

function Get-NetTCPConnection { return $script:PortTestListeners }
function Get-Process { return $null }
function taskkill { $script:PortTestEvents.Add("kill:$($args -join ' ')") }
function Start-Sleep { param([int]$Milliseconds) $script:PortTestEvents.Add("sleep:$Milliseconds") }
function Write-Host { }
function Assert-Events([string[]]$Expected) {
    $actual = $script:PortTestEvents -join "|"
    if ($actual -ne ($Expected -join "|")) { throw "Unexpected cleanup events: $actual" }
    $script:PortTestEvents.Clear()
}

Free-Port 8090 "Backend"
Assert-Events @()

$script:PortTestListeners = @(
    [pscustomobject]@{ OwningProcess = 1001 },
    [pscustomobject]@{ OwningProcess = 1001 },
    [pscustomobject]@{ OwningProcess = 1002 }
)
Free-Port 8090 "Backend" -BeforeKill {
    param($portNumber, $processId, $processName)
    $script:PortTestEvents.Add("log:${portNumber}:${processId}:${processName}")
}
Assert-Events @(
    "log:8090:1001:unknown", "kill:/PID 1001 /T /F",
    "log:8090:1002:unknown", "kill:/PID 1002 /T /F", "sleep:400"
)

$rejected = $false
try {
    Free-Port 8090 "Backend" -NoKillPort -BeforeKill { throw "Must not log a kill" }
} catch {
    if ($_.Exception.Message -ne "port-busy") { throw }
    $rejected = $true
}
if (-not $rejected) { throw "A busy port must be rejected when NoKillPort is set" }
Assert-Events @()
Write-Output "Port cleanup regression checks: ok"
