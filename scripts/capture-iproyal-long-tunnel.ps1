param(
    [string]$Url = "https://speed.cloudflare.com/__down?bytes=10485760",
    [string]$Range = "",
    [string]$Rate = "25k",
    [ValidateSet("http", "socks5h")]
    [string]$ProxyMode = "http",
    [int]$MaxTimeSeconds = 600,
    [string]$Config = "proxy-secret.local",
    [string]$LogDir = "probe-logs"
)

$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $PSScriptRoot
$ConfigPath = Join-Path $Root $Config
$LogPath = Join-Path $Root $LogDir
New-Item -ItemType Directory -Force -Path $LogPath | Out-Null

function Read-KvFile {
    param([string]$Path)
    $values = @{}
    Get-Content -LiteralPath $Path | ForEach-Object {
        $line = $_.Trim()
        if (!$line -or $line.StartsWith("#") -or !$line.Contains("=")) { return }
        $key, $value = $line.Split("=", 2)
        $values[$key.Trim()] = $value.Trim()
    }
    return $values
}

function Redact-Log {
    param([string]$Text, [hashtable]$Values)

    $safe = $Text
    foreach ($key in @("PROXY_USER", "PROXY_PASS")) {
        if ($Values.ContainsKey($key) -and $Values[$key]) {
            $safe = $safe.Replace($Values[$key], "<redacted-$key>")
        }
    }
    $safe = $safe -replace "(Proxy-Authorization:\s+Basic\s+)[A-Za-z0-9+/=]+", '${1}<redacted-basic-auth>'
    $safe = $safe -replace "(https?://)[^/@\s]+:[^/@\s]+@", '${1}<redacted-credentials>@'
    return $safe
}

$values = Read-KvFile -Path $ConfigPath
$portKey = "PROXY_HTTP_PORT"
if ($ProxyMode -eq "socks5h") {
    $portKey = "PROXY_SOCKS_PORT"
}

foreach ($required in @("PROXY_HOST", $portKey, "PROXY_USER", "PROXY_PASS")) {
    if (!$values.ContainsKey($required) -or !$values[$required]) {
        throw "$required is missing in $ConfigPath"
    }
}

$user = [uri]::EscapeDataString($values["PROXY_USER"])
$pass = [uri]::EscapeDataString($values["PROXY_PASS"])
$proxy = "${ProxyMode}://$user`:$pass@$($values["PROXY_HOST"]):$($values[$portKey])"
$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
$logFile = Join-Path $LogPath "iproyal-long-tunnel-$stamp.log"

Write-Host "LONG_TUNNEL_LOG=$logFile"
Write-Host "PROXY_MODE=$ProxyMode"
Write-Host "URL=$Url"
if ($Range) { Write-Host "RANGE=$Range" }
Write-Host "RATE=$Rate"
Write-Host "MAX_TIME_SECONDS=$MaxTimeSeconds"

$curlArgs = @(
    "-v",
    "--connect-timeout", "20",
    "--max-time", "$MaxTimeSeconds",
    "--limit-rate", $Rate,
    "-x", $proxy,
    $Url,
    "-o", "NUL",
    "-w", "`nCURL_HTTP_CODE:%{http_code}`nTIME_TOTAL:%{time_total}`nSIZE_DOWNLOAD:%{size_download}`nSPEED_DOWNLOAD:%{speed_download}`n"
)

if ($Range) {
    $curlArgs = @(
        "-v",
        "--connect-timeout", "20",
        "--max-time", "$MaxTimeSeconds",
        "--limit-rate", $Rate,
        "-r", $Range,
        "-x", $proxy,
        $Url,
        "-o", "NUL",
        "-w", "`nCURL_HTTP_CODE:%{http_code}`nTIME_TOTAL:%{time_total}`nSIZE_DOWNLOAD:%{size_download}`nSPEED_DOWNLOAD:%{speed_download}`n"
    )
}

$previousErrorActionPreference = $ErrorActionPreference
$ErrorActionPreference = "Continue"
try {
    $output = & curl.exe @curlArgs 2>&1
    $exitCode = $LASTEXITCODE
} finally {
    $ErrorActionPreference = $previousErrorActionPreference
}

$text = ($output | Out-String)
$safe = Redact-Log -Text $text -Values $values
@(
    "Timestamp: $(Get-Date -Format "yyyy-MM-dd HH:mm:ss")",
    "Proxy mode: $ProxyMode",
    "Proxy endpoint: $($values["PROXY_HOST"]):$($values[$portKey])",
    "URL: $Url",
    "Range: $Range",
    "Rate limit: $Rate",
    "Max time seconds: $MaxTimeSeconds",
    "curl exit code: $exitCode",
    "",
    $safe
) | Set-Content -LiteralPath $logFile -Encoding UTF8

if ($exitCode -eq 0) {
    Write-Host "LONG_TUNNEL_OK=$logFile"
    exit 0
}

Write-Host "LONG_TUNNEL_FAILED=$logFile"
exit $exitCode
