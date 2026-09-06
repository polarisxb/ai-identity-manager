package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ProfilePaths struct {
	ProfileUID  string
	ProxiesFile string
	GroupsFile  string
	RulesFile   string
}

func ResolveProfilePaths(clashDir string) (ProfilePaths, error) {
	profilesYaml := filepath.Join(clashDir, "profiles.yaml")
	data, err := os.ReadFile(profilesYaml)
	if err != nil {
		return ProfilePaths{}, err
	}
	text := string(data)

	currentMatch := regexp.MustCompile(`(?m)^current:\s*(\S+)\s*$`).FindStringSubmatch(text)
	if currentMatch == nil {
		return ProfilePaths{}, fmt.Errorf("current profile not found in %s", profilesYaml)
	}
	currentUID := currentMatch[1]
	profileBlock, err := yamlUIDBlock(text, currentUID)
	if err != nil {
		return ProfilePaths{}, err
	}

	proxiesUID, err := optionUID(profileBlock, "proxies")
	if err != nil {
		return ProfilePaths{}, err
	}
	groupsUID, err := optionUID(profileBlock, "groups")
	if err != nil {
		return ProfilePaths{}, err
	}
	rulesUID, err := optionUID(profileBlock, "rules")
	if err != nil {
		return ProfilePaths{}, err
	}

	profilesDir := filepath.Join(clashDir, "profiles")
	proxiesFile, err := itemFile(text, profilesDir, proxiesUID)
	if err != nil {
		return ProfilePaths{}, err
	}
	groupsFile, err := itemFile(text, profilesDir, groupsUID)
	if err != nil {
		return ProfilePaths{}, err
	}
	rulesFile, err := itemFile(text, profilesDir, rulesUID)
	if err != nil {
		return ProfilePaths{}, err
	}

	return ProfilePaths{
		ProfileUID:  currentUID,
		ProxiesFile: proxiesFile,
		GroupsFile:  groupsFile,
		RulesFile:   rulesFile,
	}, nil
}

func yamlUIDBlock(text, uid string) (string, error) {
	lines := strings.Split(text, "\n")
	start := -1
	uidPattern := regexp.MustCompile(`^-\s+uid:\s*` + regexp.QuoteMeta(uid) + `\s*$`)
	anyUIDPattern := regexp.MustCompile(`^-\s+uid:\s*`)
	for i, line := range lines {
		if uidPattern.MatchString(strings.TrimRight(line, "\r")) {
			start = i
			break
		}
	}
	if start < 0 {
		return "", fmt.Errorf("profile item %q not found", uid)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if anyUIDPattern.MatchString(strings.TrimRight(lines[i], "\r")) {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n"), nil
}

func optionUID(block, name string) (string, error) {
	pattern := regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(name) + `:\s*(\S+)\s*$`)
	match := pattern.FindStringSubmatch(block)
	if match == nil {
		return "", fmt.Errorf("current profile does not bind option %q", name)
	}
	return match[1], nil
}

func itemFile(text, profilesDir, uid string) (string, error) {
	block, err := yamlUIDBlock(text, uid)
	if err != nil {
		return "", err
	}
	pattern := regexp.MustCompile(`(?m)^\s+file:\s*(.+?)\s*$`)
	match := pattern.FindStringSubmatch(block)
	if match == nil {
		return "", fmt.Errorf("profile item %q has no file", uid)
	}
	name := strings.Trim(match[1], ` "'`)
	return filepath.Join(profilesDir, filepath.FromSlash(name)), nil
}
