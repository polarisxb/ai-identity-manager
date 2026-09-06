package identity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyEnhancementsWritesStaticChainWithoutOrdinaryProxyPollution(t *testing.T) {
	dir := t.TempDir()
	paths := ProfilePaths{
		ProxiesFile: filepath.Join(dir, "proxies.yaml"),
		GroupsFile:  filepath.Join(dir, "groups.yaml"),
		RulesFile:   filepath.Join(dir, "rules.yaml"),
	}
	for _, path := range []string{paths.ProxiesFile, paths.GroupsFile, paths.RulesFile} {
		if err := os.WriteFile(path, []byte("prepend: []\nappend: []\ndelete: []\n"), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", path, err)
		}
	}
	cfg := DefaultConfig()
	cfg.IPRoyal = IPRoyalConfig{
		Host:      "203.0.113.8",
		HTTPPort:  "12323",
		SOCKSPort: "12324",
		Username:  "static-user",
		Password:  "static-pass",
		ProxyMode: "http",
	}

	result, err := ApplyEnhancements(paths, cfg, ApplyOptions{
		BootstrapCandidates: []string{"JMS-A", "JMS-B"},
		Timestamp:           "20260617-000000",
	})
	if err != nil {
		t.Fatalf("apply enhancements: %v", err)
	}
	if len(result.ChangedFiles) != 3 {
		t.Fatalf("changed files = %v", result.ChangedFiles)
	}

	proxies := readFile(t, paths.ProxiesFile)
	groups := readFile(t, paths.GroupsFile)
	rules := readFile(t, paths.RulesFile)

	for _, want := range []string{
		"name: IPRoyal-Static",
		"type: http",
		"server: '203.0.113.8'",
		"port: 12323",
		"username: 'static-user'",
		"password: 'static-pass'",
		"dialer-proxy: JMS-Bootstrap",
	} {
		if !strings.Contains(proxies, want) {
			t.Fatalf("proxy enhancement missing %q:\n%s", want, proxies)
		}
	}
	for _, want := range []string{
		"name: JMS-Bootstrap",
		"- 'JMS-A'",
		"- 'JMS-B'",
		"name: AI-Static",
		"- IPRoyal-Static",
	} {
		if !strings.Contains(groups, want) {
			t.Fatalf("group enhancement missing %q:\n%s", want, groups)
		}
	}
	for _, want := range []string{
		"PROCESS-NAME,claude.exe,AI-Static",
		"PROCESS-NAME,codex.exe,AI-Static",
		"DOMAIN-SUFFIX,openai.com,AI-Static",
		"DOMAIN-SUFFIX,codexapis.com,AI-Static",
		"DOMAIN-SUFFIX,ipinfo.io,AI-Static",
	} {
		if !strings.Contains(rules, want) {
			t.Fatalf("rules enhancement missing %q:\n%s", want, rules)
		}
	}
	for _, stale := range []string{
		"PROCESS-NAME,Cursor.exe,AI-Static",
		"DOMAIN-SUFFIX,cursor.com,AI-Static",
		"DOMAIN-SUFFIX,cursor.sh,AI-Static",
	} {
		if strings.Contains(rules, stale) {
			t.Fatalf("rules enhancement should not include retired cursor path %q:\n%s", stale, rules)
		}
	}
	if strings.Contains(groups, "Proxies") {
		t.Fatalf("ordinary Proxies group should not be modified:\n%s", groups)
	}
}

func TestSwitchBootstrapMovesExistingNodeFirst(t *testing.T) {
	input := strings.Join([]string{
		"prepend:",
		"- name: JMS-Bootstrap",
		"  type: select",
		"  proxies:",
		"  - 'JMS-A'",
		"  - 'JMS-B'",
		"  - 'JMS-C'",
		"- name: AI-Static",
		"  type: select",
		"  proxies:",
		"  - IPRoyal-Static",
		"append: []",
		"delete: []",
	}, "\n")

	output, err := SwitchBootstrapInGroups(input, "JMS-Bootstrap", "JMS-C")
	if err != nil {
		t.Fatalf("switch bootstrap: %v", err)
	}

	first := strings.Index(output, "- 'JMS-C'")
	second := strings.Index(output, "- 'JMS-A'")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("JMS-C was not moved before JMS-A:\n%s", output)
	}
	if !strings.Contains(output, "- IPRoyal-Static") {
		t.Fatalf("final IPRoyal node should remain untouched:\n%s", output)
	}
}

func TestInspectEnhancementsReportsStaticChain(t *testing.T) {
	dir := t.TempDir()
	paths := ProfilePaths{
		ProxiesFile: filepath.Join(dir, "proxies.yaml"),
		GroupsFile:  filepath.Join(dir, "groups.yaml"),
		RulesFile:   filepath.Join(dir, "rules.yaml"),
	}
	cfg := DefaultConfig()
	cfg.IPRoyal = IPRoyalConfig{
		Host:      "203.0.113.8",
		HTTPPort:  "12323",
		Username:  "static-user",
		Password:  "static-pass",
		ProxyMode: "http",
	}
	if _, err := ApplyEnhancements(paths, cfg, ApplyOptions{
		BootstrapCandidates: []string{"JMS-A", "JMS-B"},
	}); err != nil {
		t.Fatalf("apply enhancements: %v", err)
	}

	report, err := InspectEnhancements(paths, cfg)
	if err != nil {
		t.Fatalf("inspect enhancements: %v", err)
	}

	if !report.OK {
		t.Fatalf("report should be OK: %+v", report)
	}
	if len(report.BootstrapCandidates) != 2 || report.BootstrapCandidates[0] != "JMS-A" || report.BootstrapCandidates[1] != "JMS-B" {
		t.Fatalf("bootstrap candidates = %+v", report.BootstrapCandidates)
	}
}

func TestPlanEnhancementWritesReportsNoOp(t *testing.T) {
	dir := t.TempDir()
	paths := ProfilePaths{
		ProxiesFile: filepath.Join(dir, "proxies.yaml"),
		GroupsFile:  filepath.Join(dir, "groups.yaml"),
		RulesFile:   filepath.Join(dir, "rules.yaml"),
	}
	cfg := DefaultConfig()
	cfg.IPRoyal = IPRoyalConfig{
		Host:      "203.0.113.8",
		HTTPPort:  "12323",
		Username:  "u",
		Password:  "p",
		ProxyMode: "http",
	}
	if _, err := ApplyEnhancements(paths, cfg, ApplyOptions{BootstrapCandidates: []string{"JMS-A"}}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	writes, err := PlanEnhancementWrites(paths, cfg, ApplyOptions{BootstrapCandidates: []string{"JMS-A"}})
	if err != nil {
		t.Fatalf("plan writes: %v", err)
	}
	for _, write := range writes {
		if write.Changed {
			t.Fatalf("expected no-op write: %+v", write)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
