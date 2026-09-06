$ErrorActionPreference = "Stop"

$TaskName = "IdentityKeeper"
$StartupDir = [Environment]::GetFolderPath("Startup")
$ShortcutPath = Join-Path $StartupDir "IdentityKeeper.lnk"
$RunPath = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"

$task = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
if ($task) {
    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
    Write-Host "Removed scheduled task: $TaskName"
} else {
    Write-Host "Scheduled task not found: $TaskName"
}

if (Test-Path -LiteralPath $ShortcutPath) {
    Remove-Item -LiteralPath $ShortcutPath -Force
    Write-Host "Removed old startup shortcut: $ShortcutPath"
}

if (Get-ItemProperty -Path $RunPath -Name $TaskName -ErrorAction SilentlyContinue) {
    Remove-ItemProperty -Path $RunPath -Name $TaskName -Force
    Write-Host "Removed HKCU Run startup entry: $TaskName"
}
