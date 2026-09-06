package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProfilePathsFindsCurrentEnhancementFiles(t *testing.T) {
	dir := t.TempDir()
	profilesDir := filepath.Join(dir, "profiles")
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
	if err := os.WriteFile(filepath.Join(dir, "profiles.yaml"), []byte(profilesYaml), 0o600); err != nil {
		t.Fatalf("write profiles.yaml: %v", err)
	}

	paths, err := ResolveProfilePaths(dir)
	if err != nil {
		t.Fatalf("resolve profile paths: %v", err)
	}

	if paths.ProfileUID != "profile-main" {
		t.Fatalf("profile uid = %q", paths.ProfileUID)
	}
	if paths.ProxiesFile != filepath.Join(profilesDir, "proxies-enhancement.yaml") {
		t.Fatalf("proxies file = %q", paths.ProxiesFile)
	}
	if paths.GroupsFile != filepath.Join(profilesDir, "groups-enhancement.yaml") {
		t.Fatalf("groups file = %q", paths.GroupsFile)
	}
	if paths.RulesFile != filepath.Join(profilesDir, "rules-enhancement.yaml") {
		t.Fatalf("rules file = %q", paths.RulesFile)
	}
}
