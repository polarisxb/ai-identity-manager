$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
& (Join-Path $PSScriptRoot "start-keeper.ps1")

$pacPath = Join-Path $Root "identity-keeper-ai.pac"
$pacUrl = "file:///" + ($pacPath -replace "\\", "/")
$regPath = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings"

Set-ItemProperty -Path $regPath -Name AutoConfigURL -Value $pacUrl
Set-ItemProperty -Path $regPath -Name ProxyEnable -Value 0

Write-Host "Windows PAC enabled: $pacUrl"
Write-Host "Only AI domains in identity-keeper-ai.pac use 127.0.0.1:18080"
