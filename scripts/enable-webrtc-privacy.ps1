$ErrorActionPreference = "Stop"

$policyValue = "disable_non_proxied_udp"
$paths = @(
    "HKCU:\Software\Policies\Microsoft\Edge",
    "HKCU:\Software\Policies\Google\Chrome"
)

foreach ($path in $paths) {
    New-Item -Path $path -Force | Out-Null
    Set-ItemProperty -Path $path -Name WebRtcIPHandlingPolicy -Value $policyValue -Type String
}

Write-Host "WebRTC IP handling policy enabled for Edge and Chrome: $policyValue"
Write-Host "Restart browser processes for the policy to take effect."
