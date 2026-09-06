param(
    [string]$Url = "https://ipinfo.io",
    [int]$Attempts = 60,
    [int]$DelaySeconds = 5,
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
    param(
        [string]$Text,
        [hashtable]$Values
    )

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

function Is-Failure {
    param([int]$ExitCode, [string]$Text)

    if ($ExitCode -ne 0) { return $true }
    if ($Text -match "HTTP/1\.[01]\s+50[0-9]") { return $true }
    if ($Text -match "CONNECT tunnel failed") { return $true }
    if ($Text -match "Failed to connect") { return $true }
    if ($Text -match "Connection reset|forcibly closed|timed out|timeout|EOF|502 Bad Gateway|i/o timeout") { return $true }
    return $false
}

$values = Read-KvFile -Path $ConfigPath
foreach ($required in @("PROXY_HOST", "PROXY_HTTP_PORT", "PROXY_USER", "PROXY_PASS")) {
    if (!$values.ContainsKey($required) -or !$values[$required]) {
        throw "$required is missing in $ConfigPath"
    }
}

$user = [uri]::EscapeDataString($values["PROXY_USER"])
$pass = [uri]::EscapeDataString($values["PROXY_PASS"])
$proxy = "http://$user`:$pass@$($values["PROXY_HOST"]):$($values["PROXY_HTTP_PORT"])"
$summaryPath = Join-Path $LogPath ("iproyal-curl-summary-{0}.log" -f (Get-Date -Format "yyyyMMdd-HHmmss"))

"Target URL: $Url" | Set-Content -LiteralPath $summaryPath -Encoding UTF8
"Proxy endpoint: $($values["PROXY_HOST"]):$($values["PROXY_HTTP_PORT"])" | Add-Content -LiteralPath $summaryPath -Encoding UTF8
"Attempts: $Attempts delay=${DelaySeconds}s" | Add-Content -LiteralPath $summaryPath -Encoding UTF8
"" | Add-Content -LiteralPath $summaryPath -Encoding UTF8

for ($i = 1; $i -le $Attempts; $i++) {
    $stamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    Write-Host "attempt $i/$Attempts $stamp"

    $curlArgs = @(
        "-v",
        "--connect-timeout", "15",
        "--max-time", "45",
        "-x", $proxy,
        $Url,
        "-o", "NUL",
        "-w", "`nCURL_HTTP_CODE:%{http_code}`n"
    )

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

    "[$stamp] attempt=$i exit=$exitCode" | Add-Content -LiteralPath $summaryPath -Encoding UTF8
    (($safe -split "\r?\n") | Where-Object {
        $_ -match "Trying|Connected|CONNECT|HTTP/|error|failed|timeout|timed out|reset|forcibly|CURL_HTTP_CODE|502|503|504"
    }) | Add-Content -LiteralPath $summaryPath -Encoding UTF8
    "" | Add-Content -LiteralPath $summaryPath -Encoding UTF8

    if (Is-Failure -ExitCode $exitCode -Text $safe) {
        $failurePath = Join-Path $LogPath ("iproyal-curl-failure-{0}-attempt-{1}.log" -f (Get-Date -Format "yyyyMMdd-HHmmss"), $i)
        @(
            "Timestamp: $stamp",
            "Target URL: $Url",
            "Proxy endpoint: $($values["PROXY_HOST"]):$($values["PROXY_HTTP_PORT"])",
            "curl exit code: $exitCode",
            "",
            $safe
        ) | Set-Content -LiteralPath $failurePath -Encoding UTF8
        Write-Host "FAILURE_LOG=$failurePath"
        exit 2
    }

    if ($i -lt $Attempts) {
        Start-Sleep -Seconds $DelaySeconds
    }
}

Write-Host "NO_FAILURE_CAPTURED=$summaryPath"
