param(
    [string]$Proxy = "http://127.0.0.1:18080",
    [string]$Url = "https://ipinfo.io"
)

$ErrorActionPreference = "Stop"
$out = curl.exe -sS --max-time 30 -x $Proxy $Url
if ($LASTEXITCODE -ne 0) {
    throw "curl failed with exit code $LASTEXITCODE"
}
$out
