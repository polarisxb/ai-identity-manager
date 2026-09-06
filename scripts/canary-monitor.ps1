# Launcher for the AI-Identity-Monitor scheduled task (login-start, restart-on-crash).
# Runs the Go monitor --watch with a per-day log (auto-rotates daily).
# Infra only — the actual monitoring logic lives in the Go binary, not here.
Set-Location -LiteralPath 'C:\Users\32304\Desktop\proxy'
$log = "canary-$(Get-Date -Format 'yyyyMMdd').log"
& '.\ai-identity-manager.exe' monitor --watch --interval 10s >> $log
