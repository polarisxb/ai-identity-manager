param(
    [string]$VergeDir = (Join-Path $env:APPDATA "io.github.clash-verge-rev.clash-verge-rev"),
    [string]$KeeperName = "AI-Identity-Keeper",
    [string]$GroupName = "AI-Identity",
    [string]$KeeperHost = "127.0.0.1",
    [int]$KeeperPort = 18080,
    [int]$ClashPort = 7889,
    [switch]$NoKeeperStartup,
    [switch]$SkipRuntimePatch,
    [switch]$SkipCoreRestart
)

$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $PSScriptRoot
$BypassRules = @(
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
    "DOMAIN-SUFFIX,gtimg.com,DIRECT"
)
$Rules = @(
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
    $stamp = Get-Date -Format "yyyyMMdd-HHmmss"
    Copy-Item -LiteralPath $Path -Destination "$Path.bak-$stamp" -Force
}

function Insert-AfterHeader {
    param(
        [string]$Text,
        [string]$Header,
        [string[]]$Lines
    )

    $pattern = "(?m)^$([regex]::Escape($Header))\s*$"
    $match = [regex]::Match($Text, $pattern)
    if (!$match.Success) {
        throw "Header not found in YAML: $Header"
    }

    $block = ($Lines -join "`r`n") + "`r`n"
    return $Text.Substring(0, $match.Index + $match.Length) + "`r`n" + $block + $Text.Substring($match.Index + $match.Length)
}

function Ensure-EnhancementBlock {
    param(
        [string]$Path,
        [string[]]$Lines,
        [string]$Needle
    )

    if (!(Test-Path -LiteralPath $Path)) {
        $content = "prepend:`r`n" + ($Lines -join "`r`n") + "`r`nappend: []`r`ndelete: []`r`n"
        Set-Content -LiteralPath $Path -Value $content -Encoding UTF8
        Write-Host "created enhancement: $Path"
        return
    }

    $raw = Get-Content -LiteralPath $Path -Raw
    if ($raw.Contains($Needle)) {
        Write-Host "enhancement already has $Needle"
        return
    }

    Backup-File -Path $Path
    if ($raw -match "(?m)^prepend:\s*$") {
        $raw = Insert-AfterHeader -Text $raw -Header "prepend:" -Lines $Lines
    } else {
        $raw = "prepend:`r`n" + ($Lines -join "`r`n") + "`r`n" + $raw
    }
    Set-Content -LiteralPath $Path -Value $raw -Encoding UTF8
    Write-Host "patched enhancement: $Path"
}

function Get-CurrentEnhancementFiles {
    param([string]$BaseDir)

    $profilesYaml = Join-Path $BaseDir "profiles.yaml"
    if (!(Test-Path -LiteralPath $profilesYaml)) {
        throw "profiles.yaml not found: $profilesYaml"
    }

    $raw = Get-Content -LiteralPath $profilesYaml -Raw
    $currentMatch = [regex]::Match($raw, "(?m)^current:\s*(\S+)\s*$")
    if (!$currentMatch.Success) {
        throw "current profile not found in profiles.yaml"
    }

    $current = $currentMatch.Groups[1].Value
    $lines = $raw -split "\r?\n"
    $start = -1
    for ($i = 0; $i -lt $lines.Count; $i++) {
        if ($lines[$i] -match "^- uid:\s*$([regex]::Escape($current))\s*$") {
            $start = $i
            break
        }
    }
    if ($start -lt 0) {
        throw "current profile block not found: $current"
    }

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

function Patch-Enhancements {
    param([string]$BaseDir)

    $files = Get-CurrentEnhancementFiles -BaseDir $BaseDir

    if ($files.ContainsKey("proxies")) {
        Ensure-EnhancementBlock -Path $files["proxies"] -Needle $KeeperName -Lines @(
            "  - name: $KeeperName",
            "    type: http",
            "    server: $KeeperHost",
            "    port: $KeeperPort"
        )
    }

    if ($files.ContainsKey("groups")) {
        Ensure-EnhancementBlock -Path $files["groups"] -Needle "name: $GroupName" -Lines @(
            "  - name: $GroupName",
            "    type: select",
            "    proxies:",
            "      - $KeeperName"
        )
    }

	if ($files.ContainsKey("rules")) {
		$bypassLines = $BypassRules | ForEach-Object { "  - $_" }
		Ensure-EnhancementBlock -Path $files["rules"] -Needle "PROCESS-NAME,Weixin.exe,DIRECT" -Lines $bypassLines

		$ruleLines = $Rules | ForEach-Object { "  - $_" }
		Ensure-EnhancementBlock -Path $files["rules"] -Needle "claude.ai,$GroupName" -Lines $ruleLines
	}
}

function Patch-RuntimeConfig {
    param([string]$Path)

    if (!(Test-Path -LiteralPath $Path)) {
        Write-Host "runtime config skipped, not found: $Path"
        return
    }

    $raw = Get-Content -LiteralPath $Path -Raw
    $patched = $raw
    $keeperPattern = "(?m)^-\s+name:\s*$([regex]::Escape($KeeperName))\s*$"
    $groupPattern = "(?m)^-\s+name:\s*$([regex]::Escape($GroupName))\s*$"

    if ($patched -notmatch $keeperPattern) {
        $patched = Insert-AfterHeader -Text $patched -Header "proxies:" -Lines @(
            "- name: $KeeperName",
            "  type: http",
            "  server: $KeeperHost",
            "  port: $KeeperPort"
        )
    }

    if ($patched -notmatch $groupPattern) {
        $patched = Insert-AfterHeader -Text $patched -Header "proxy-groups:" -Lines @(
            "- name: $GroupName",
            "  type: select",
            "  proxies:",
            "  - $KeeperName"
        )
    }

	if (!$patched.Contains("PROCESS-NAME,Weixin.exe,DIRECT")) {
		$bypassLines = $BypassRules | ForEach-Object { "- $_" }
		$patched = Insert-AfterHeader -Text $patched -Header "rules:" -Lines $bypassLines
	}

	if (!$patched.Contains("claude.ai,$GroupName")) {
		$ruleLines = $Rules | ForEach-Object { "- $_" }
		$patched = Insert-AfterHeader -Text $patched -Header "rules:" -Lines $ruleLines
	}

	$patched = Remove-KeeperFromNonIdentityGroups -Text $patched

	if ($patched -ne $raw) {
		Backup-File -Path $Path
        Set-Content -LiteralPath $Path -Value $patched -Encoding UTF8
        Write-Host "patched runtime config: $Path"
    } else {
        Write-Host "runtime config already patched: $Path"
	}
}

function Remove-KeeperFromNonIdentityGroups {
	param([string]$Text)

	$lines = $Text -split "\r?\n"
	$out = New-Object System.Collections.Generic.List[string]
	$inGroups = $false
	$currentGroup = ""

	foreach ($line in $lines) {
		if ($line -match "^proxy-groups:\s*$") {
			$inGroups = $true
			$currentGroup = ""
			$out.Add($line)
			continue
		}
		if ($line -match "^rules:\s*$") {
			$inGroups = $false
			$currentGroup = ""
			$out.Add($line)
			continue
		}
		if ($inGroups -and $line -match "^- name:\s*(.+?)\s*$") {
			$currentGroup = $Matches[1]
			$out.Add($line)
			continue
		}
		if ($inGroups -and $currentGroup -ne $GroupName -and $line -match "^\s+-\s+$([regex]::Escape($KeeperName))\s*$") {
			continue
		}
		$out.Add($line)
	}

	return ($out -join "`r`n")
}

function Test-LocalPort {
    param([string]$HostName, [int]$Port, [int]$TimeoutMs = 500)

    $client = [System.Net.Sockets.TcpClient]::new()
    try {
        $result = $client.BeginConnect($HostName, $Port, $null, $null)
        if (!$result.AsyncWaitHandle.WaitOne($TimeoutMs)) { return $false }
        $client.EndConnect($result)
        return $true
    } catch {
        return $false
    } finally {
        $client.Close()
    }
}

function Wait-LocalPort {
    param([string]$HostName, [int]$Port, [int]$Seconds)

    $deadline = (Get-Date).AddSeconds($Seconds)
    while ((Get-Date) -lt $deadline) {
        if (Test-LocalPort -HostName $HostName -Port $Port -TimeoutMs 400) {
            return $true
        }
        Start-Sleep -Milliseconds 300
    }
    return $false
}

function Start-Keeper {
    if ($NoKeeperStartup) {
        & (Join-Path $PSScriptRoot "start-keeper.ps1")
        return
    }

    & (Join-Path $PSScriptRoot "install-keeper-startup-task.ps1")
}

function Get-MihomoSecret {
    param([string]$BaseDir)

    foreach ($name in @("config.yaml", "clash-verge.yaml")) {
        $path = Join-Path $BaseDir $name
        if (!(Test-Path -LiteralPath $path)) { continue }

        $raw = Get-Content -LiteralPath $path -Raw
        $match = [regex]::Match($raw, "(?m)^secret:\s*(.*?)\s*$")
        if ($match.Success) {
            return $match.Groups[1].Value.Trim("'`" ")
        }
    }

    return ""
}

function Invoke-MihomoPipe {
    param(
        [string]$Method,
        [string]$Path,
        [string]$Body = ""
    )

    $bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($Body)
    $client = [System.IO.Pipes.NamedPipeClientStream]::new(".", "verge-mihomo", [System.IO.Pipes.PipeDirection]::InOut)

    try {
        $client.Connect(3000)
        $writer = [System.IO.StreamWriter]::new($client, [System.Text.Encoding]::ASCII, 1024, $true)
        $writer.NewLine = "`r`n"
        $writer.Write("$Method $Path HTTP/1.1`r`n")
        $writer.Write("Host: 127.0.0.1`r`n")
        if ($script:MihomoSecret) {
            $writer.Write("Authorization: Bearer $script:MihomoSecret`r`n")
        }
        $writer.Write("Content-Type: application/json`r`n")
        $writer.Write("Content-Length: $($bodyBytes.Length)`r`n")
        $writer.Write("Connection: close`r`n`r`n")
        $writer.Flush()

        if ($bodyBytes.Length -gt 0) {
            $client.Write($bodyBytes, 0, $bodyBytes.Length)
            $client.Flush()
        }

        $reader = [System.IO.StreamReader]::new($client, [System.Text.Encoding]::UTF8)
        return $reader.ReadToEnd()
    } finally {
        try { $client.Dispose() } catch {}
    }
}

function Reload-CoreViaPipe {
    param([string]$BaseDir)

    $config = Join-Path $BaseDir "clash-verge.yaml"
    $body = @{ path = $config; force = $true } | ConvertTo-Json -Compress
    $response = Invoke-MihomoPipe -Method "PUT" -Path "/configs?force=true" -Body $body
    $statusLine = (($response -split "`r?`n") | Select-Object -First 1)

    if ($statusLine -match "HTTP/1\.1 2\d\d") {
        Write-Host "mihomo reloaded via named pipe"
        return $true
    }

    Write-Host $response
    return $false
}

function Restart-Core {
    param([string]$BaseDir)

    if ($SkipCoreRestart) {
        Write-Host "core restart skipped"
        return
    }

    if (Reload-CoreViaPipe -BaseDir $BaseDir) {
        return
    }

    try {
        Get-Process verge-mihomo -ErrorAction SilentlyContinue | Stop-Process -Force
    } catch {
        throw "mihomo reload failed and process restart is not permitted: $($_.Exception.Message)"
    }

    if (Wait-LocalPort -HostName "127.0.0.1" -Port $ClashPort -Seconds 8) {
        Write-Host "mihomo restarted by Clash Verge"
        return
    }

    $coreCandidates = @(
        "C:\Program Files\Clash Verge\verge-mihomo.exe",
        "C:\Program Files\Clash Verge\verge-mihomo-alpha.exe"
    )
    $core = $coreCandidates | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
    if (!$core) {
        throw "verge-mihomo.exe not found under C:\Program Files\Clash Verge"
    }

    $config = Join-Path $BaseDir "clash-verge.yaml"
    Start-Process -FilePath $core `
        -ArgumentList @("-d", "`"$BaseDir`"", "-f", "`"$config`"") `
        -WindowStyle Hidden `
        -WorkingDirectory $BaseDir | Out-Null

    if (!(Wait-LocalPort -HostName "127.0.0.1" -Port $ClashPort -Seconds 10)) {
        throw "mihomo did not listen on 127.0.0.1:$ClashPort after restart"
    }
    Write-Host "mihomo started on 127.0.0.1:$ClashPort"
}

if (!(Test-Path -LiteralPath $VergeDir)) {
    throw "Clash Verge data dir not found: $VergeDir"
}

$script:MihomoSecret = Get-MihomoSecret -BaseDir $VergeDir
Start-Keeper
Patch-Enhancements -BaseDir $VergeDir

if (!$SkipRuntimePatch) {
    Patch-RuntimeConfig -Path (Join-Path $VergeDir "clash-verge.yaml")
    Patch-RuntimeConfig -Path (Join-Path $VergeDir "clash-verge-check.yaml")
}

Restart-Core -BaseDir $VergeDir

Write-Host "AI identity rule mode enabled."
