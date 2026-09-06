# Snapshot the config that git does NOT track, into recovery-kit/ for offsite backup.
#
# git already versions: source, docs, scripts, the install recipe.
# git does NOT (by design): proxy-secret.local (the secret) + Clash Verge's live
# profile enhancement files (they live in %APPDATA%). This script copies those, so
# a full backup = the git repo + the recovery-kit/ folder.
#
# Usage:  powershell -ExecutionPolicy Bypass -File .\scripts\backup-config.ps1
# Then copy the whole recovery-kit/ folder to cloud/USB. It contains the secret —
# keep it private. See RECOVERY.md.

param(
  [string]$ClashDir   = "$env:APPDATA\io.github.clash-verge-rev.clash-verge-rev",
  [string]$SecretFile = "$PSScriptRoot\..\proxy-secret.local",
  [string]$OutDir     = "$PSScriptRoot\..\recovery-kit"
)

$ErrorActionPreference = "Stop"
$profilesYaml = Join-Path $ClashDir "profiles.yaml"
$profilesDir  = Join-Path $ClashDir "profiles"
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

# 1) the secret (irreplaceable, not in git)
if (Test-Path -LiteralPath $SecretFile) {
  Copy-Item -LiteralPath $SecretFile -Destination (Join-Path $OutDir "proxy-secret.local") -Force
}

# 2) the current profile's bound enhancement files (proxies/groups/rules/merge/script)
$copied = New-Object System.Collections.Generic.List[string]
if (Test-Path -LiteralPath $profilesYaml) {
  $text  = Get-Content -LiteralPath $profilesYaml -Raw
  $cur   = [regex]::Match($text, "(?m)^current:\s*(\S+)").Groups[1].Value
  $block = [regex]::Match($text, "(?ms)^- uid:\s*$([regex]::Escape($cur))\s*\r?\n.*?(?=^- uid:|\z)").Value
  foreach ($opt in @("proxies", "groups", "rules", "merge", "script")) {
    $m = [regex]::Match($block, "(?m)^\s{4}$($opt):\s*(\S+)")
    if (-not $m.Success) { continue }
    $uid  = $m.Groups[1].Value
    $item = [regex]::Match($text, "(?ms)^- uid:\s*$([regex]::Escape($uid))\s*\r?\n.*?(?=^- uid:|\z)").Value
    $fm   = [regex]::Match($item, "(?m)^\s{2}file:\s*(.+?)\s*$")
    if (-not $fm.Success) { continue }
    $src = Join-Path $profilesDir $fm.Groups[1].Value.Trim()
    if (Test-Path -LiteralPath $src) {
      $dst = Join-Path $OutDir ("profile-$opt-" + (Split-Path $src -Leaf))
      Copy-Item -LiteralPath $src -Destination $dst -Force
      $copied.Add("$opt  ->  $(Split-Path $src -Leaf)")
    }
  }
}

# 3) subscription URLs (needed to re-import base profiles on a fresh machine)
$urls = @()
if (Test-Path -LiteralPath $profilesYaml) {
  $urls = [regex]::Matches((Get-Content -LiteralPath $profilesYaml -Raw), "https?://[^\s'`"]+") |
          ForEach-Object { $_.Value } | Sort-Object -Unique
}

# 4) manifest
$m = New-Object System.Collections.Generic.List[string]
$m.Add("AI Identity Manager - recovery snapshot")
$m.Add("taken:           $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')")
$m.Add("current profile: $cur")
$m.Add("")
$m.Add("copied enhancement files (paste content back into the new profile's items):")
foreach ($c in $copied) { $m.Add("  $c") }
$m.Add("")
$m.Add("subscription URLs (re-import these base profiles on a new machine):")
foreach ($u in $urls) { $m.Add("  $u") }
$m.Add("")
$m.Add("restore procedure: see RECOVERY.md")
Set-Content -LiteralPath (Join-Path $OutDir "MANIFEST.txt") -Value $m -Encoding UTF8

Write-Host "Recovery kit written to: $OutDir"
Write-Host "Files: proxy-secret.local + $($copied.Count) profile enhancement file(s) + MANIFEST.txt"
Write-Host "Copy the whole recovery-kit/ folder to cloud/USB. It contains the secret - keep it private."
