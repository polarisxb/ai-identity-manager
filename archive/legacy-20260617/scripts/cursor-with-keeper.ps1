param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$CursorArgs
)

$ErrorActionPreference = "Stop"
& (Join-Path $PSScriptRoot "start-keeper.ps1")

$env:HTTP_PROXY = "http://127.0.0.1:18080"
$env:HTTPS_PROXY = "http://127.0.0.1:18080"

$cmd = Get-Command cursor -ErrorAction SilentlyContinue
if (!$cmd) {
    throw "cursor command not found in PATH"
}

& $cmd.Source @CursorArgs
exit $LASTEXITCODE
