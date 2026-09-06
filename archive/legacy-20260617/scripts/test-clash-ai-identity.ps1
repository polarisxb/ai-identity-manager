param(
    [string]$KeeperProxy = "http://127.0.0.1:18080",
    [string]$ClashProxy = "http://127.0.0.1:7889",
    [string]$Url = "https://ipinfo.io"
)

$ErrorActionPreference = "Stop"

function Invoke-ProxyJson {
    param([string]$Proxy, [string]$TargetUrl)

    $raw = curl.exe -sS --max-time 30 -x $Proxy $TargetUrl
    if ($LASTEXITCODE -ne 0) {
        throw "curl failed through $Proxy with exit code $LASTEXITCODE"
    }
    return $raw | ConvertFrom-Json
}

$keeper = Invoke-ProxyJson -Proxy $KeeperProxy -TargetUrl $Url
$clash = Invoke-ProxyJson -Proxy $ClashProxy -TargetUrl $Url

Write-Host "Keeper: $($keeper.ip) $($keeper.org)"
Write-Host "Clash : $($clash.ip) $($clash.org)"

if ($keeper.ip -ne $clash.ip) {
    throw "Clash rule path is not using the Keeper identity. Keeper=$($keeper.ip), Clash=$($clash.ip)"
}

Write-Host "OK: Clash rule mode is using the same static identity as Identity Keeper."
