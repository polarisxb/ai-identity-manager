$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$PidPath = Join-Path $Root "run-logs\identity-keeper.pid"

if (!(Test-Path $PidPath)) {
    Write-Host "identity-keeper pid file not found"
    exit 0
}

$pidValue = [int](Get-Content $PidPath)
$process = Get-Process -Id $pidValue -ErrorAction SilentlyContinue
if ($process) {
    Stop-Process -Id $pidValue
    Write-Host "identity-keeper stopped pid=$pidValue"
} else {
    Write-Host "identity-keeper not running pid=$pidValue"
}
