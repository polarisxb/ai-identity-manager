package killswitch

import (
	"fmt"
	"strings"
)

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func addRuleScript(t Target) string {
	name := psQuote(ruleName(t))
	if t.Kind == KindAppx {
		return "New-NetFirewallRule -DisplayName " + name + " -Direction Outbound -Package " + psQuote(t.PackageSID) + " -Protocol UDP -Action Block -Profile Any | Out-Null"
	}
	return "New-NetFirewallRule -DisplayName " + name + " -Direction Outbound -Program " + psQuote(t.Program) + " -Protocol UDP -Action Block -Profile Any | Out-Null"
}

func removeRuleScript(name string) string {
	return "Get-NetFirewallRule -DisplayName " + psQuote(name) + " -EA SilentlyContinue | Remove-NetFirewallRule"
}

func resolveScript(appx, globs []string) string {
	var b strings.Builder
	b.WriteString("$ErrorActionPreference = 'Stop'\n")
	b.WriteString("$mappingRoot = 'HKCU:\\Software\\Classes\\Local Settings\\Software\\Microsoft\\Windows\\CurrentVersion\\AppContainer\\Mappings'\n")
	for _, pattern := range appx {
		b.WriteString("$pattern = ")
		b.WriteString(psQuote(pattern))
		b.WriteString("\n")
		b.WriteString("Get-AppxPackage $pattern | ForEach-Object {\n")
		b.WriteString("  $family = $_.PackageFamilyName\n")
		b.WriteString("  $display = if ($_.Name) { $_.Name } else { $family }\n")
		b.WriteString("  $sid = $null\n")
		b.WriteString("  foreach ($mapping in Get-ChildItem $mappingRoot -EA SilentlyContinue) {\n")
		b.WriteString("    $props = Get-ItemProperty $mapping.PSPath -EA SilentlyContinue\n")
		b.WriteString("    if ($props.Moniker -eq $family) { $sid = Split-Path $mapping.PSChildName -Leaf }\n")
		b.WriteString("  }\n")
		b.WriteString("  if ($sid) { Write-Output ('appx|' + $sid + '|' + $display) }\n")
		b.WriteString("}\n")
	}
	for _, glob := range globs {
		b.WriteString("$glob = ")
		b.WriteString(psQuote(glob))
		b.WriteString("\n")
		b.WriteString("Get-ChildItem -Path $glob -File -EA SilentlyContinue | Sort-Object FullName | Select-Object -Last 1 | ForEach-Object {\n")
		b.WriteString("  Write-Output ('program|' + $_.FullName + '|Claude (standalone)')\n")
		b.WriteString("}\n")
	}
	return b.String()
}

func listScript() string {
	return strings.Join([]string{
		"$ErrorActionPreference = 'Stop'",
		"Get-NetFirewallRule -DisplayName 'AI-Identity-KillSwitch*' -EA SilentlyContinue | ForEach-Object {",
		"  $rule = $_",
		"  $program = ''",
		"  $sid = ''",
		"  $app = Get-NetFirewallApplicationFilter -AssociatedNetFirewallRule $rule -EA SilentlyContinue",
		"  if ($app -and $app.Program) { $program = $app.Program }",
		"  if ($app -and $app.Package) { $sid = $app.Package }",
		"  Write-Output ($rule.DisplayName + '|' + $sid + '|' + $program)",
		"}",
	}, "\n")
}

func persistIntervalMinutes(p PersistConfig) int {
	if p.IntervalMinutes > 0 {
		return p.IntervalMinutes
	}
	return DefaultRefreshMinutes
}

func persistStatusScript() string {
	return strings.Join([]string{
		"$ErrorActionPreference = 'Stop'",
		"$task = Get-ScheduledTask -TaskName " + psQuote(RefreshTaskName) + " -EA SilentlyContinue",
		"if ($task) { Write-Output 'persist|installed' } else { Write-Output 'persist|missing' }",
	}, "\n")
}

func removePersistScript() string {
	return "Unregister-ScheduledTask -TaskName " + psQuote(RefreshTaskName) + " -Confirm:$false -EA SilentlyContinue"
}

func installPersistScript(p PersistConfig) string {
	minutes := persistIntervalMinutes(p)
	return strings.Join([]string{
		"$ErrorActionPreference = 'Stop'",
		"$exe = " + psQuote(p.Executable),
		"$arg = " + psQuote(p.Arguments),
		"$wd = " + psQuote(p.WorkingDir),
		"$name = " + psQuote(RefreshTaskName),
		"$userId = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name",
		"$helperDir = Join-Path $wd '.ai-identity'",
		"New-Item -ItemType Directory -Force -Path $helperDir | Out-Null",
		"$helper = Join-Path $helperDir 'killswitch-refresh.vbs'",
		"$exeVbs = $exe.Replace('\"','\"\"')",
		"$argVbs = $arg.Replace('\"','\"\"')",
		"$wdVbs = $wd.Replace('\"','\"\"')",
		"@( ",
		"  'Set sh = CreateObject(\"Wscript.Shell\")'",
		"  ('sh.CurrentDirectory = \"' + $wdVbs + '\"')",
		"  ('sh.Run \"\"\"' + $exeVbs + '\"\" ' + $argVbs + '\", 0, True')",
		") | Set-Content -LiteralPath $helper -Encoding ASCII",
		"$wscript = Join-Path $env:SystemRoot 'System32\\wscript.exe'",
		"$action = New-ScheduledTaskAction -Execute $wscript -Argument ('//nologo \"' + $helper + '\"') -WorkingDirectory $wd",
		"$tLogon = New-ScheduledTaskTrigger -AtLogOn -User $userId",
		fmt.Sprintf("$tRepeat = New-ScheduledTaskTrigger -Once -At ((Get-Date).AddMinutes(1)) -RepetitionInterval (New-TimeSpan -Minutes %d) -RepetitionDuration (New-TimeSpan -Days 3650)", minutes),
		"$principal = New-ScheduledTaskPrincipal -UserId $userId -LogonType Interactive -RunLevel Highest",
		"$settings = New-ScheduledTaskSettingsSet -Hidden -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -ExecutionTimeLimit (New-TimeSpan -Minutes 10)",
		"Register-ScheduledTask -TaskName $name -Action $action -Trigger @($tLogon,$tRepeat) -Principal $principal -Settings $settings -Description 'Refresh AI identity UDP killswitch after Claude Desktop updates' -Force | Out-Null",
	}, "\n")
}

func parsePersistInstalled(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "persist|installed" {
			return true
		}
	}
	return false
}
