$ErrorActionPreference = "Stop"
$regPath = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings"

Remove-ItemProperty -Path $regPath -Name AutoConfigURL -ErrorAction SilentlyContinue
Set-ItemProperty -Path $regPath -Name ProxyEnable -Value 0

Write-Host "Windows proxy/PAC disabled"
