package identity

import (
	"net"
	"os"
	"strings"
	"time"
)

type DoctorCheck struct {
	Name    string
	Level   Level
	Message string
}

func RunDoctorChecks(cfg Config, paths ProfilePaths, statePath string) []DoctorCheck {
	checks := []DoctorCheck{
		checkPath("clash_dir", cfg.Clash.AppDir),
	}
	if paths.ProxiesFile != "" || paths.GroupsFile != "" || paths.RulesFile != "" {
		checks = append(checks,
			checkPath("proxies_file", paths.ProxiesFile),
			checkPath("groups_file", paths.GroupsFile),
			checkPath("rules_file", paths.RulesFile),
		)
		if report, err := InspectEnhancements(paths, cfg); err != nil {
			checks = append(checks, DoctorCheck{Name: "static_chain", Level: Red, Message: err.Error()})
		} else if report.OK {
			checks = append(checks, DoctorCheck{Name: "static_chain", Level: Green, Message: "complete"})
		} else {
			checks = append(checks, DoctorCheck{Name: "static_chain", Level: Red, Message: "missing: " + strings.Join(report.Missing, ", ")})
		}
	}

	if files, _ := DiscoverLogFiles(cfg.Clash.AppDir); len(files) == 0 {
		checks = append(checks, DoctorCheck{Name: "logs", Level: Yellow, Message: "no Clash logs discovered"})
	} else {
		checks = append(checks, DoctorCheck{Name: "logs", Level: Green, Message: "Clash logs discovered"})
	}
	checks = append(checks, checkTCP("mixed_port", cfg.Clash.MixedPort))
	if _, err := LoadVerifyResult(statePath); err != nil {
		checks = append(checks, DoctorCheck{Name: "state", Level: Yellow, Message: err.Error()})
	} else {
		checks = append(checks, DoctorCheck{Name: "state", Level: Green, Message: "state readable or absent"})
	}
	return checks
}

func checkPath(name, path string) DoctorCheck {
	if strings.TrimSpace(path) == "" {
		return DoctorCheck{Name: name, Level: Yellow, Message: "not configured"}
	}
	if _, err := os.Stat(path); err != nil {
		return DoctorCheck{Name: name, Level: Red, Message: err.Error()}
	}
	return DoctorCheck{Name: name, Level: Green, Message: "ok"}
}

func checkTCP(name, addr string) DoctorCheck {
	if strings.TrimSpace(addr) == "" {
		return DoctorCheck{Name: name, Level: Yellow, Message: "not configured"}
	}
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return DoctorCheck{Name: name, Level: Yellow, Message: err.Error()}
	}
	_ = conn.Close()
	return DoctorCheck{Name: name, Level: Green, Message: "reachable"}
}
