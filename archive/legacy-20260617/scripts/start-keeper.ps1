param(
    [string]$Listen = "127.0.0.1:18080",
    [string]$Config = "proxy-secret.local"
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$Exe = Join-Path $Root "identity-keeper.exe"
$ConfigPath = Join-Path $Root $Config
$LogDir = Join-Path $Root "run-logs"
$PidPath = Join-Path $LogDir "identity-keeper.pid"

function Test-LocalPort {
    param([string]$Address, [int]$Port)
    $client = [System.Net.Sockets.TcpClient]::new()
    try {
        $result = $client.BeginConnect($Address, $Port, $null, $null)
        if (-not $result.AsyncWaitHandle.WaitOne(500)) { return $false }
        $client.EndConnect($result)
        return $true
    } catch {
        return $false
    } finally {
        $client.Close()
    }
}

$hostPart, $portPart = $Listen.Split(":", 2)
$port = [int]$portPart

if (Test-LocalPort -Address $hostPart -Port $port) {
    Write-Host "identity-keeper already listening on $Listen"
    exit 0
}

if (!(Test-Path $Exe)) {
    Push-Location $Root
    try {
        go build -o identity-keeper.exe ./cmd/identity-keeper
    } finally {
        Pop-Location
    }
}

if (!(Test-Path $ConfigPath)) {
    throw "Config file not found: $ConfigPath"
}

New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
$out = Join-Path $LogDir "identity-keeper.out.log"
$err = Join-Path $LogDir "identity-keeper.err.log"

$p = Start-Process -FilePath $Exe `
    -ArgumentList @("--config", $ConfigPath, "--listen", $Listen) `
    -WorkingDirectory $Root `
    -RedirectStandardOutput $out `
    -RedirectStandardError $err `
    -PassThru `
    -WindowStyle Hidden

Set-Content -Path $PidPath -Value $p.Id
Start-Sleep -Milliseconds 800

if (!(Test-LocalPort -Address $hostPart -Port $port)) {
    Get-Content $out, $err -ErrorAction SilentlyContinue
    throw "identity-keeper failed to listen on $Listen"
}

Write-Host "identity-keeper started on $Listen pid=$($p.Id)"
