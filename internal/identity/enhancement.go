package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type ApplyOptions struct {
	BootstrapCandidates []string
	Timestamp           string
	NoRules             bool
}

type ApplyResult struct {
	ChangedFiles []string
	Backups      []string
}

type PlannedWrite struct {
	Path       string
	Existing   string
	Next       string
	BackupPath string
	Changed    bool
	Exists     bool
}

type ChainReport struct {
	OK                  bool
	Missing             []string
	BootstrapCandidates []string
}

func InspectEnhancements(paths ProfilePaths, cfg Config) (ChainReport, error) {
	proxies, err := os.ReadFile(paths.ProxiesFile)
	if err != nil {
		return ChainReport{}, err
	}
	groups, err := os.ReadFile(paths.GroupsFile)
	if err != nil {
		return ChainReport{}, err
	}
	rules, err := os.ReadFile(paths.RulesFile)
	if err != nil {
		return ChainReport{}, err
	}

	report := ChainReport{}
	proxyText := string(proxies)
	groupText := string(groups)
	ruleText := string(rules)
	checks := []struct {
		name string
		ok   bool
	}{
		{"static node " + cfg.Nodes.StaticNode, strings.Contains(proxyText, "name: "+cfg.Nodes.StaticNode)},
		{"bootstrap dialer " + cfg.Nodes.BootstrapGroup, strings.Contains(proxyText, "dialer-proxy: "+cfg.Nodes.BootstrapGroup)},
		{"bootstrap group " + cfg.Nodes.BootstrapGroup, strings.Contains(groupText, "name: "+cfg.Nodes.BootstrapGroup)},
		{"final group " + cfg.Nodes.FinalGroup, strings.Contains(groupText, "name: "+cfg.Nodes.FinalGroup)},
		{"final group member " + cfg.Nodes.StaticNode, strings.Contains(groupText, "- "+cfg.Nodes.StaticNode)},
		{"claude rule", strings.Contains(ruleText, "claude.exe,"+cfg.Nodes.FinalGroup) || strings.Contains(ruleText, "claude.ai,"+cfg.Nodes.FinalGroup)},
		{"openai rule", strings.Contains(ruleText, "openai.com,"+cfg.Nodes.FinalGroup)},
		{"codex rule", strings.Contains(ruleText, "codex.exe,"+cfg.Nodes.FinalGroup) || strings.Contains(ruleText, "codexapis.com,"+cfg.Nodes.FinalGroup)},
		{"ipinfo rule", strings.Contains(ruleText, "ipinfo.io,"+cfg.Nodes.FinalGroup)},
	}
	for _, check := range checks {
		if !check.ok {
			report.Missing = append(report.Missing, check.name)
		}
	}
	report.BootstrapCandidates = extractBootstrapCandidates(groupText, cfg.Nodes.BootstrapGroup)
	report.OK = len(report.Missing) == 0
	return report, nil
}

func ApplyEnhancements(paths ProfilePaths, cfg Config, opts ApplyOptions) (ApplyResult, error) {
	writes, err := PlanEnhancementWrites(paths, cfg, opts)
	if err != nil {
		return ApplyResult{}, err
	}
	result := ApplyResult{}
	for _, write := range writes {
		if !write.Changed {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(write.Path), 0o755); err != nil {
			return ApplyResult{}, err
		}
		if write.Exists {
			if err := os.WriteFile(write.BackupPath, []byte(write.Existing), 0o600); err != nil {
				return ApplyResult{}, err
			}
			result.Backups = append(result.Backups, write.BackupPath)
		}
		if err := os.WriteFile(write.Path, []byte(write.Next), 0o600); err != nil {
			return ApplyResult{}, err
		}
		result.ChangedFiles = append(result.ChangedFiles, write.Path)
	}
	return result, nil
}

func PlanEnhancementWrites(paths ProfilePaths, cfg Config, opts ApplyOptions) ([]PlannedWrite, error) {
	if cfg.IPRoyal.Host == "" || cfg.IPRoyal.HTTPPort == "" {
		return nil, fmt.Errorf("iproyal host and HTTP port are required")
	}
	candidates := opts.BootstrapCandidates
	if len(candidates) == 0 {
		candidates = cfg.Bootstrap.Candidates
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("at least one JMS bootstrap candidate is required")
	}
	stamp := opts.Timestamp
	if stamp == "" {
		stamp = time.Now().Format("20060102-150405")
	}
	rendered := []struct {
		path  string
		lines []string
	}{
		{path: paths.ProxiesFile, lines: renderProxyEnhancement(cfg)},
		{path: paths.GroupsFile, lines: renderGroupEnhancement(cfg, candidates)},
	}
	if !opts.NoRules {
		rendered = append(rendered, struct {
			path  string
			lines []string
		}{path: paths.RulesFile, lines: renderRuleEnhancement(cfg)})
	}

	writes := make([]PlannedWrite, 0, len(rendered))
	for _, item := range rendered {
		next := strings.Join(item.lines, "\n") + "\n"
		existingBytes, err := os.ReadFile(item.path)
		exists := err == nil
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		existing := string(existingBytes)
		writes = append(writes, PlannedWrite{
			Path:       item.path,
			Existing:   existing,
			Next:       next,
			BackupPath: item.path + ".bak-ai-identity-" + stamp,
			Changed:    !exists || existing != next,
			Exists:     exists,
		})
	}
	return writes, nil
}

func extractBootstrapCandidates(content, groupName string) []string {
	lines := strings.Split(content, "\n")
	groupStart := -1
	groupPattern := regexp.MustCompile(`^\s*-\s+name:\s*` + regexp.QuoteMeta(groupName) + `\s*$`)
	for i, line := range lines {
		if groupPattern.MatchString(line) {
			groupStart = i
			break
		}
	}
	if groupStart < 0 {
		return nil
	}
	proxiesLine := -1
	for i := groupStart + 1; i < len(lines); i++ {
		if regexp.MustCompile(`^\s*-\s+name:\s*`).MatchString(lines[i]) || isTopLevelKey(lines[i]) {
			break
		}
		if strings.TrimSpace(lines[i]) == "proxies:" {
			proxiesLine = i
			break
		}
	}
	if proxiesLine < 0 {
		return nil
	}
	candidates := []string{}
	for i := proxiesLine + 1; i < len(lines); i++ {
		line := lines[i]
		if regexp.MustCompile(`^\s*-\s+name:\s*`).MatchString(line) || isTopLevelKey(line) {
			break
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			candidates = append(candidates, strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")), `"'`))
		}
	}
	return candidates
}

func renderProxyEnhancement(cfg Config) []string {
	port := cfg.IPRoyal.HTTPPort
	if cfg.IPRoyal.ProxyMode == "socks5" {
		port = cfg.IPRoyal.SOCKSPort
	}
	return []string{
		"# Managed by ai-identity-manager",
		"# Final exit identity is fixed. Do not add fallback nodes here.",
		"prepend: []",
		"append:",
		"- name: " + cfg.Nodes.StaticNode,
		"  type: " + cfg.IPRoyal.ProxyMode,
		"  server: " + yamlSingleQuote(cfg.IPRoyal.Host),
		"  port: " + port,
		"  username: " + yamlSingleQuote(cfg.IPRoyal.Username),
		"  password: " + yamlSingleQuote(cfg.IPRoyal.Password),
		"  dialer-proxy: " + cfg.Nodes.BootstrapGroup,
		"  tfo: false",
		"delete: []",
	}
}

func renderGroupEnhancement(cfg Config, candidates []string) []string {
	lines := []string{
		"# Managed by ai-identity-manager",
		"# JMS-Bootstrap is manual-select only: no final identity fallback.",
		"prepend:",
		"- name: " + cfg.Nodes.BootstrapGroup,
		"  type: select",
		"  proxies:",
	}
	for _, candidate := range dedupeNonEmpty(candidates) {
		lines = append(lines, "  - "+yamlSingleQuote(candidate))
	}
	lines = append(lines,
		"- name: "+cfg.Nodes.FinalGroup,
		"  type: select",
		"  proxies:",
		"  - "+cfg.Nodes.StaticNode,
		"append: []",
		"delete: []",
	)
	return lines
}

func renderRuleEnhancement(cfg Config) []string {
	group := cfg.Nodes.FinalGroup
	rules := []string{
		"PROCESS-NAME,claude.exe," + group,
		"PROCESS-NAME,Claude.exe," + group,
		"PROCESS-NAME,ChatGPT.exe," + group,
		"PROCESS-NAME,codex.exe," + group,
		"PROCESS-NAME,Codex.exe," + group,
		"DOMAIN-SUFFIX,claude.ai," + group,
		"DOMAIN-SUFFIX,anthropic.com," + group,
		"DOMAIN-SUFFIX,anthropic.ai," + group,
		"DOMAIN,browser-intake-us5-datadoghq.com," + group,
		"DOMAIN-SUFFIX,openai.com," + group,
		"DOMAIN-SUFFIX,chatgpt.com," + group,
		"DOMAIN-SUFFIX,codexapis.com," + group,
		"DOMAIN-SUFFIX,oaistatic.com," + group,
		"DOMAIN-SUFFIX,oaiusercontent.com," + group,
		"DOMAIN-SUFFIX,ping0.cc," + group,
		"DOMAIN-SUFFIX,ipinfo.io," + group,
	}
	lines := []string{
		"# Managed by ai-identity-manager",
		"# Only identity-sensitive AI/test domains use the static ISP path.",
		"prepend:",
	}
	for _, rule := range rules {
		lines = append(lines, "- "+rule)
	}
	lines = append(lines, "append: []", "delete: []")
	return lines
}

func SwitchBootstrapInGroups(content, groupName, nodeName string) (string, error) {
	lines := strings.Split(content, "\n")
	groupStart := -1
	groupPattern := regexp.MustCompile(`^\s*-\s+name:\s*` + regexp.QuoteMeta(groupName) + `\s*$`)
	for i, line := range lines {
		if groupPattern.MatchString(line) {
			groupStart = i
			break
		}
	}
	if groupStart < 0 {
		return "", fmt.Errorf("bootstrap group %q not found", groupName)
	}

	proxiesLine := -1
	for i := groupStart + 1; i < len(lines); i++ {
		if regexp.MustCompile(`^\s*-\s+name:\s*`).MatchString(lines[i]) || isTopLevelKey(lines[i]) {
			break
		}
		if strings.TrimSpace(lines[i]) == "proxies:" {
			proxiesLine = i
			break
		}
	}
	if proxiesLine < 0 {
		return "", fmt.Errorf("bootstrap group %q has no proxies list", groupName)
	}

	end := proxiesLine + 1
	items := []string{}
	for end < len(lines) {
		trimmed := strings.TrimSpace(lines[end])
		if trimmed == "" {
			end++
			continue
		}
		if regexp.MustCompile(`^\s*-\s+name:\s*`).MatchString(lines[end]) || isTopLevelKey(lines[end]) {
			break
		}
		if strings.HasPrefix(trimmed, "- ") {
			items = append(items, strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")), `"'`))
		}
		end++
	}

	found := false
	reordered := []string{nodeName}
	for _, item := range items {
		if item == nodeName {
			found = true
			continue
		}
		reordered = append(reordered, item)
	}
	if !found {
		return "", fmt.Errorf("bootstrap node %q not found in %s", nodeName, groupName)
	}

	newLines := append([]string{}, lines[:proxiesLine+1]...)
	for _, item := range reordered {
		if strings.TrimSpace(item) != "" {
			newLines = append(newLines, "  - "+yamlSingleQuote(item))
		}
	}
	newLines = append(newLines, lines[end:]...)
	return strings.Join(newLines, "\n"), nil
}

func backupFile(path, stamp string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if stamp == "" {
		stamp = time.Now().Format("20060102-150405")
	}
	backup := path + ".bak-ai-identity-" + stamp
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return backup, os.WriteFile(backup, data, 0o600)
}

func yamlSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func dedupeNonEmpty(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func isTopLevelKey(line string) bool {
	return regexp.MustCompile(`^[A-Za-z0-9_-]+:\s*`).MatchString(line)
}
