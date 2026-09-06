package killswitch

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestComputePlanAddsMissingRemovesStaleKeepsMatch(t *testing.T) {
	targets := []Target{
		{Kind: KindAppx, Label: "Claude (store)", PackageSID: "S-1-15-2-NEW"},
		{Kind: KindProgram, Label: "Claude (standalone)", Program: `C:\a\app-2\claude.exe`},
	}
	existing := []Rule{
		{Name: RulePrefix + " UDP Claude (store)", PackageSID: "S-1-15-2-OLD"},
		{Name: RulePrefix + " UDP Claude (standalone)", Program: `C:\a\app-2\claude.exe`},
		{Name: RulePrefix + " UDP Cursor", Program: `C:\x\cursor.exe`},
	}

	p := ComputePlan(targets, existing)

	if len(p.Add) != 1 || p.Add[0].Kind != KindAppx {
		t.Fatalf("Add=%+v", p.Add)
	}
	if !contains(p.RemoveNames, RulePrefix+" UDP Claude (store)") || !contains(p.RemoveNames, RulePrefix+" UDP Cursor") {
		t.Fatalf("Remove=%+v", p.RemoveNames)
	}
	if !contains(p.KeepNames, RulePrefix+" UDP Claude (standalone)") {
		t.Fatalf("Keep=%+v", p.KeepNames)
	}
}

func contains(xs []string, x string) bool {
	for _, e := range xs {
		if e == x {
			return true
		}
	}
	return false
}

type fakeRunner struct {
	outputs map[string]string
	ran     []string
	err     error
}

func (f *fakeRunner) Run(ctx context.Context, script string) (string, error) {
	f.ran = append(f.ran, script)
	if f.err != nil {
		return "", f.err
	}
	for k, v := range f.outputs {
		if strings.Contains(script, k) {
			return v, nil
		}
	}
	return "", nil
}

func TestResolveParsesTargets(t *testing.T) {
	f := &fakeRunner{outputs: map[string]string{
		"Get-AppxPackage": "appx|S-1-15-2-X|Claude (store)\nprogram|C:\\a\\claude.exe|Claude (standalone)\n",
	}}
	ks := New(Config{Appx: []string{"*Claude*"}, Globs: []string{`C:\a\claude.exe`}}, f)

	ts, err := ks.Resolve(context.Background())

	if err != nil {
		t.Fatalf("Resolve err=%v", err)
	}
	if len(ts) != 2 || ts[0].Kind != KindAppx || ts[0].PackageSID != "S-1-15-2-X" {
		t.Fatalf("targets=%+v", ts)
	}
	if ts[1].Kind != KindProgram || ts[1].Program != `C:\a\claude.exe` {
		t.Fatalf("program target=%+v", ts[1])
	}
}

func TestApplyAddsMissingRemovesStale(t *testing.T) {
	f := &fakeRunner{outputs: map[string]string{
		"Get-AppxPackage":         "appx|S-NEW|Claude (store)\n",
		"AI-Identity-KillSwitch*": "AI-Identity-KillSwitch UDP Claude (store)|S-OLD|\n",
	}}
	ks := New(Config{Appx: []string{"*Claude*"}}, f)

	if err := ks.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(f.ran, "\n")
	if !strings.Contains(joined, "Remove-NetFirewallRule") || !strings.Contains(joined, "-Package 'S-NEW'") {
		t.Fatalf("ran=%v", f.ran)
	}
	if strings.Contains(joined, "Register-ScheduledTask") {
		t.Fatalf("apply without Persist.Enabled should not install refresh task: %v", f.ran)
	}
}

func TestApplyInstallsPersistTask(t *testing.T) {
	f := &fakeRunner{outputs: map[string]string{
		"Get-AppxPackage":         "appx|S-NEW|Claude (store)\n",
		"AI-Identity-KillSwitch*": "AI-Identity-KillSwitch UDP Claude (store)|S-NEW|\n",
	}}
	ks := New(Config{
		Appx: []string{"*Claude*"},
		Persist: PersistConfig{
			Enabled:    true,
			Executable: `C:\proxy\ai-identity-manager.exe`,
			Arguments:  "killswitch apply --no-persist --secret C:\\proxy\\proxy-secret.local",
			WorkingDir: `C:\proxy`,
		},
	}, f)

	if err := ks.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(f.ran, "\n")
	for _, want := range []string{
		"Register-ScheduledTask",
		"AI-Identity-KillSwitch-Refresh",
		"-RunLevel Highest",
		"killswitch apply --no-persist",
		`C:\proxy\ai-identity-manager.exe`,
		"wscript.exe",
		"killswitch-refresh.vbs",
		", 0, True",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("persist install missing %q:\n%s", want, joined)
		}
	}
}

func TestStatusReportsCoverage(t *testing.T) {
	f := &fakeRunner{outputs: map[string]string{
		"Get-AppxPackage":         "appx|S-OK|Claude (store)\nprogram|C:\\a\\claude.exe|Claude (standalone)\n",
		"AI-Identity-KillSwitch*": "AI-Identity-KillSwitch UDP Claude (store)|S-OK|\n",
	}}
	ks := New(Config{Appx: []string{"*Claude*"}, Globs: []string{`C:\a\claude.exe`}}, f)

	status, err := ks.Status(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	if status.Covered != 1 || status.Missing != 1 || status.AllCovered {
		t.Fatalf("status=%+v", status)
	}
	if status.PersistInstalled {
		t.Fatalf("persist should be missing without scheduled task output: %+v", status)
	}
}

func TestStatusReportsPersistInstalled(t *testing.T) {
	f := &fakeRunner{outputs: map[string]string{
		"Get-AppxPackage":         "appx|S-OK|Claude (store)\n",
		"AI-Identity-KillSwitch*": "AI-Identity-KillSwitch UDP Claude (store)|S-OK|\n",
		"Get-ScheduledTask":       "persist|installed\n",
	}}
	ks := New(Config{Appx: []string{"*Claude*"}}, f)

	status, err := ks.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.AllCovered || !status.PersistInstalled || status.PersistTaskName != RefreshTaskName {
		t.Fatalf("status=%+v", status)
	}
}

func TestRemoveDeletesAllManagedRules(t *testing.T) {
	f := &fakeRunner{outputs: map[string]string{
		"AI-Identity-KillSwitch*": "AI-Identity-KillSwitch UDP Claude (store)|S-OK|\nAI-Identity-KillSwitch UDP Cursor||C:\\x\\cursor.exe\n",
	}}
	ks := New(Config{}, f)

	if err := ks.Remove(context.Background()); err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(f.ran, "\n")
	if strings.Count(joined, "Remove-NetFirewallRule") != 2 {
		t.Fatalf("ran=%v", f.ran)
	}
	if !strings.Contains(joined, "Unregister-ScheduledTask") || !strings.Contains(joined, RefreshTaskName) {
		t.Fatalf("remove should unregister persist task: %v", f.ran)
	}
}

func TestApplyPropagatesRunnerError(t *testing.T) {
	ks := New(Config{Appx: []string{"*Claude*"}}, &fakeRunner{err: errors.New("access denied")})

	err := ks.Apply(context.Background())

	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("err=%v", err)
	}
}
