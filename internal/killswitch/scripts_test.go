package killswitch

import (
	"strings"
	"testing"
)

func TestAddRuleScriptBuildsAppxUDPBlockRule(t *testing.T) {
	script := addRuleScript(Target{
		Kind:       KindAppx,
		Label:      "Claude (store)",
		PackageSID: "S-1-15-2-123",
	})

	for _, want := range []string{
		"New-NetFirewallRule",
		"-DisplayName 'AI-Identity-KillSwitch UDP Claude (store)'",
		"-Direction Outbound",
		"-Package 'S-1-15-2-123'",
		"-Protocol UDP",
		"-Action Block",
		"-Profile Any",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}
}

func TestAddRuleScriptBuildsProgramUDPBlockRuleAndQuotesLabel(t *testing.T) {
	script := addRuleScript(Target{
		Kind:    KindProgram,
		Label:   "Claude's standalone",
		Program: `C:\a\app-2\claude.exe`,
	})

	for _, want := range []string{
		"-DisplayName 'AI-Identity-KillSwitch UDP Claude''s standalone'",
		"-Program 'C:\\a\\app-2\\claude.exe'",
		"-Protocol UDP",
		"-Action Block",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}
}

func TestResolveAndListScriptsContainExpectedPowerShellFragments(t *testing.T) {
	resolve := resolveScript([]string{"*Claude*"}, []string{`C:\a\app-*\claude.exe`})
	for _, want := range []string{"Get-AppxPackage", "AppContainer", "appx|", "$sid = Split-Path", "program|", `C:\a\app-*\claude.exe`} {
		if !strings.Contains(resolve, want) {
			t.Fatalf("resolve script missing %q:\n%s", want, resolve)
		}
	}

	list := listScript()
	for _, want := range []string{"Get-NetFirewallRule", "AI-Identity-KillSwitch*", "Get-NetFirewallApplicationFilter", "$app.Package"} {
		if !strings.Contains(list, want) {
			t.Fatalf("list script missing %q:\n%s", want, list)
		}
	}
}

func TestInstallPersistScriptRegistersHighestTaskWithNoPersistApply(t *testing.T) {
	script := installPersistScript(PersistConfig{
		Executable:      `C:\proxy\ai-identity-manager.exe`,
		Arguments:       `killswitch apply --no-persist --secret C:\proxy\proxy-secret.local`,
		WorkingDir:      `C:\proxy`,
		IntervalMinutes: 5,
	})
	for _, want := range []string{
		"Register-ScheduledTask",
		"$name = 'AI-Identity-KillSwitch-Refresh'",
		"-TaskName $name",
		"-RunLevel Highest",
		"-AtLogOn",
		"New-TimeSpan -Minutes 5",
		`killswitch apply --no-persist --secret C:\proxy\proxy-secret.local`,
		"killswitch-refresh.vbs",
		"wscript.exe",
		`, 0, True`,
		`-Execute $wscript`,
		`-WorkingDirectory $wd`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("install persist script missing %q:\n%s", want, script)
		}
	}
}

func TestPersistStatusAndRemoveScriptsUseRefreshTaskName(t *testing.T) {
	status := persistStatusScript()
	if !strings.Contains(status, "Get-ScheduledTask") || !strings.Contains(status, RefreshTaskName) {
		t.Fatalf("status script=%s", status)
	}
	remove := removePersistScript()
	if !strings.Contains(remove, "Unregister-ScheduledTask") || !strings.Contains(remove, RefreshTaskName) {
		t.Fatalf("remove script=%s", remove)
	}
}

func TestParsePersistInstalled(t *testing.T) {
	if !parsePersistInstalled("persist|installed\n") {
		t.Fatal("expected installed")
	}
	if parsePersistInstalled("persist|missing\n") {
		t.Fatal("expected missing")
	}
}
