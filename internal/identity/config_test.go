package identity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigMergesIdentityYamlAndSecretFile(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "proxy-secret.local")
	identityPath := filepath.Join(dir, "identity.local.yaml")

	if err := os.WriteFile(secretPath, []byte(strings.Join([]string{
		"PROXY_HOST=203.0.113.8",
		"PROXY_HTTP_PORT=12323",
		"PROXY_SOCKS_PORT=12324",
		"PROXY_USER=static-user",
		"PROXY_PASS=static-pass",
		"EXPECTED_EXIT_IP=203.0.113.8",
		"EXPECTED_ASN=AS12345",
		"EXPECTED_COUNTRY=US",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if err := os.WriteFile(identityPath, []byte(strings.Join([]string{
		"identity:",
		"  name: ai-static-iproyal",
		"iproyal:",
		"  secret_file: proxy-secret.local",
		"  proxy_mode: http",
		"clash:",
		"  app_dir: C:/Users/test/AppData/Roaming/io.github.clash-verge-rev.clash-verge-rev",
		"  mixed_port: 127.0.0.1:7889",
		"  controller: 127.0.0.1:9097",
		"bootstrap:",
		"  switch_after_failures: 5",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("write identity config: %v", err)
	}

	cfg, err := LoadConfig(LoadOptions{
		IdentityFile: identityPath,
		SecretFile:   secretPath,
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Name != "ai-static-iproyal" {
		t.Fatalf("name = %q", cfg.Name)
	}
	if cfg.IPRoyal.Host != "203.0.113.8" || cfg.IPRoyal.HTTPPort != "12323" || cfg.IPRoyal.SOCKSPort != "12324" {
		t.Fatalf("unexpected iproyal endpoint: %+v", cfg.IPRoyal)
	}
	if cfg.IPRoyal.Username != "static-user" || cfg.IPRoyal.Password != "static-pass" {
		t.Fatalf("secret credentials not loaded")
	}
	if cfg.Expected.ExitIP != "203.0.113.8" || cfg.Expected.ASN != "AS12345" || cfg.Expected.Country != "US" {
		t.Fatalf("expected identity not loaded: %+v", cfg.Expected)
	}
	if cfg.Clash.AppDir != "C:/Users/test/AppData/Roaming/io.github.clash-verge-rev.clash-verge-rev" {
		t.Fatalf("clash app dir = %q", cfg.Clash.AppDir)
	}
	if cfg.Nodes.FinalGroup != "AI-Static" || cfg.Nodes.StaticNode != "IPRoyal-Static" || cfg.Nodes.BootstrapGroup != "JMS-Bootstrap" {
		t.Fatalf("node defaults not applied: %+v", cfg.Nodes)
	}
	if cfg.Bootstrap.SwitchAfterFailures != 5 {
		t.Fatalf("switch_after_failures = %d", cfg.Bootstrap.SwitchAfterFailures)
	}
}

func TestLoadConfigReadsControllerSecretAndPipe(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "proxy-secret.local")
	identityPath := filepath.Join(dir, "identity.local.yaml")

	if err := os.WriteFile(secretPath, []byte(strings.Join([]string{
		"CONTROLLER_SECRET=ctl-secret",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if err := os.WriteFile(identityPath, []byte(strings.Join([]string{
		"clash:",
		"  controller_pipe: \\\\.\\pipe\\custom-mihomo",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("write identity config: %v", err)
	}

	cfg, err := LoadConfig(LoadOptions{
		IdentityFile: identityPath,
		SecretFile:   secretPath,
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Clash.ControllerSecret != "ctl-secret" {
		t.Fatalf("controller secret = %q", cfg.Clash.ControllerSecret)
	}
	if cfg.Clash.ControllerPipe != `\\.\pipe\custom-mihomo` {
		t.Fatalf("controller pipe = %q", cfg.Clash.ControllerPipe)
	}
	found := false
	for _, secret := range cfg.SecretValues() {
		if secret == "ctl-secret" {
			found = true
		}
	}
	if !found {
		t.Fatalf("controller secret missing from SecretValues: %v", cfg.SecretValues())
	}
}

func TestLoadConfigReadsRuntimeConfig(t *testing.T) {
	if got := DefaultConfig().Clash.RuntimeConfig; got != "clash-verge.yaml" {
		t.Fatalf("default runtime config = %q", got)
	}

	dir := t.TempDir()
	secretPath := filepath.Join(dir, "proxy-secret.local")
	identityPath := filepath.Join(dir, "identity.local.yaml")
	if err := os.WriteFile(secretPath, []byte("CONTROLLER_SECRET=ctl-secret\n"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if err := os.WriteFile(identityPath, []byte(strings.Join([]string{
		"clash:",
		"  runtime_config: custom-runtime.yaml",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("write identity config: %v", err)
	}

	cfg, err := LoadConfig(LoadOptions{
		IdentityFile: identityPath,
		SecretFile:   secretPath,
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Clash.RuntimeConfig != "custom-runtime.yaml" {
		t.Fatalf("runtime config = %q", cfg.Clash.RuntimeConfig)
	}
}

func TestLoadConfigReadsKillSwitchConfig(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "proxy-secret.local")
	identityPath := filepath.Join(dir, "identity.local.yaml")
	if err := os.WriteFile(secretPath, []byte("CONTROLLER_SECRET=ctl-secret\n"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if err := os.WriteFile(identityPath, []byte(strings.Join([]string{
		"killswitch:",
		"  enabled: true",
		"  block_udp: true",
		"  appx: '*Claude*' # appcontainer package match",
		"  globs: '%LOCALAPPDATA%/AnthropicClaude/app-*/claude.exe'",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("write identity config: %v", err)
	}

	cfg, err := LoadConfig(LoadOptions{
		IdentityFile: identityPath,
		SecretFile:   secretPath,
		Env:          map[string]string{"LOCALAPPDATA": `C:\Users\test\AppData\Local`},
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if !cfg.KillSwitch.Enabled || !cfg.KillSwitch.BlockUDP {
		t.Fatalf("killswitch flags = %+v", cfg.KillSwitch)
	}
	if len(cfg.KillSwitch.Appx) != 1 || cfg.KillSwitch.Appx[0] != "*Claude*" {
		t.Fatalf("appx = %#v", cfg.KillSwitch.Appx)
	}
	if len(cfg.KillSwitch.Globs) != 1 || cfg.KillSwitch.Globs[0] != `C:\Users\test\AppData\Local/AnthropicClaude/app-*/claude.exe` {
		t.Fatalf("globs = %#v", cfg.KillSwitch.Globs)
	}
}

func TestDefaultConfigIncludesKillSwitchClaudeTargets(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.KillSwitch.Enabled {
		t.Fatal("killswitch should be opt-in by default")
	}
	if !cfg.KillSwitch.BlockUDP {
		t.Fatal("killswitch should default to UDP blocking when enabled")
	}
	if len(cfg.KillSwitch.Appx) != 1 || cfg.KillSwitch.Appx[0] != "*Claude*" {
		t.Fatalf("default appx = %#v", cfg.KillSwitch.Appx)
	}
	if len(cfg.KillSwitch.Globs) != 1 || !strings.Contains(cfg.KillSwitch.Globs[0], "AnthropicClaude") {
		t.Fatalf("default globs = %#v", cfg.KillSwitch.Globs)
	}
}

func TestRedactRemovesCredentialsAndProxyAuth(t *testing.T) {
	text := "user=static-user pass=static-pass Proxy-Authorization: Basic abc123 http://static-user:static-pass@example.test"

	redacted := Redact(text, []string{"static-user", "static-pass"})

	for _, forbidden := range []string{"static-user", "static-pass", "abc123"} {
		if strings.Contains(redacted, forbidden) {
			t.Fatalf("redacted text still contains %q: %s", forbidden, redacted)
		}
	}
	if !strings.Contains(redacted, "<redacted") {
		t.Fatalf("redacted marker missing: %s", redacted)
	}
}
