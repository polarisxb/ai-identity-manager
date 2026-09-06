param(
    [string]$VergeDir = (Join-Path $env:APPDATA "io.github.clash-verge-rev.clash-verge-rev"),
    [string]$KeeperName = "AI-Identity-Keeper",
    [string]$GroupName = "AI-Identity"
)

$ErrorActionPreference = "Stop"

$AddedRules = @(
    "PROCESS-NAME,Weixin.exe,DIRECT",
    "PROCESS-NAME,WeChat.exe,DIRECT",
    "PROCESS-NAME,WeChatAppEx.exe,DIRECT",
    "PROCESS-NAME,WeChatBrowser.exe,DIRECT",
    "PROCESS-NAME,WeChatUtility.exe,DIRECT",
    "DOMAIN-SUFFIX,weixin.qq.com,DIRECT",
    "DOMAIN-SUFFIX,wx.qq.com,DIRECT",
    "DOMAIN-SUFFIX,wechat.com,DIRECT",
    "DOMAIN-SUFFIX,servicewechat.com,DIRECT",
    "DOMAIN-SUFFIX,wechatpay.com,DIRECT",
    "DOMAIN-SUFFIX,qq.com,DIRECT",
    "DOMAIN-SUFFIX,qpic.cn,DIRECT",
    "DOMAIN-SUFFIX,gtimg.com,DIRECT",
    "DOMAIN-SUFFIX,claude.ai,$GroupName",
    "DOMAIN-SUFFIX,anthropic.com,$GroupName",
    "DOMAIN-SUFFIX,openai.com,$GroupName",
    "DOMAIN-SUFFIX,chatgpt.com,$GroupName",
    "DOMAIN-SUFFIX,cursor.com,$GroupName",
    "DOMAIN-SUFFIX,cursor.sh,$GroupName",
    "DOMAIN-SUFFIX,ipinfo.io,$GroupName",
    "DOMAIN-SUFFIX,ping0.cc,$GroupName",
    "DOMAIN-SUFFIX,ip.sb,$GroupName",
    "DOMAIN-SUFFIX,ifconfig.me,$GroupName"
)

function Backup-File {
    param([string]$Path)
    if (!(Test-Path -LiteralPath $Path)) { return }
    $stamp = Get-Date -Format "yyyyMMdd-HHmmss"
    Copy-Item -LiteralPath $Path -Destination "$Path.bak-disable-$stamp" -Force
}

function Remove-NamedYamlBlock {
    param(
        [string]$Text,
        [string]$Name
    )

    $lines = $Text -split "\r?\n"
    $out = New-Object System.Collections.Generic.List[string]
    $skip = $false
    $namePattern = "^\s*-\s+name:\s*$([regex]::Escape($Name))\s*$"

    foreach ($line in $lines) {
        if ($line -match $namePattern) {
            $skip = $true
            continue
        }

        if ($skip) {
            if ($line -match "^\s*-\s+name:\s*" -or $line -match "^[A-Za-z0-9_-]+:\s*") {
                $skip = $false
            } else {
                continue
            }
        }

        if (!$skip) {
            $out.Add($line)
        }
    }

    return ($out -join "`r`n")
}

function Remove-AddedRulesAndMembers {
    param([string]$Text)

    $ruleSet = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
    foreach ($rule in $AddedRules) {
        [void]$ruleSet.Add($rule)
    }

    $lines = $Text -split "\r?\n"
    $out = New-Object System.Collections.Generic.List[string]
    foreach ($line in $lines) {
        $trim = $line.Trim()
        if ($trim -eq "- $KeeperName") { continue }
        if ($trim.StartsWith("- ")) {
            $rule = $trim.Substring(2)
            if ($ruleSet.Contains($rule)) { continue }
        }
        $out.Add($line)
    }
    return ($out -join "`r`n")
}

function Patch-File {
    param([string]$Path)
    if (!(Test-Path -LiteralPath $Path)) { return }

    $raw = Get-Content -LiteralPath $Path -Raw
    $patched = Remove-NamedYamlBlock -Text $raw -Name $KeeperName
    $patched = Remove-NamedYamlBlock -Text $patched -Name $GroupName
    $patched = Remove-AddedRulesAndMembers -Text $patched

    if ($patched -ne $raw) {
        Backup-File -Path $Path
        Set-Content -LiteralPath $Path -Value $patched -Encoding UTF8
        Write-Host "patched: $Path"
    } else {
        Write-Host "already clean: $Path"
    }
}

function Get-CurrentEnhancementFiles {
    param([string]$BaseDir)

    $profilesYaml = Join-Path $BaseDir "profiles.yaml"
    $raw = Get-Content -LiteralPath $profilesYaml -Raw
    $currentMatch = [regex]::Match($raw, "(?m)^current:\s*(\S+)\s*$")
    if (!$currentMatch.Success) { return @{} }

    $current = $currentMatch.Groups[1].Value
    $lines = $raw -split "\r?\n"
    $start = -1
    for ($i = 0; $i -lt $lines.Count; $i++) {
        if ($lines[$i] -match "^- uid:\s*$([regex]::Escape($current))\s*$") {
            $start = $i
            break
        }
    }
    if ($start -lt 0) { return @{} }

    $end = $lines.Count
    for ($i = $start + 1; $i -lt $lines.Count; $i++) {
        if ($lines[$i] -match "^- uid:\s*") {
            $end = $i
            break
        }
    }

    $block = ($lines[$start..($end - 1)] -join "`n")
    $result = @{}
    foreach ($key in @("proxies", "groups", "rules")) {
        $match = [regex]::Match($block, "(?m)^\s+${key}:\s*(\S+)\s*$")
        if ($match.Success) {
            $result[$key] = Join-Path (Join-Path $BaseDir "profiles") ($match.Groups[1].Value + ".yaml")
        }
    }
    return $result
}

function Get-MihomoSecret {
    param([string]$BaseDir)

    foreach ($name in @("config.yaml", "clash-verge.yaml")) {
        $path = Join-Path $BaseDir $name
        if (!(Test-Path -LiteralPath $path)) { continue }
        $raw = Get-Content -LiteralPath $path -Raw
        $match = [regex]::Match($raw, "(?m)^secret:\s*(.*?)\s*$")
        if ($match.Success) { return $match.Groups[1].Value.Trim("'`" ") }
    }
    return ""
}

function Invoke-MihomoPipe {
    param(
        [string]$BaseDir,
        [string]$Secret
    )

    $config = Join-Path $BaseDir "clash-verge.yaml"
    $body = @{ path = $config; force = $true } | ConvertTo-Json -Compress
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($body)
    $client = [System.IO.Pipes.NamedPipeClientStream]::new(".", "verge-mihomo", [System.IO.Pipes.PipeDirection]::InOut)

    try {
        $client.Connect(3000)
        $writer = [System.IO.StreamWriter]::new($client, [System.Text.Encoding]::ASCII, 1024, $true)
        $writer.NewLine = "`r`n"
        $writer.Write("PUT /configs?force=true HTTP/1.1`r`n")
        $writer.Write("Host: 127.0.0.1`r`n")
        if ($Secret) { $writer.Write("Authorization: Bearer $Secret`r`n") }
        $writer.Write("Content-Type: application/json`r`n")
        $writer.Write("Content-Length: $($bytes.Length)`r`n")
        $writer.Write("Connection: close`r`n`r`n")
        $writer.Flush()
        $client.Write($bytes, 0, $bytes.Length)
        $client.Flush()

        $reader = [System.IO.StreamReader]::new($client, [System.Text.Encoding]::UTF8)
        $response = $reader.ReadToEnd()
        $statusLine = (($response -split "`r?`n") | Select-Object -First 1)
        if ($statusLine -notmatch "HTTP/1\.1 2\d\d") {
            Write-Host $response
            throw "mihomo reload failed"
        }
        Write-Host "mihomo reloaded"
    } finally {
        try { $client.Dispose() } catch {}
    }
}

if (!(Test-Path -LiteralPath $VergeDir)) {
    throw "Clash Verge data dir not found: $VergeDir"
}

$files = Get-CurrentEnhancementFiles -BaseDir $VergeDir
foreach ($path in $files.Values) {
    Patch-File -Path $path
}

Patch-File -Path (Join-Path $VergeDir "clash-verge.yaml")
Patch-File -Path (Join-Path $VergeDir "clash-verge-check.yaml")
Invoke-MihomoPipe -BaseDir $VergeDir -Secret (Get-MihomoSecret -BaseDir $VergeDir)

& (Join-Path $PSScriptRoot "uninstall-keeper-startup-task.ps1")
& (Join-Path $PSScriptRoot "stop-keeper.ps1")

Write-Host "AI identity integration disabled."
