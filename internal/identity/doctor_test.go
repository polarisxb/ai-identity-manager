package identity

import (
	"net"
	"path/filepath"
	"testing"
)

func TestRunDoctorChecksReportsMissingLogs(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Clash.AppDir = t.TempDir()
	cfg.Clash.MixedPort = ""
	cfg.Clash.Controller = ""
	paths := ProfilePaths{
		ProxiesFile: filepath.Join(cfg.Clash.AppDir, "missing-proxies.yaml"),
		GroupsFile:  filepath.Join(cfg.Clash.AppDir, "missing-groups.yaml"),
		RulesFile:   filepath.Join(cfg.Clash.AppDir, "missing-rules.yaml"),
	}

	checks := RunDoctorChecks(cfg, paths, filepath.Join(cfg.Clash.AppDir, ".ai-identity", "status.json"))
	if findDoctorCheck(checks, "logs").Level != Yellow {
		t.Fatalf("logs check should be YELLOW: %+v", checks)
	}
	if findDoctorCheck(checks, "state").Level != Green {
		t.Fatalf("missing state file should be acceptable: %+v", checks)
	}
}

func TestRunDoctorChecksReportsReachableMixedPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	cfg := DefaultConfig()
	cfg.Clash.AppDir = t.TempDir()
	cfg.Clash.MixedPort = listener.Addr().String()
	cfg.Clash.Controller = ""

	checks := RunDoctorChecks(cfg, ProfilePaths{}, filepath.Join(cfg.Clash.AppDir, ".ai-identity", "status.json"))
	if findDoctorCheck(checks, "mixed_port").Level != Green {
		t.Fatalf("mixed_port should be GREEN: %+v", checks)
	}
}

func findDoctorCheck(checks []DoctorCheck, name string) DoctorCheck {
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	return DoctorCheck{Name: name, Level: Red, Message: "missing check"}
}
