param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$ClaudeArgs
)

$ErrorActionPreference = "Stop"
& (Join-Path $PSScriptRoot "start-keeper.ps1")

$env:HTTP_PROXY = "http://127.0.0.1:18080"
$env:HTTPS_PROXY = "http://127.0.0.1:18080"

$cmd = Get-Command claude -ErrorAction SilentlyContinue
if (!$cmd) {
    throw "claude command not found in PATH"
}

& $cmd.Source @ClaudeArgs
exit $LASTEXITCODE
