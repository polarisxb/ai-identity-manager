param(
    [ValidateSet("chrome", "edge")]
    [string]$Browser = "edge",
    [switch]$UseExistingProfile,
    [switch]$AllTrafficStatic,
    [switch]$RestartBrowser,
    [switch]$AllowWebRTC,
    [switch]$StrictIdentity,
    [switch]$DisableOtherExtensions
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
& (Join-Path $PSScriptRoot "start-keeper.ps1")

if ($Browser -eq "chrome") {
    $candidates = @(
        "$env:ProgramFiles\Google\Chrome\Application\chrome.exe",
        "${env:ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe"
    )
} else {
    $candidates = @(
        "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe",
        "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe"
    )
}

$exe = $candidates | Where-Object { Test-Path $_ } | Select-Object -First 1
if (!$exe) {
    throw "$Browser executable not found"
}

if ($StrictIdentity) {
    $UseExistingProfile = $false
    $AllTrafficStatic = $true
    $RestartBrowser = $true
}

if ($RestartBrowser) {
    if ($Browser -eq "chrome") {
        Get-Process chrome -ErrorAction SilentlyContinue | Stop-Process
    } else {
        Get-Process msedge -ErrorAction SilentlyContinue | Stop-Process
    }
    Start-Sleep -Seconds 2
}

$pacPath = Join-Path $Root "identity-keeper-ai.pac"
$pacUrl = "file:///" + ($pacPath -replace "\\", "/")

if ($AllTrafficStatic) {
    $proxyArg = "--proxy-server=http://127.0.0.1:18080"
} else {
    $proxyArg = "--proxy-pac-url=$pacUrl"
}

$args = @(
    $proxyArg,
    "--disable-webrtc",
    "--disable-webrtc-multiple-routes",
    "--force-webrtc-ip-handling-policy=disable_non_proxied_udp",
    "https://claude.ai"
)

if (!$AllowWebRTC) {
    $extensionDir = Join-Path $Root "extensions\block-webrtc"
    $args = @("--load-extension=$extensionDir") + $args
    if ($StrictIdentity -or $DisableOtherExtensions) {
        $args = @("--disable-extensions-except=$extensionDir") + $args
    }
}

if (!$UseExistingProfile) {
    $profileDir = Join-Path $Root "browser-profile\claude-$Browser"
    New-Item -ItemType Directory -Force -Path $profileDir | Out-Null
    $args = @("--user-data-dir=$profileDir") + $args
}

Start-Process -FilePath $exe -ArgumentList $args
