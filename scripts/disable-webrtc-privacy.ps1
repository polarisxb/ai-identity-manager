$ErrorActionPreference = "Stop"

$paths = @(
    "HKCU:\Software\Policies\Microsoft\Edge",
    "HKCU:\Software\Policies\Google\Chrome"
)

foreach ($path in $paths) {
    Remove-ItemProperty -Path $path -Name WebRtcIPHandlingPolicy -ErrorAction SilentlyContinue
}

Write-Host "WebRTC IP handling policy removed for Edge and Chrome."
Write-Host "Restart browser processes for the change to take effect."
