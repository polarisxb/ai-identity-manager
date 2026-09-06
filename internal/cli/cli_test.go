package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-identity-manager/internal/controller"
	"ai-identity-manager/internal/identity"
	"ai-identity-manager/internal/identity/session"
	"ai-identity-manager/internal/killswitch"
)

func TestStatusCommandPrintsYellowWhenNoAIHitObserved(t *testing.T) {
	fixture := newCLIFixture(t)
	code := Run([]string{
		"apply",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
		"--bootstrap", "JMS-A",
	}, &fixture.stdout, &fixture.stderr)
	if code != 0 {
		t.Fatalf("apply exit code = %d, stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	fixture.stdout.Reset()
	fixture.stderr.Reset()

	code = Run([]string{
		"status",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 1 {
		t.Fatalf("exit code = %d, stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	if !strings.Contains(fixture.stdout.String(), "YELLOW") {
		t.Fatalf("status output missing YELLOW:\n%s", fixture.stdout.String())
	}
}

func TestApplyCommandWritesCurrentProfileEnhancements(t *testing.T) {
	fixture := newCLIFixture(t)
	code := Run([]string{
		"apply",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
		"--bootstrap", "JMS-A",
		"--bootstrap", "JMS-B",
	}, &fixture.stdout, &fixture.stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	proxies := readFixtureFile(t, filepath.Join(fixture.clashDir, "profiles", "proxies-enhancement.yaml"))
	groups := readFixtureFile(t, filepath.Join(fixture.clashDir, "profiles", "groups-enhancement.yaml"))
	rules := readFixtureFile(t, filepath.Join(fixture.clashDir, "profiles", "rules-enhancement.yaml"))

	for _, want := range []string{"IPRoyal-Static", "dialer-proxy: JMS-Bootstrap"} {
		if !strings.Contains(proxies, want) {
			t.Fatalf("proxies enhancement missing %q:\n%s", want, proxies)
		}
	}
	for _, want := range []string{"JMS-Bootstrap", "'JMS-A'", "'JMS-B'", "AI-Static"} {
		if !strings.Contains(groups, want) {
			t.Fatalf("groups enhancement missing %q:\n%s", want, groups)
		}
	}
	if !strings.Contains(rules, "claude.exe,AI-Static") || !strings.Contains(rules, "openai.com,AI-Static") {
		t.Fatalf("rules enhancement missing AI rules:\n%s", rules)
	}
	if strings.Contains(fixture.stdout.String(), "static-pass") {
		t.Fatalf("stdout leaked secret:\n%s", fixture.stdout.String())
	}
}

func TestApplyDryRunDoesNotModifyEnhancementFiles(t *testing.T) {
	fixture := newCLIFixture(t)
	proxiesPath := filepath.Join(fixture.clashDir, "profiles", "proxies-enhancement.yaml")
	groupsPath := filepath.Join(fixture.clashDir, "profiles", "groups-enhancement.yaml")
	rulesPath := filepath.Join(fixture.clashDir, "profiles", "rules-enhancement.yaml")
	before := map[string]string{
		proxiesPath: readFixtureFile(t, proxiesPath),
		groupsPath:  readFixtureFile(t, groupsPath),
		rulesPath:   readFixtureFile(t, rulesPath),
	}

	code := Run([]string{
		"apply",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
		"--bootstrap", "JMS-A",
		"--dry-run",
	}, &fixture.stdout, &fixture.stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	for path, want := range before {
		if got := readFixtureFile(t, path); got != want {
			t.Fatalf("dry-run modified %s\nbefore:\n%s\nafter:\n%s", path, want, got)
		}
	}
	if !strings.Contains(fixture.stdout.String(), "planned:") {
		t.Fatalf("dry-run output missing planned writes:\n%s", fixture.stdout.String())
	}
	if strings.Contains(fixture.stdout.String(), "static-pass") {
		t.Fatalf("dry-run leaked secret:\n%s", fixture.stdout.String())
	}
}

func TestDoctorCommandPrintsLocalChecks(t *testing.T) {
	fixture := newCLIFixture(t)
	applyFixtureChain(t, &fixture)
	withFakeKillRunner(t, &fakeKillRunner{outputs: map[string]string{
		"Get-AppxPackage":         "appx|S-OK|Claude (store)\nprogram|C:\\a\\claude.exe|Claude (standalone)\n",
		"AI-Identity-KillSwitch*": "AI-Identity-KillSwitch UDP Claude (store)|S-OK|\n",
	}})
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return fakeController{}, nil
	})

	code := Run([]string{
		"doctor",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 0 && code != 1 {
		t.Fatalf("unexpected exit code = %d, stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	for _, want := range []string{"check logs", "check state", "check static_chain", "check killswitch", "check controller"} {
		if !strings.Contains(fixture.stdout.String(), want) {
			t.Fatalf("doctor output missing %q:\n%s", want, fixture.stdout.String())
		}
	}
	if !strings.Contains(fixture.stdout.String(), "check controller: GREEN") {
		t.Fatalf("controller check should be GREEN via fake controller:\n%s", fixture.stdout.String())
	}
}

func TestDoctorControllerYellowWhenUnreachable(t *testing.T) {
	fixture := newCLIFixture(t)
	applyFixtureChain(t, &fixture)
	withFakeKillRunner(t, &fakeKillRunner{outputs: map[string]string{
		"Get-AppxPackage":         "appx|S-OK|Claude (store)\n",
		"AI-Identity-KillSwitch*": "\n",
	}})
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return nil, controller.ErrNoController
	})

	code := Run([]string{
		"doctor",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code == 0 {
		t.Fatalf("doctor with unreachable controller should not exit 0:\n%s", fixture.stdout.String())
	}
	if !strings.Contains(fixture.stdout.String(), "check controller: YELLOW") {
		t.Fatalf("controller check should be YELLOW when unreachable:\n%s", fixture.stdout.String())
	}
}

func TestDoctorKillSwitchGreenWhenFullyCovered(t *testing.T) {
	fixture := newCLIFixture(t)
	applyFixtureChain(t, &fixture)
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return fakeController{}, nil
	})
	withFakeKillRunner(t, &fakeKillRunner{outputs: map[string]string{
		"Get-AppxPackage":         "appx|S-OK|Claude (store)\nprogram|C:\\a\\claude.exe|Claude (standalone)\n",
		"AI-Identity-KillSwitch*": "AI-Identity-KillSwitch UDP Claude (store)|S-OK|\nAI-Identity-KillSwitch UDP Claude (standalone)||C:\\a\\claude.exe\n",
		"Get-ScheduledTask":       "persist|installed\n",
	}})

	code := Run([]string{
		"doctor",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)
	_ = code
	if !strings.Contains(fixture.stdout.String(), "check killswitch: GREEN covered=2 persist=installed") {
		t.Fatalf("killswitch should be GREEN covered=2 persist=installed when all targets covered (Enabled flag off):\n%s", fixture.stdout.String())
	}
}

func TestKillSwitchStatusPrintsCoverage(t *testing.T) {
	fixture := newCLIFixture(t)
	enableFixtureKillSwitch(t, &fixture)
	withFakeKillRunner(t, &fakeKillRunner{outputs: map[string]string{
		"Get-AppxPackage":         "appx|S-OK|Claude (store)\nprogram|C:\\a\\claude.exe|Claude (standalone)\n",
		"AI-Identity-KillSwitch*": "AI-Identity-KillSwitch UDP Claude (store)|S-OK|\n",
	}})

	code := Run([]string{
		"killswitch",
		"status",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 1 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, fixture.stdout.String(), fixture.stderr.String())
	}
	if !strings.Contains(fixture.stdout.String(), "Claude (store)") || !strings.Contains(fixture.stdout.String(), "missing=1") {
		t.Fatalf("status output missing coverage:\n%s", fixture.stdout.String())
	}
	if !strings.Contains(fixture.stdout.String(), "persist=missing") {
		t.Fatalf("status output missing persist state:\n%s", fixture.stdout.String())
	}
}

func TestKillSwitchApplyInstallsRefreshTask(t *testing.T) {
	fixture := newCLIFixture(t)
	enableFixtureKillSwitch(t, &fixture)
	runner := &fakeKillRunner{outputs: map[string]string{
		"Get-AppxPackage":         "appx|S-OK|Claude (store)\n",
		"AI-Identity-KillSwitch*": "AI-Identity-KillSwitch UDP Claude (store)|S-OK|\n",
	}}
	withFakeKillRunner(t, runner)

	code := Run([]string{
		"killswitch",
		"apply",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, fixture.stdout.String(), fixture.stderr.String())
	}
	joined := strings.Join(runner.ran, "\n")
	for _, want := range []string{"Register-ScheduledTask", "AI-Identity-KillSwitch-Refresh", "--no-persist", "-RunLevel Highest"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("apply persist missing %q:\n%s", want, joined)
		}
	}
	if !strings.Contains(fixture.stdout.String(), "refresh_task=AI-Identity-KillSwitch-Refresh") {
		t.Fatalf("apply output missing refresh task:\n%s", fixture.stdout.String())
	}
}

func TestWantsHiddenConsoleOnlyForNoPersistRefresh(t *testing.T) {
	if !wantsHiddenConsole([]string{"killswitch", "apply", "--no-persist", "--secret", "x"}) {
		t.Fatal("refresh apply should hide console")
	}
	if wantsHiddenConsole([]string{"killswitch", "apply", "--secret", "x"}) {
		t.Fatal("interactive apply should keep console")
	}
	if wantsHiddenConsole([]string{"status"}) {
		t.Fatal("status should keep console")
	}
}

func TestKillSwitchApplyNoPersistSkipsRefreshTask(t *testing.T) {
	fixture := newCLIFixture(t)
	enableFixtureKillSwitch(t, &fixture)
	runner := &fakeKillRunner{outputs: map[string]string{
		"Get-AppxPackage":         "appx|S-OK|Claude (store)\n",
		"AI-Identity-KillSwitch*": "AI-Identity-KillSwitch UDP Claude (store)|S-OK|\n",
	}}
	withFakeKillRunner(t, runner)

	code := Run([]string{
		"killswitch",
		"apply",
		"--no-persist",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, fixture.stdout.String(), fixture.stderr.String())
	}
	if strings.Contains(strings.Join(runner.ran, "\n"), "Register-ScheduledTask") {
		t.Fatalf("--no-persist should not register task: %v", runner.ran)
	}
}

func TestKillSwitchApplyAccessDeniedExit2(t *testing.T) {
	fixture := newCLIFixture(t)
	enableFixtureKillSwitch(t, &fixture)
	withFakeKillRunner(t, &fakeKillRunner{err: errors.New("Access is denied")})

	code := Run([]string{
		"killswitch",
		"apply",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 2 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, fixture.stdout.String(), fixture.stderr.String())
	}
	if !strings.Contains(fixture.stderr.String(), "以管理员运行") {
		t.Fatalf("stderr missing admin hint:\n%s", fixture.stderr.String())
	}
}

func TestStatusCommandPrintsGreenWhenAIHitAndVerifyMatch(t *testing.T) {
	fixture := newCLIFixture(t)
	applyFixtureChain(t, &fixture)
	writeFixtureAIHitLog(t, fixture.clashDir)
	stateFile := filepath.Join(fixture.root, ".ai-identity", "status.json")
	if err := identity.SaveVerifyResult(stateFile, identity.VerifyResult{
		Checked:         true,
		ExitIP:          "203.0.113.8",
		ASN:             "AS12345",
		Country:         "US",
		ExpectedExitIP:  "203.0.113.8",
		ExpectedASN:     "AS12345",
		ExpectedCountry: "US",
	}, time.Date(2026, 6, 17, 1, 2, 3, 0, time.Local)); err != nil {
		t.Fatalf("save verify result: %v", err)
	}

	code := Run([]string{
		"status",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
		"--state", stateFile,
	}, &fixture.stdout, &fixture.stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	if !strings.Contains(fixture.stdout.String(), "GREEN") {
		t.Fatalf("status output missing GREEN:\n%s", fixture.stdout.String())
	}
}

func TestStatusCommandReadsNestedServiceLogForGreen(t *testing.T) {
	fixture := newCLIFixture(t)
	applyFixtureChain(t, &fixture)
	serviceDir := filepath.Join(fixture.clashDir, "logs", "service")
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatalf("mkdir service logs: %v", err)
	}
	logLine := `time="2026-06-17T01:05:05" level=info msg="[TCP] 127.0.0.1:63515(claude.exe) --> claude.ai:443 match ProcessName(claude.exe) using AI-Static[IPRoyal-Static]"`
	if err := os.WriteFile(filepath.Join(serviceDir, "service_latest.log"), []byte(logLine+"\n"), 0o600); err != nil {
		t.Fatalf("write service_latest.log: %v", err)
	}
	stateFile := filepath.Join(fixture.root, ".ai-identity", "status.json")
	if err := identity.SaveVerifyResult(stateFile, identity.VerifyResult{
		Checked:         true,
		ExitIP:          "203.0.113.8",
		ASN:             "AS12345",
		Country:         "US",
		ExpectedExitIP:  "203.0.113.8",
		ExpectedASN:     "AS12345",
		ExpectedCountry: "US",
	}, time.Date(2026, 6, 17, 1, 2, 3, 0, time.Local)); err != nil {
		t.Fatalf("save verify result: %v", err)
	}

	code := Run([]string{
		"status",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
		"--state", stateFile,
	}, &fixture.stdout, &fixture.stderr)

	if code != 0 {
		t.Fatalf("exit code = %d stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	if !strings.Contains(fixture.stdout.String(), "GREEN") {
		t.Fatalf("status output missing GREEN:\n%s", fixture.stdout.String())
	}
	if !strings.Contains(fixture.stdout.String(), "service_latest.log") {
		t.Fatalf("status output missing evidence source:\n%s", fixture.stdout.String())
	}
	if !strings.Contains(fixture.stdout.String(), "evidence_line=") || !strings.Contains(fixture.stdout.String(), "claude.exe") {
		t.Fatalf("status output missing evidence line:\n%s", fixture.stdout.String())
	}
}

func TestStatusCommandPrintsRedWhenPersistedVerifyMismatches(t *testing.T) {
	fixture := newCLIFixture(t)
	applyFixtureChain(t, &fixture)
	writeFixtureAIHitLog(t, fixture.clashDir)
	stateFile := filepath.Join(fixture.root, ".ai-identity", "status.json")
	if err := identity.SaveVerifyResult(stateFile, identity.VerifyResult{
		Checked:         true,
		ExitIP:          "198.51.100.9",
		ASN:             "AS99999",
		Country:         "NL",
		ExpectedExitIP:  "203.0.113.8",
		ExpectedASN:     "AS12345",
		ExpectedCountry: "US",
	}, time.Date(2026, 6, 17, 1, 2, 3, 0, time.Local)); err != nil {
		t.Fatalf("save verify result: %v", err)
	}

	code := Run([]string{
		"status",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
		"--state", stateFile,
	}, &fixture.stdout, &fixture.stderr)

	if code != 2 {
		t.Fatalf("exit code = %d, stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	if !strings.Contains(fixture.stdout.String(), "RED") || !strings.Contains(fixture.stdout.String(), "exit IP mismatch") {
		t.Fatalf("status output missing RED mismatch:\n%s", fixture.stdout.String())
	}
}

func TestMonitorOnceAllStaticExit0(t *testing.T) {
	fixture := newCLIFixture(t)
	var gotEndpoint controller.Endpoint
	withFakeProbe(t, fakeProbe{id: identity.ExitIdentity{IP: "203.0.113.8", ASN: "AS12345", Country: "US"}})
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		gotEndpoint = ep
		return fakeController{conns: []controller.Connection{
			controllerConn("api.anthropic.com", "claude.exe", "IPRoyal-Static", "AI-Static"),
		}}, nil
	})

	code := Run([]string{
		"monitor",
		"--once",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	if gotEndpoint.Secret != "ctl-secret" || gotEndpoint.PipePath == "" {
		t.Fatalf("endpoint = %+v", gotEndpoint)
	}
	if !strings.Contains(fixture.stdout.String(), "ai_connections=1") || !strings.Contains(fixture.stdout.String(), "all_static=YES") {
		t.Fatalf("monitor output missing static summary:\n%s", fixture.stdout.String())
	}
	if !strings.Contains(fixture.stdout.String(), "exit_ip=203.0.113.8") || !strings.Contains(fixture.stdout.String(), "verdict=matched") {
		t.Fatalf("monitor output missing exit identity verdict:\n%s", fixture.stdout.String())
	}
	if !strings.Contains(fixture.stdout.String(), "reason_code=G-VERIFIED") {
		t.Fatalf("monitor output missing reason code:\n%s", fixture.stdout.String())
	}
}

func TestMonitorOnceLeakExit2(t *testing.T) {
	fixture := newCLIFixture(t)
	withFakeProbe(t, fakeProbe{id: identity.ExitIdentity{IP: "203.0.113.8", ASN: "AS12345", Country: "US"}})
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return fakeController{conns: []controller.Connection{
			controllerConn("api.anthropic.com", "claude.exe", "IPRoyal-Static", "AI-Static"),
			controllerConn("www.codexapis.com", "codex.exe", "JMS-x", "Proxies"),
		}}, nil
	})

	code := Run([]string{
		"monitor",
		"--once",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 2 {
		t.Fatalf("exit code = %d, stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	for _, want := range []string{"all_static=NO", "leak target=codex", "www.codexapis.com"} {
		if !strings.Contains(fixture.stdout.String(), want) {
			t.Fatalf("monitor output missing %q:\n%s", want, fixture.stdout.String())
		}
	}
	if !strings.Contains(fixture.stdout.String(), "reason_code=R-AI-NONSTATIC") {
		t.Fatalf("monitor output missing leak reason code:\n%s", fixture.stdout.String())
	}
}

func TestMonitorOnceExitMismatchExit2(t *testing.T) {
	fixture := newCLIFixture(t)
	withFakeProbe(t, fakeProbe{id: identity.ExitIdentity{IP: "198.51.100.9", ASN: "AS12345", Country: "US"}})
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return fakeController{conns: []controller.Connection{
			controllerConn("api.anthropic.com", "claude.exe", "IPRoyal-Static", "AI-Static"),
		}}, nil
	})

	code := Run([]string{
		"monitor",
		"--once",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 2 {
		t.Fatalf("exit code = %d, stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	if !strings.Contains(fixture.stdout.String(), "verdict=mismatched") {
		t.Fatalf("monitor output missing mismatched verdict:\n%s", fixture.stdout.String())
	}
}

func TestMonitorOnceNoExpectedExit1(t *testing.T) {
	fixture := newCLIFixture(t)
	if err := os.WriteFile(fixture.secretFile, []byte(strings.Join([]string{
		"PROXY_HOST=203.0.113.8",
		"PROXY_USER=static-user",
		"PROXY_PASS=static-pass",
		"CONTROLLER_SECRET=ctl-secret",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("rewrite secret: %v", err)
	}
	withFakeProbe(t, fakeProbe{id: identity.ExitIdentity{IP: "203.0.113.8", ASN: "AS12345", Country: "US"}})
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return fakeController{conns: []controller.Connection{
			controllerConn("api.anthropic.com", "claude.exe", "IPRoyal-Static", "AI-Static"),
		}}, nil
	})

	code := Run([]string{
		"monitor",
		"--once",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 1 {
		t.Fatalf("exit code = %d, stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	if !strings.Contains(fixture.stdout.String(), "verdict=unverifiable") || strings.Contains(fixture.stdout.String(), "GREEN") {
		t.Fatalf("monitor output should be unverifiable without GREEN:\n%s", fixture.stdout.String())
	}
}

func TestMonitorOnceProbeFailureExit1(t *testing.T) {
	fixture := newCLIFixture(t)
	withFakeProbe(t, fakeProbe{err: errors.New("probe failed")})
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return fakeController{conns: []controller.Connection{
			controllerConn("api.anthropic.com", "claude.exe", "IPRoyal-Static", "AI-Static"),
		}}, nil
	})

	code := Run([]string{
		"monitor",
		"--once",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 1 {
		t.Fatalf("exit code = %d, stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	if !strings.Contains(fixture.stdout.String(), "exit_ip_sample_failed") {
		t.Fatalf("monitor output missing sample failure:\n%s", fixture.stdout.String())
	}
}

func TestMonitorOnceNoControllerExit2(t *testing.T) {
	fixture := newCLIFixture(t)
	withFakeProbe(t, fakeProbe{id: identity.ExitIdentity{IP: "203.0.113.8", ASN: "AS12345", Country: "US"}})
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return nil, controller.ErrNoController
	})

	code := Run([]string{
		"monitor",
		"--once",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 2 {
		t.Fatalf("exit code = %d, stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	if !strings.Contains(fixture.stderr.String(), "controller unreachable") {
		t.Fatalf("stderr missing unreachable message:\n%s", fixture.stderr.String())
	}
}

func TestMonitorOnceMissingControllerSecretExit2(t *testing.T) {
	fixture := newCLIFixture(t)
	if err := os.WriteFile(fixture.secretFile, []byte(strings.Join([]string{
		"PROXY_HOST=203.0.113.8",
		"PROXY_USER=static-user",
		"PROXY_PASS=static-pass",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("rewrite secret: %v", err)
	}
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		t.Fatal("openController should not be called without controller secret")
		return nil, nil
	})

	code := Run([]string{
		"monitor",
		"--once",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 2 {
		t.Fatalf("exit code = %d, stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	if !strings.Contains(fixture.stderr.String(), "controller secret") {
		t.Fatalf("stderr missing controller secret message:\n%s", fixture.stderr.String())
	}
}

func TestMonitorNoSecretLeak(t *testing.T) {
	fixture := newCLIFixture(t)
	withFakeProbe(t, fakeProbe{id: identity.ExitIdentity{IP: "203.0.113.8", ASN: "AS12345", Country: "US"}})
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return nil, errors.New("controller failed with ctl-secret")
	})

	code := Run([]string{
		"monitor",
		"--once",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 2 {
		t.Fatalf("exit code = %d, stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	if strings.Contains(fixture.stdout.String(), "ctl-secret") || strings.Contains(fixture.stderr.String(), "ctl-secret") {
		t.Fatalf("secret leaked stdout=%s stderr=%s", fixture.stdout.String(), fixture.stderr.String())
	}
	if !strings.Contains(fixture.stderr.String(), "<redacted-secret>") {
		t.Fatalf("stderr missing redaction marker:\n%s", fixture.stderr.String())
	}
}

func TestMonitorOnceSurfacesChainError(t *testing.T) {
	fixture := newCLIFixture(t)
	writeFixtureChainErrorLog(t, fixture.clashDir)
	withFakeProbe(t, fakeProbe{id: identity.ExitIdentity{IP: "203.0.113.8", ASN: "AS12345", Country: "US"}})
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return fakeController{conns: []controller.Connection{
			controllerConn("api.anthropic.com", "claude.exe", "IPRoyal-Static", "AI-Static"),
		}}, nil
	})

	code := Run([]string{
		"monitor",
		"--once",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", code, fixture.stdout.String(), fixture.stderr.String())
	}
	out := fixture.stdout.String()
	if !strings.Contains(out, "chain_error") || !strings.Contains(out, "IPRoyal") {
		t.Fatalf("missing chain_error:\n%s", out)
	}
}

func TestMonitorStepDetectsDropAndIPChange(t *testing.T) {
	cfg := identity.DefaultConfig()
	tr := session.NewTracker()
	ctrl := fakeController{conns: []controller.Connection{
		controllerConnID("1", "claude.ai", "claude.exe", "IPRoyal-Static", "AI-Static"),
		controllerConnID("2", "api.anthropic.com", "claude.exe", "IPRoyal-Static", "AI-Static"),
	}}
	probe := fakeProbe{id: identity.ExitIdentity{IP: "1.1.1.1"}}
	var out bytes.Buffer
	prev, err := monitorStep(context.Background(), ctrl, probe, cfg, tr, nil, true, &out)
	if err != nil {
		t.Fatalf("first monitorStep: %v", err)
	}
	ctrl.conns = []controller.Connection{
		controllerConnID("1", "claude.ai", "claude.exe", "IPRoyal-Static", "AI-Static"),
	}
	probe2 := fakeProbe{id: identity.ExitIdentity{IP: "2.2.2.2"}}
	if _, err := monitorStep(context.Background(), ctrl, probe2, cfg, tr, prev, true, &out); err != nil {
		t.Fatalf("second monitorStep: %v", err)
	}
	r := tr.Report()
	if r.Drops < 1 || !r.IPChanged || r.Worst != identity.Red {
		t.Fatalf("report = %+v", r)
	}
}

func TestMonitorStepSkipsExitSampleOnDropWithoutSchedule(t *testing.T) {
	cfg := identity.DefaultConfig()
	tr := session.NewTracker()
	prev := []controller.Connection{
		controllerConnID("1", "api.anthropic.com", "claude.exe", "IPRoyal-Static", "AI-Static"),
	}
	ctrl := fakeController{conns: []controller.Connection{}}
	probe := fakeProbe{id: identity.ExitIdentity{IP: "9.9.9.9"}}
	var out bytes.Buffer

	if _, err := monitorStep(context.Background(), ctrl, probe, cfg, tr, prev, false, &out); err != nil {
		t.Fatalf("monitorStep: %v", err)
	}
	if strings.Contains(out.String(), "exit_ip=") {
		t.Fatalf("exit IP must not be sampled on drops when not scheduled:\n%s", out.String())
	}
}

func TestMonitorWatchPrintsSummary(t *testing.T) {
	fixture := newCLIFixture(t)
	withFakeProbe(t, fakeProbe{id: identity.ExitIdentity{IP: "203.0.113.8", ASN: "AS12345", Country: "US"}})
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return fakeController{conns: []controller.Connection{
			controllerConnID("1", "claude.ai", "claude.exe", "IPRoyal-Static", "AI-Static"),
		}}, nil
	})

	code := Run([]string{
		"monitor",
		"--watch",
		"--duration", "1ns",
		"--interval", "1h",
		"--ip-interval", "1h",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	if !strings.Contains(fixture.stdout.String(), "session_summary") {
		t.Fatalf("watch output missing summary:\n%s", fixture.stdout.String())
	}
}

func TestHelpMentionsDefaultMonitorIPInterval(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"help"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("help exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "--ip-interval 5m") {
		t.Fatalf("help should mention 5m default ip interval:\n%s", stdout.String())
	}
}

func TestReloadAllStaticExit0(t *testing.T) {
	fixture := newCLIFixture(t)
	var path string
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return fakeController{
			conns: []controller.Connection{
				controllerConn("api.anthropic.com", "claude.exe", "IPRoyal-Static", "AI-Static"),
			},
			reloadedPath: &path,
		}, nil
	})

	code := Run([]string{
		"reload",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, fixture.stdout.String(), fixture.stderr.String())
	}
	if !strings.HasSuffix(path, "clash-verge.yaml") {
		t.Fatalf("reloaded path = %q", path)
	}
}

func TestReloadLeakExit2(t *testing.T) {
	fixture := newCLIFixture(t)
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return fakeController{conns: []controller.Connection{
			controllerConn("www.codexapis.com", "codex.exe", "JMS-x", "Proxies"),
		}}, nil
	})

	code := Run([]string{
		"reload",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 2 || !strings.Contains(fixture.stdout.String(), "re-apply") {
		t.Fatalf("exit=%d stdout=%s", code, fixture.stdout.String())
	}
}

func TestReloadNoAIExit1(t *testing.T) {
	fixture := newCLIFixture(t)
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return fakeController{conns: []controller.Connection{
			controllerConn("baidu.com", "wechat.exe", "DIRECT"),
		}}, nil
	})

	code := Run([]string{
		"reload",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 1 {
		t.Fatalf("exit=%d stdout=%s", code, fixture.stdout.String())
	}
}

func TestReloadRuntimeErrorExit2(t *testing.T) {
	fixture := newCLIFixture(t)
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return fakeController{reloadErr: errors.New("boom")}, nil
	})

	code := Run([]string{
		"reload",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 2 || !strings.Contains(fixture.stderr.String(), "reload failed") {
		t.Fatalf("exit=%d stderr=%s", code, fixture.stderr.String())
	}
}

func TestReloadNoControllerExit2(t *testing.T) {
	fixture := newCLIFixture(t)
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return nil, controller.ErrNoController
	})

	code := Run([]string{
		"reload",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 2 || !strings.Contains(fixture.stderr.String(), "controller unreachable") {
		t.Fatalf("exit=%d stderr=%s", code, fixture.stderr.String())
	}
}

func TestReloadMissingSecretExit2(t *testing.T) {
	fixture := newCLIFixture(t)
	if err := os.WriteFile(fixture.secretFile, []byte("PROXY_HOST=203.0.113.8\nPROXY_USER=u\nPROXY_PASS=p\n"), 0o600); err != nil {
		t.Fatalf("rewrite secret: %v", err)
	}
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		t.Fatal("openController must not be called without secret")
		return nil, nil
	})

	code := Run([]string{
		"reload",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 2 || !strings.Contains(fixture.stderr.String(), "controller secret") {
		t.Fatalf("exit=%d stderr=%s", code, fixture.stderr.String())
	}
}

func TestApplyWritesAuditAndActivity(t *testing.T) {
	fixture := newCLIFixture(t)
	stateFile := filepath.Join(fixture.root, ".ai-identity", "status.json")

	code := Run([]string{
		"apply",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
		"--bootstrap", "JMS-A",
		"--state", stateFile,
	}, &fixture.stdout, &fixture.stderr)

	if code != 0 {
		t.Fatalf("apply exit=%d stdout=%s stderr=%s", code, fixture.stdout.String(), fixture.stderr.String())
	}
	act, err := identity.LoadActivity(filepath.Join(fixture.root, ".ai-identity", "activity.json"))
	if err != nil {
		t.Fatal(err)
	}
	if act.LastApplyAt.IsZero() {
		t.Fatal("last_apply_at not recorded")
	}
	auditFile := filepath.Join(fixture.root, ".ai-identity", "audit.log")
	evs, err := identity.LoadRecentAudit(auditFile, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) == 0 || evs[len(evs)-1].Type != "apply" {
		t.Fatalf("audit=%+v", evs)
	}
	data, err := os.ReadFile(auditFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"static-pass", "ctl-secret"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("audit leaked secret %q:\n%s", secret, data)
		}
	}
}

func TestAuditCommandPrintsRecent(t *testing.T) {
	fixture := newCLIFixture(t)
	stateFile := filepath.Join(fixture.root, ".ai-identity", "status.json")
	code := Run([]string{
		"apply",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
		"--bootstrap", "JMS-A",
		"--state", stateFile,
	}, &fixture.stdout, &fixture.stderr)
	if code != 0 {
		t.Fatalf("apply exit=%d stdout=%s stderr=%s", code, fixture.stdout.String(), fixture.stderr.String())
	}
	fixture.stdout.Reset()
	fixture.stderr.Reset()

	code = Run([]string{
		"audit",
		"--state", stateFile,
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
	}, &fixture.stdout, &fixture.stderr)

	if code != 0 || !strings.Contains(fixture.stdout.String(), "apply") {
		t.Fatalf("audit exit=%d stdout=%s stderr=%s", code, fixture.stdout.String(), fixture.stderr.String())
	}
}

func TestReloadWritesAuditAndActivity(t *testing.T) {
	fixture := newCLIFixture(t)
	stateFile := filepath.Join(fixture.root, ".ai-identity", "status.json")
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return fakeController{conns: []controller.Connection{
			controllerConn("api.anthropic.com", "claude.exe", "IPRoyal-Static", "AI-Static"),
		}}, nil
	})

	code := Run([]string{
		"reload",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
		"--state", stateFile,
	}, &fixture.stdout, &fixture.stderr)

	if code != 0 {
		t.Fatalf("reload exit=%d stdout=%s stderr=%s", code, fixture.stdout.String(), fixture.stderr.String())
	}
	act, err := identity.LoadActivity(filepath.Join(fixture.root, ".ai-identity", "activity.json"))
	if err != nil {
		t.Fatal(err)
	}
	if act.LastReloadAt.IsZero() {
		t.Fatal("last_reload_at not recorded")
	}
	evs, err := identity.LoadRecentAudit(filepath.Join(fixture.root, ".ai-identity", "audit.log"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) == 0 || evs[len(evs)-1].Type != "reload" || !strings.Contains(evs[len(evs)-1].Summary, "result=ok") {
		t.Fatalf("audit=%+v", evs)
	}
}

func TestReloadControllerErrorWritesAudit(t *testing.T) {
	fixture := newCLIFixture(t)
	stateFile := filepath.Join(fixture.root, ".ai-identity", "status.json")
	withFakeOpenController(t, func(ctx context.Context, ep controller.Endpoint) (controller.Controller, error) {
		return nil, controller.ErrNoController
	})

	code := Run([]string{
		"reload",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
		"--state", stateFile,
	}, &fixture.stdout, &fixture.stderr)

	if code != 2 {
		t.Fatalf("reload exit=%d stdout=%s stderr=%s", code, fixture.stdout.String(), fixture.stderr.String())
	}
	evs, err := identity.LoadRecentAudit(filepath.Join(fixture.root, ".ai-identity", "audit.log"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) == 0 || evs[len(evs)-1].Type != "reload" || !strings.Contains(evs[len(evs)-1].Summary, "result=unreachable") {
		t.Fatalf("audit=%+v", evs)
	}
}

func TestVerifyWritesAudit(t *testing.T) {
	fixture := newCLIFixture(t)
	stateFile := filepath.Join(fixture.root, ".ai-identity", "status.json")
	withFakeVerifyProbe(t, fakeProbe{id: identity.ExitIdentity{IP: "203.0.113.8", ASN: "AS12345", Country: "US"}})

	code := Run([]string{
		"verify",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
		"--state", stateFile,
	}, &fixture.stdout, &fixture.stderr)

	if code != 0 {
		t.Fatalf("verify exit=%d stdout=%s stderr=%s", code, fixture.stdout.String(), fixture.stderr.String())
	}
	evs, err := identity.LoadRecentAudit(filepath.Join(fixture.root, ".ai-identity", "audit.log"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) == 0 || evs[len(evs)-1].Type != "verify" || !strings.Contains(evs[len(evs)-1].Summary, "level=GREEN") {
		t.Fatalf("audit=%+v", evs)
	}
}

func TestSwitchBootstrapWritesAudit(t *testing.T) {
	fixture := newCLIFixture(t)
	applyFixtureChain(t, &fixture)
	stateFile := filepath.Join(fixture.root, ".ai-identity", "status.json")

	code := Run([]string{
		"switch-bootstrap",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
		"--state", stateFile,
		"--node", "JMS-A",
	}, &fixture.stdout, &fixture.stderr)

	if code != 0 {
		t.Fatalf("switch exit=%d stdout=%s stderr=%s", code, fixture.stdout.String(), fixture.stderr.String())
	}
	evs, err := identity.LoadRecentAudit(filepath.Join(fixture.root, ".ai-identity", "audit.log"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) == 0 || evs[len(evs)-1].Type != "switch" || !strings.Contains(evs[len(evs)-1].Summary, "bootstrap=JMS-A") {
		t.Fatalf("audit=%+v", evs)
	}
}

type cliFixture struct {
	root         string
	clashDir     string
	identityFile string
	secretFile   string
	stdout       bytes.Buffer
	stderr       bytes.Buffer
}

func applyFixtureChain(t *testing.T, fixture *cliFixture) {
	t.Helper()
	code := Run([]string{
		"apply",
		"--config", fixture.identityFile,
		"--secret", fixture.secretFile,
		"--clash-dir", fixture.clashDir,
		"--bootstrap", "JMS-A",
	}, &fixture.stdout, &fixture.stderr)
	if code != 0 {
		t.Fatalf("apply exit code = %d, stderr=%s stdout=%s", code, fixture.stderr.String(), fixture.stdout.String())
	}
	fixture.stdout.Reset()
	fixture.stderr.Reset()
}

func writeFixtureAIHitLog(t *testing.T, clashDir string) {
	t.Helper()
	logDir := filepath.Join(clashDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	line := `time="2026-06-17T01:00:00" level=info msg="[TCP] 127.0.0.1:54000 --> claude.ai:443 match ProcessName(claude.exe) using AI-Static[IPRoyal-Static]"`
	if err := os.WriteFile(filepath.Join(logDir, "latest.log"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write latest.log: %v", err)
	}
}

func writeFixtureChainErrorLog(t *testing.T, clashDir string) {
	t.Helper()
	dir := filepath.Join(clashDir, "logs", "service")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir service logs: %v", err)
	}
	line := `time="2026-06-18T20:47:34+08:00" level=warning msg="[TCP] dial AI-Static (match ProcessName/claude.exe) 10.0.0.2:54644(claude.exe) --> api.anthropic.com:443 error: can not connect remote err code: 403"`
	if err := os.WriteFile(filepath.Join(dir, "service_latest.log"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write chain error log: %v", err)
	}
}

func newCLIFixture(t *testing.T) cliFixture {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	clashDir := filepath.Join(root, "clash")
	profilesDir := filepath.Join(clashDir, "profiles")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatalf("mkdir profiles: %v", err)
	}
	profilesYaml := `current: profile-main
items:
- uid: profile-main
  option:
    proxies: proxies-option
    groups: groups-option
    rules: rules-option
- uid: proxies-option
  file: proxies-enhancement.yaml
- uid: groups-option
  file: groups-enhancement.yaml
- uid: rules-option
  file: rules-enhancement.yaml
`
	if err := os.WriteFile(filepath.Join(clashDir, "profiles.yaml"), []byte(profilesYaml), 0o600); err != nil {
		t.Fatalf("write profiles.yaml: %v", err)
	}
	for _, name := range []string{"proxies-enhancement.yaml", "groups-enhancement.yaml", "rules-enhancement.yaml"} {
		if err := os.WriteFile(filepath.Join(profilesDir, name), []byte("prepend: []\nappend: []\ndelete: []\n"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	secretFile := filepath.Join(root, "proxy-secret.local")
	if err := os.WriteFile(secretFile, []byte(strings.Join([]string{
		"PROXY_HOST=203.0.113.8",
		"PROXY_HTTP_PORT=12323",
		"PROXY_USER=static-user",
		"PROXY_PASS=static-pass",
		"CONTROLLER_SECRET=ctl-secret",
		"EXPECTED_EXIT_IP=203.0.113.8",
		"EXPECTED_ASN=AS12345",
		"EXPECTED_COUNTRY=US",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	identityFile := filepath.Join(root, "identity.local.yaml")
	if err := os.WriteFile(identityFile, []byte(strings.Join([]string{
		"identity:",
		"  name: ai-static-iproyal",
		"iproyal:",
		"  secret_file: proxy-secret.local",
		"  proxy_mode: http",
		"clash:",
		"  app_dir: " + filepath.ToSlash(clashDir),
	}, "\n")), 0o600); err != nil {
		t.Fatalf("write identity: %v", err)
	}

	return cliFixture{
		root:         root,
		clashDir:     clashDir,
		identityFile: identityFile,
		secretFile:   secretFile,
	}
}

func enableFixtureKillSwitch(t *testing.T, fixture *cliFixture) {
	t.Helper()
	data, err := os.ReadFile(fixture.identityFile)
	if err != nil {
		t.Fatalf("read identity: %v", err)
	}
	next := string(data) + strings.Join([]string{
		"",
		"killswitch:",
		"  enabled: true",
		"  block_udp: true",
		"  appx: '*Claude*'",
		"  globs: 'C:\\a\\claude.exe'",
	}, "\n")
	if err := os.WriteFile(fixture.identityFile, []byte(next), 0o600); err != nil {
		t.Fatalf("write identity: %v", err)
	}
}

func readFixtureFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

type fakeController struct {
	conns        []controller.Connection
	err          error
	reloadErr    error
	reloadedPath *string
}

func (f fakeController) Ping(ctx context.Context) (controller.Version, error) {
	return controller.Version{}, nil
}

func (f fakeController) Connections(ctx context.Context) ([]controller.Connection, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.conns, nil
}

func (f fakeController) RuntimeConfig(ctx context.Context) (controller.RuntimeConfig, error) {
	return controller.RuntimeConfig{}, nil
}

func (f fakeController) ReloadRuntime(ctx context.Context, configPath string) error {
	if f.reloadedPath != nil {
		*f.reloadedPath = configPath
	}
	return f.reloadErr
}

func (f fakeController) Transport() string {
	return "fake"
}

func (f fakeController) Close() error {
	return nil
}

type fakeProbe struct {
	id  identity.ExitIdentity
	err error
}

func (f fakeProbe) Fetch(ctx context.Context) (identity.ExitIdentity, error) {
	return f.id, f.err
}

func withFakeProbe(t *testing.T, p identity.ExitProbe) {
	t.Helper()
	previous := newExitProbe
	newExitProbe = func(cfg identity.Config) identity.ExitProbe {
		return p
	}
	t.Cleanup(func() {
		newExitProbe = previous
	})
}

func withFakeVerifyProbe(t *testing.T, p identity.ExitProbe) {
	t.Helper()
	previous := newVerifyProbe
	newVerifyProbe = func(cfg identity.Config, url string, timeout time.Duration) identity.ExitProbe {
		return p
	}
	t.Cleanup(func() {
		newVerifyProbe = previous
	})
}

func withFakeOpenController(t *testing.T, fn func(context.Context, controller.Endpoint) (controller.Controller, error)) {
	t.Helper()
	previous := openController
	openController = fn
	t.Cleanup(func() {
		openController = previous
	})
}

func withFakeKillRunner(t *testing.T, runner killswitch.CommandRunner) {
	t.Helper()
	previous := newKillRunner
	newKillRunner = func() killswitch.CommandRunner {
		return runner
	}
	t.Cleanup(func() {
		newKillRunner = previous
	})
}

type fakeKillRunner struct {
	outputs map[string]string
	ran     []string
	err     error
}

func (f *fakeKillRunner) Run(ctx context.Context, script string) (string, error) {
	f.ran = append(f.ran, script)
	if f.err != nil {
		return "", f.err
	}
	for key, out := range f.outputs {
		if strings.Contains(script, key) {
			return out, nil
		}
	}
	return "", nil
}

func controllerConn(host, proc string, chains ...string) controller.Connection {
	return controller.Connection{
		Chains: chains,
		Metadata: controller.Metadata{
			Host:    host,
			Process: proc,
		},
	}
}

func controllerConnID(id, host, proc string, chains ...string) controller.Connection {
	c := controllerConn(host, proc, chains...)
	c.ID = id
	return c
}
