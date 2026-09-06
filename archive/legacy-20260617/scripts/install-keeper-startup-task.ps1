$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $PSScriptRoot
$Exe = Join-Path $Root "identity-keeper.exe"
$Config = Join-Path $Root "proxy-secret.local"
$TaskName = "IdentityKeeper"

if (!(Test-Path $Exe)) {
    Push-Location $Root
    try {
        go build -o identity-keeper.exe ./cmd/identity-keeper
    } finally {
        Pop-Location
    }
}

if (!(Test-Path $Config)) {
    throw "Config file not found: $Config"
}

function Install-SchtasksFallback {
    $taskRun = "`"$Exe`" --config `"$Config`" --listen 127.0.0.1:18080"
    & schtasks.exe /Create /TN $TaskName /SC ONLOGON /TR $taskRun /RL LIMITED /F | Out-Host
    if ($LASTEXITCODE -ne 0) {
        throw "schtasks.exe failed with exit code $LASTEXITCODE"
    }

    & schtasks.exe /Run /TN $TaskName | Out-Host
    if ($LASTEXITCODE -ne 0) {
        throw "schtasks.exe could not start $TaskName, exit code $LASTEXITCODE"
    }

    Write-Host "Installed and started scheduled task via schtasks.exe: $TaskName"
}

function Install-RunKeyFallback {
    $runPath = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"
    $value = "`"$Exe`" --config `"$Config`" --listen 127.0.0.1:18080"
    New-Item -Path $runPath -Force | Out-Null
    New-ItemProperty -Path $runPath -Name $TaskName -Value $value -PropertyType String -Force | Out-Null

    if (!(Get-Process identity-keeper -ErrorAction SilentlyContinue)) {
        Start-Process -FilePath $Exe `
            -ArgumentList @("--config", "`"$Config`"", "--listen", "127.0.0.1:18080") `
            -WorkingDirectory $Root `
            -WindowStyle Hidden | Out-Null
    }

    Write-Host "Installed HKCU Run startup entry: $TaskName"
}

try {
    $action = New-ScheduledTaskAction `
        -Execute $Exe `
        -Argument "--config `"$Config`" --listen 127.0.0.1:18080" `
        -WorkingDirectory $Root

    $trigger = New-ScheduledTaskTrigger -AtLogOn
    $principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Limited
    $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit ([TimeSpan]::Zero)

    Register-ScheduledTask `
        -TaskName $TaskName `
        -Action $action `
        -Trigger $trigger `
        -Principal $principal `
        -Settings $settings `
        -Force | Out-Null

    Start-ScheduledTask -TaskName $TaskName

    Write-Host "Installed and started scheduled task: $TaskName"
} catch [Microsoft.Management.Infrastructure.CimException] {
    Write-Host "Register-ScheduledTask unavailable, falling back to schtasks.exe: $($_.Exception.Message)"
    try {
        Install-SchtasksFallback
    } catch {
        Write-Host "schtasks.exe unavailable, falling back to HKCU Run: $($_.Exception.Message)"
        Install-RunKeyFallback
    }
}
