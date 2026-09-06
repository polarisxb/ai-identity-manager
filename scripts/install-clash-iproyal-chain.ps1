param(
  [string]$SecretFile = "$PSScriptRoot\..\proxy-secret.local",
  [string]$ClashDir = "$env:APPDATA\io.github.clash-verge-rev.clash-verge-rev",
  [string]$NodeName = "IPRoyal-Static",
  [string]$FinalGroupName = "AI-Static",
  [string]$BootstrapGroupName = "JMS-Bootstrap",
  [ValidateSet("http", "socks5")]
  [string]$ProxyMode = "http",
  [string]$BootstrapNode = "",
  [switch]$NoRules
)

$ErrorActionPreference = "Stop"

function Read-LocalEnvFile {
  param([string]$Path)

  if (-not (Test-Path -LiteralPath $Path)) {
    throw "Secret file not found: $Path"
  }

  $values = @{}
  foreach ($line in Get-Content -LiteralPath $Path) {
    $trimmed = $line.Trim()
    if ($trimmed.Length -eq 0 -or $trimmed.StartsWith("#")) {
      continue
    }
    $parts = $trimmed.Split("=", 2)
    if ($parts.Count -ne 2) {
      continue
    }
    $values[$parts[0].Trim()] = $parts[1].Trim()
  }

  return $values
}

function Escape-YamlSingleQuoted {
  param([string]$Value)
  return "'" + ($Value -replace "'", "''") + "'"
}

function Backup-File {
  param([string]$Path)

  if (Test-Path -LiteralPath $Path) {
    $stamp = Get-Date -Format "yyyyMMdd-HHmmss"
    Copy-Item -LiteralPath $Path -Destination "$Path.bak-iproyal-$stamp" -Force
  }
}

function Get-CurrentProfileBlock {
  param([string]$ProfilesYaml)

  $text = Get-Content -LiteralPath $ProfilesYaml -Raw
  $currentMatch = [regex]::Match($text, "(?m)^current:\s*(\S+)\s*$")
  if (-not $currentMatch.Success) {
    throw "Cannot find current profile uid in profiles.yaml"
  }

  $currentUid = [regex]::Escape($currentMatch.Groups[1].Value)
  $blockMatch = [regex]::Match($text, "(?ms)^- uid:\s*$currentUid\s*\r?\n.*?(?=^- uid:|\z)")
  if (-not $blockMatch.Success) {
    throw "Cannot find current profile block in profiles.yaml"
  }

  return @{
    Uid = $currentMatch.Groups[1].Value
    Text = $blockMatch.Value
  }
}

function Get-OptionUid {
  param(
    [string]$ProfileBlock,
    [string]$Name
  )

  $match = [regex]::Match($ProfileBlock, "(?m)^\s{4}$([regex]::Escape($Name)):\s*(\S+)\s*$")
  if (-not $match.Success) {
    throw "Current profile does not bind option '$Name'"
  }

  return $match.Groups[1].Value
}

function Get-ItemFile {
  param(
    [string]$ProfilesYaml,
    [string]$Uid
  )

  $text = Get-Content -LiteralPath $ProfilesYaml -Raw
  $escaped = [regex]::Escape($Uid)
  $blockMatch = [regex]::Match($text, "(?ms)^- uid:\s*$escaped\s*\r?\n.*?(?=^- uid:|\z)")
  if (-not $blockMatch.Success) {
    throw "Cannot find profile item '$Uid'"
  }

  $fileMatch = [regex]::Match($blockMatch.Value, "(?m)^\s{2}file:\s*(.+?)\s*$")
  if (-not $fileMatch.Success) {
    throw "Cannot find file for profile item '$Uid'"
  }

  return $fileMatch.Groups[1].Value.Trim()
}

function Get-SelectedProxy {
  param([string]$ProfileBlock)

  $match = [regex]::Match($ProfileBlock, "(?ms)- name:\s*Proxies\s*\r?\n\s*now:\s*(.+?)\s*(?:\r?\n|$)")
  if (-not $match.Success) {
    return ""
  }

  return $match.Groups[1].Value.Trim().Trim("'").Trim('"')
}

function Get-GeneratedProxyNames {
  param([string]$ClashDir)

  $configPath = Join-Path $ClashDir "clash-verge.yaml"
  if (-not (Test-Path -LiteralPath $configPath)) {
    return @()
  }

  $names = New-Object System.Collections.Generic.List[string]
  $inProxySection = $false
  foreach ($line in Get-Content -LiteralPath $configPath) {
    if ($line -match "^proxies:\s*$") {
      $inProxySection = $true
      continue
    }
    if ($line -match "^proxy-groups:\s*$") {
      break
    }
    if (-not $inProxySection) {
      continue
    }

    $match = [regex]::Match($line, "^\s*-\s+name:\s*(JMS-.+?)\s*$")
    if ($match.Success) {
      $value = $match.Groups[1].Value.Trim().Trim("'").Trim('"')
      if (-not $names.Contains($value)) {
        $names.Add($value)
      }
    }
  }

  return @($names)
}

function Decode-Base64Url {
  param([string]$Value)
  $t = $Value.Replace('-', '+').Replace('_', '/')
  switch ($t.Length % 4) {
    2 { $t += "==" }
    3 { $t += "=" }
    1 { $t += "===" }
  }
  return [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($t))
}

function Get-ProfileSubscriptionUrl {
  param([string]$ProfileBlock)

  $match = [regex]::Match($ProfileBlock, "(?m)^\s{2}url:\s*(.+?)\s*$")
  if (-not $match.Success) {
    return ""
  }
  return $match.Groups[1].Value.Trim().Trim("'").Trim('"')
}

function Get-RawJmsSubscriptionUrl {
  param([string]$ProfileUrl)

  if ([string]::IsNullOrWhiteSpace($ProfileUrl)) {
    return ""
  }
  # Clash Verge often stores a converter URL with the real sub in ?url=
  if ($ProfileUrl -match 'url=([^&]+)') {
    try {
      return [Uri]::UnescapeDataString($Matches[1])
    } catch {
      return $Matches[1]
    }
  }
  if ($ProfileUrl -match 'jmssub\.net|getsub\.php') {
    return $ProfileUrl
  }
  return ""
}

function Get-JmsNodesFromSubscription {
  param([string]$SubscriptionUrl)

  $nodes = @()
  if ([string]::IsNullOrWhiteSpace($SubscriptionUrl)) {
    return $nodes
  }

  try {
    $resp = Invoke-WebRequest -Uri $SubscriptionUrl -UseBasicParsing -TimeoutSec 30
    $body = $resp.Content.Trim()
  } catch {
    Write-Warning "Failed to fetch JMS subscription for VLESS inject: $($_.Exception.Message)"
    return $nodes
  }

  $decoded = $body
  try {
    $decoded = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($body))
  } catch {
    # already plain text
  }

  foreach ($line in ($decoded -split "\r?\n")) {
    $line = $line.Trim()
    if ($line.Length -eq 0) { continue }

    if ($line.StartsWith("ss://")) {
      $hashIdx = $line.LastIndexOf("#")
      $name = if ($hashIdx -ge 0) { [Uri]::UnescapeDataString($line.Substring($hashIdx + 1)) } else { "ss" }
      $payload = if ($hashIdx -ge 0) { $line.Substring(5, $hashIdx - 5) } else { $line.Substring(5) }
      try {
        $info = Decode-Base64Url $payload
      } catch {
        continue
      }
      if ($info -match '^([^:]+):([^@]+)@([^:]+):(\d+)$') {
        $nodes += [pscustomobject]@{
          Kind     = "ss"
          Name     = $name
          Server   = $Matches[3]
          Port     = $Matches[4]
          Cipher   = $Matches[1]
          Password = $Matches[2]
        }
      }
    } elseif ($line.StartsWith("vless://")) {
      if ($line -notmatch '^vless://([^@]+)@([^:]+):(\d+)\?([^#]+)#(.+)$') {
        continue
      }
      $uuid = $Matches[1]
      $server = $Matches[2]
      $port = $Matches[3]
      $qs = $Matches[4]
      $name = [Uri]::UnescapeDataString($Matches[5])
      $params = @{}
      foreach ($pair in ($qs -split '&')) {
        $kv = $pair -split '=', 2
        if ($kv.Count -eq 2) {
          $params[$kv[0]] = [Uri]::UnescapeDataString($kv[1])
        }
      }
      $nodes += [pscustomobject]@{
        Kind   = "vless"
        Name   = $name
        Server = $server
        Port   = $port
        Uuid   = $uuid
        Flow   = $params["flow"]
        Sni    = $params["sni"]
        Fp     = $params["fp"]
        Pbk    = $params["pbk"]
        Sid    = $params["sid"]
      }
    }
  }

  # Do not use unary comma here: caller wraps with @() to recollect pipeline output.
  return $nodes
}

function Get-RemoteProfileProxyNames {
  param(
    [string]$ProfilesDir,
    [string]$ProfileFileName
  )

  $path = Join-Path $ProfilesDir $ProfileFileName
  $names = @{}
  if (-not (Test-Path -LiteralPath $path)) {
    return $names
  }
  foreach ($line in Get-Content -LiteralPath $path) {
    # remote profile may use flow-style: - {name: JMS-..., ...}
    foreach ($m in [regex]::Matches($line, "name:\s*['""]?(JMS-[^\s,'""}]+)")) {
      $names[$m.Groups[1].Value.Trim()] = $true
    }
  }
  return $names
}

function Get-SelectedBootstrapFromProfile {
  param([string]$ProfileBlock)

  # Prefer current JMS-Bootstrap selection over the generic Proxies group
  # (Proxies may still hold stale names like "HK").
  $match = [regex]::Match($ProfileBlock, "(?ms)- name:\s*JMS-Bootstrap\s*\r?\n\s*now:\s*(.+?)\s*(?:\r?\n|$)")
  if ($match.Success) {
    return $match.Groups[1].Value.Trim().Trim("'").Trim('"')
  }
  return ""
}

function Write-Lines {
  param(
    [string]$Path,
    [string[]]$Lines
  )

  Set-Content -LiteralPath $Path -Value $Lines -Encoding UTF8
}

$secret = Read-LocalEnvFile -Path $SecretFile
foreach ($key in @("PROXY_HOST", "PROXY_HTTP_PORT", "PROXY_SOCKS_PORT", "PROXY_USER", "PROXY_PASS")) {
  if (-not $secret.ContainsKey($key) -or [string]::IsNullOrWhiteSpace($secret[$key])) {
    throw "Missing required key in proxy-secret.local: $key"
  }
}

$profilesYaml = Join-Path $ClashDir "profiles.yaml"
$profilesDir = Join-Path $ClashDir "profiles"
if (-not (Test-Path -LiteralPath $profilesYaml)) {
  throw "profiles.yaml not found: $profilesYaml"
}

$profile = Get-CurrentProfileBlock -ProfilesYaml $profilesYaml
$proxiesUid = Get-OptionUid -ProfileBlock $profile.Text -Name "proxies"
$groupsUid = Get-OptionUid -ProfileBlock $profile.Text -Name "groups"
$rulesUid = Get-OptionUid -ProfileBlock $profile.Text -Name "rules"

$proxiesFile = Join-Path $profilesDir (Get-ItemFile -ProfilesYaml $profilesYaml -Uid $proxiesUid)
$groupsFile = Join-Path $profilesDir (Get-ItemFile -ProfilesYaml $profilesYaml -Uid $groupsUid)
$rulesFile = Join-Path $profilesDir (Get-ItemFile -ProfilesYaml $profilesYaml -Uid $rulesUid)

# Pull full JMS node list from raw subscription. Converter (wcc/ACL4SSR) often
# drops VLESS Reality nodes, leaving only SS in the remote profile.
$profileFileName = Get-ItemFile -ProfilesYaml $profilesYaml -Uid $profile.Uid
$remoteNames = Get-RemoteProfileProxyNames -ProfilesDir $profilesDir -ProfileFileName $profileFileName
$subUrl = Get-RawJmsSubscriptionUrl -ProfileUrl (Get-ProfileSubscriptionUrl -ProfileBlock $profile.Text)
$jmsNodes = @(Get-JmsNodesFromSubscription -SubscriptionUrl $subUrl)
if ($jmsNodes.Count -gt 0) {
  Write-Host "JMS subscription nodes: $($jmsNodes.Count) (raw sub)"
} else {
  Write-Host "JMS subscription nodes: 0 (will use names already present in clash-verge.yaml)"
}

if ([string]::IsNullOrWhiteSpace($BootstrapNode)) {
  $BootstrapNode = Get-SelectedBootstrapFromProfile -ProfileBlock $profile.Text
}
if ([string]::IsNullOrWhiteSpace($BootstrapNode) -or $BootstrapNode -notlike "JMS-*") {
  # Proxies group often holds stale ACL names like "HK"; ignore non-JMS picks.
  $maybe = Get-SelectedProxy -ProfileBlock $profile.Text
  if ($maybe -like "JMS-*") {
    $BootstrapNode = $maybe
  }
}
if ([string]::IsNullOrWhiteSpace($BootstrapNode) -or $BootstrapNode -notlike "JMS-*") {
  if ($jmsNodes.Count -gt 0) {
    $BootstrapNode = $jmsNodes[0].Name
  } else {
    $generated = @(Get-GeneratedProxyNames -ClashDir $ClashDir)
    if ($generated.Count -gt 0) {
      $BootstrapNode = $generated[0]
    }
  }
}
if ([string]::IsNullOrWhiteSpace($BootstrapNode)) {
  throw "Cannot determine bootstrap JMS node. Pass -BootstrapNode explicitly."
}

$bootstrapCandidates = New-Object System.Collections.Generic.List[string]
$bootstrapCandidates.Add($BootstrapNode)
foreach ($node in $jmsNodes) {
  if (-not $bootstrapCandidates.Contains($node.Name)) {
    $bootstrapCandidates.Add($node.Name)
  }
}
foreach ($candidate in Get-GeneratedProxyNames -ClashDir $ClashDir) {
  if ($candidate -eq $BootstrapGroupName -or $candidate -eq $FinalGroupName -or $candidate -eq $NodeName) {
    continue
  }
  if (-not $bootstrapCandidates.Contains($candidate)) {
    $bootstrapCandidates.Add($candidate)
  }
}

$hostValue = Escape-YamlSingleQuoted $secret["PROXY_HOST"]
$userValue = Escape-YamlSingleQuoted $secret["PROXY_USER"]
$passValue = Escape-YamlSingleQuoted $secret["PROXY_PASS"]
$portValue = if ($ProxyMode -eq "socks5") { $secret["PROXY_SOCKS_PORT"] } else { $secret["PROXY_HTTP_PORT"] }

foreach ($path in @($proxiesFile, $groupsFile, $rulesFile)) {
  Backup-File -Path $path
}

# Proxies enhancement: inject raw-sub VLESS (converters drop or later re-add it),
# delete those names from the remote list first so a subscription refresh cannot
# merge two copies of the same node, then append IPRoyal-Static.
$proxyLines = New-Object System.Collections.Generic.List[string]
$proxyLines.Add("# Managed by scripts/install-clash-iproyal-chain.ps1")
$proxyLines.Add("# Injects JMS nodes dropped by subscription converters (e.g. VLESS Reality).")
$proxyLines.Add("# Final exit identity is fixed. Do not add fallback nodes here.")
$proxyLines.Add("prepend: []")
$proxyLines.Add("append:")

$injected = 0
$deleteNames = New-Object System.Collections.Generic.List[string]
$seenInject = @{}
foreach ($node in $jmsNodes) {
  if ($seenInject.ContainsKey($node.Name)) {
    continue
  }
  $alreadyRemote = $remoteNames.ContainsKey($node.Name)
  # SS from the converter can stay as-is. VLESS is always taken from the raw
  # subscription: converters historically dropped it, then later reintroduced
  # the same names and merged into duplicate-name failures.
  if ($node.Kind -eq "ss" -and $alreadyRemote) {
    continue
  }
  if ($node.Kind -ne "ss" -and $node.Kind -ne "vless") {
    continue
  }
  $injected++
  $seenInject[$node.Name] = $true
  if (-not $deleteNames.Contains($node.Name)) {
    $deleteNames.Add($node.Name)
  }
  if ($node.Kind -eq "ss") {
    $proxyLines.Add("- name: $(Escape-YamlSingleQuoted $node.Name)")
    $proxyLines.Add("  type: ss")
    $proxyLines.Add("  server: $($node.Server)")
    $proxyLines.Add("  port: $($node.Port)")
    $proxyLines.Add("  cipher: $($node.Cipher)")
    $proxyLines.Add("  password: $(Escape-YamlSingleQuoted $node.Password)")
    $proxyLines.Add("  tfo: false")
  } elseif ($node.Kind -eq "vless") {
    $proxyLines.Add("- name: $(Escape-YamlSingleQuoted $node.Name)")
    $proxyLines.Add("  type: vless")
    $proxyLines.Add("  server: $($node.Server)")
    $proxyLines.Add("  port: $($node.Port)")
    $proxyLines.Add("  uuid: $($node.Uuid)")
    $proxyLines.Add("  tls: true")
    if (-not [string]::IsNullOrWhiteSpace($node.Flow)) {
      $proxyLines.Add("  flow: $($node.Flow)")
    }
    $proxyLines.Add("  skip-cert-verify: true")
    if (-not [string]::IsNullOrWhiteSpace($node.Sni)) {
      $proxyLines.Add("  servername: $($node.Sni)")
    }
    if (-not [string]::IsNullOrWhiteSpace($node.Fp)) {
      $proxyLines.Add("  client-fingerprint: $($node.Fp)")
    }
    if (-not [string]::IsNullOrWhiteSpace($node.Pbk)) {
      $proxyLines.Add("  reality-opts:")
      $proxyLines.Add("    public-key: $($node.Pbk)")
      if (-not [string]::IsNullOrWhiteSpace($node.Sid)) {
        $proxyLines.Add("    short-id: $($node.Sid)")
      }
    }
    $proxyLines.Add("  tfo: false")
  }
}

$proxyLines.Add("- name: $NodeName")
$proxyLines.Add("  type: $ProxyMode")
$proxyLines.Add("  server: $hostValue")
$proxyLines.Add("  port: $portValue")
$proxyLines.Add("  username: $userValue")
$proxyLines.Add("  password: $passValue")
$proxyLines.Add("  dialer-proxy: $BootstrapGroupName")
$proxyLines.Add("  tfo: false")
if ($deleteNames.Count -eq 0) {
  $proxyLines.Add("delete: []")
} else {
  $proxyLines.Add("delete:")
  foreach ($name in $deleteNames) {
    $proxyLines.Add("- $(Escape-YamlSingleQuoted $name)")
  }
}
Write-Lines -Path $proxiesFile -Lines @($proxyLines)
Write-Host "Injected JMS nodes (delete+append to avoid duplicate names): $injected"

$groupLines = @(
  "# Managed by scripts/install-clash-iproyal-chain.ps1",
  "# JMS-Bootstrap is manual-select only: no automatic fallback or health-check traffic.",
  "prepend:",
  "- name: $BootstrapGroupName",
  "  type: select",
  "  proxies:"
)
foreach ($candidate in $bootstrapCandidates) {
  $groupLines += "  - $(Escape-YamlSingleQuoted $candidate)"
}
$groupLines += @(
  "- name: $FinalGroupName",
  "  type: select",
  "  proxies:",
  "  - $NodeName",
  "append: []",
  "delete: []"
)
Write-Lines -Path $groupsFile -Lines $groupLines

if (-not $NoRules) {
  Write-Lines -Path $rulesFile -Lines @(
    "# Managed by scripts/install-clash-iproyal-chain.ps1",
    "# Only identity-sensitive AI/test domains use the static ISP path.",
    "prepend:",
    "- PROCESS-NAME,Devin.exe,$FinalGroupName",
    "- PROCESS-NAME,devin.exe,$FinalGroupName",
    "- DOMAIN-SUFFIX,devin.ai,$FinalGroupName",
    "- PROCESS-NAME,claude.exe,$FinalGroupName",
    "- PROCESS-NAME,Claude.exe,$FinalGroupName",
    "- PROCESS-NAME,ChatGPT.exe,$FinalGroupName",
    "- PROCESS-NAME,codex.exe,$FinalGroupName",
    "- PROCESS-NAME,Codex.exe,$FinalGroupName",
    "- DOMAIN-SUFFIX,claude.ai,$FinalGroupName",
    "- DOMAIN-SUFFIX,anthropic.com,$FinalGroupName",
    "- DOMAIN-SUFFIX,anthropic.ai,$FinalGroupName",
    "- DOMAIN,browser-intake-us5-datadoghq.com,$FinalGroupName",
    "- DOMAIN-SUFFIX,openai.com,$FinalGroupName",
    "- DOMAIN-SUFFIX,chatgpt.com,$FinalGroupName",
    "- DOMAIN-SUFFIX,codexapis.com,$FinalGroupName",
    "- DOMAIN-SUFFIX,oaistatic.com,$FinalGroupName",
    "- DOMAIN-SUFFIX,oaiusercontent.com,$FinalGroupName",
    "- DOMAIN-SUFFIX,ping0.cc,$FinalGroupName",
    "- DOMAIN-SUFFIX,ipinfo.io,$FinalGroupName",
    "append: []",
    "delete: []"
  )
}

Write-Host "Installed Clash Verge Rev persistent chain config."
Write-Host "Current profile: $($profile.Uid)"
Write-Host "Proxy enhancement: $proxiesFile"
Write-Host "Group enhancement: $groupsFile"
Write-Host "Rule enhancement: $rulesFile"
Write-Host "Final node: $NodeName via $BootstrapGroupName"
