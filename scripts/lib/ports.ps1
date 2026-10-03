# Shared listener cleanup for the development launchers (Windows PowerShell 5.1).
function Free-Port {
    param(
        [int]$Port,
        [string]$Label,
        [switch]$NoKillPort,
        [scriptblock]$BeforeKill
    )

    $connections = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
    if (-not $connections) { return }
    $owners = $connections | Select-Object -ExpandProperty OwningProcess -Unique
    foreach ($processId in $owners) {
        $process = Get-Process -Id $processId -ErrorAction SilentlyContinue
        $processName = if ($process) { $process.ProcessName } else { "unknown" }
        if ($NoKillPort) {
            Write-Host "==> ERROR: $Label port $Port busy (PID $processId / $processName). -NoKillPort set, aborting." -ForegroundColor Red
            throw "port-busy"
        }
        Write-Host "==> $Label port $Port busy (PID $processId / $processName) -> killing orphan..." -ForegroundColor Yellow
        if ($BeforeKill) { & $BeforeKill $Port $processId $processName }
        taskkill /PID $processId /T /F 2>$null | Out-Null
    }
    Start-Sleep -Milliseconds 400
}
