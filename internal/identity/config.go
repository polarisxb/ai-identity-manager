package identity

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type LoadOptions struct {
	IdentityFile string
	SecretFile   string
	ClashDir     string
	Env          map[string]string
}

type Config struct {
	Name       string
	IPRoyal    IPRoyalConfig
	Expected   ExpectedIdentity
	Clash      ClashConfig
	Nodes      NodeConfig
	Bootstrap  BootstrapConfig
	Watch      WatchConfig
	KillSwitch KillSwitchConfig
}

type IPRoyalConfig struct {
	Host       string
	HTTPPort   string
	SOCKSPort  string
	Username   string
	Password   string
	ProxyMode  string
	SecretFile string
}

type ExpectedIdentity struct {
	ExitIP  string
	ASN     string
	Country string
}

type ClashConfig struct {
	AppDir           string
	MixedPort        string
	Controller       string
	ControllerPipe   string
	ControllerSecret string
	RuntimeConfig    string
}

type NodeConfig struct {
	StaticNode     string
	FinalGroup     string
	BootstrapGroup string
}

type BootstrapConfig struct {
	Candidates           []string
	SwitchAfterFailures  int
	LastSelectedNodeName string
}

type WatchConfig struct {
	PollInterval   time.Duration
	VerifyInterval time.Duration
}

type KillSwitchConfig struct {
	Enabled  bool
	Appx     []string
	Globs    []string
	BlockUDP bool
}

func DefaultConfig() Config {
	return Config{
		Name: "ai-static-iproyal",
		IPRoyal: IPRoyalConfig{
			ProxyMode:  "http",
			SecretFile: "proxy-secret.local",
		},
		Clash: ClashConfig{
			AppDir:         "%APPDATA%/io.github.clash-verge-rev.clash-verge-rev",
			MixedPort:      "127.0.0.1:7889",
			Controller:     "127.0.0.1:9097",
			ControllerPipe: `\\.\pipe\verge-mihomo`,
			RuntimeConfig:  "clash-verge.yaml",
		},
		Nodes: NodeConfig{
			StaticNode:     "IPRoyal-Static",
			FinalGroup:     "AI-Static",
			BootstrapGroup: "JMS-Bootstrap",
		},
		Bootstrap: BootstrapConfig{
			SwitchAfterFailures: 3,
		},
		Watch: WatchConfig{
			PollInterval: 5 * time.Second,
		},
		KillSwitch: KillSwitchConfig{
			Appx:     []string{"*Claude*"},
			Globs:    []string{"%LOCALAPPDATA%/AnthropicClaude/app-*/claude.exe"},
			BlockUDP: true,
		},
	}
}

func LoadConfig(opts LoadOptions) (Config, error) {
	cfg := DefaultConfig()
	env := opts.Env
	if env == nil {
		env = processEnv()
	}

	if opts.IdentityFile != "" {
		values, err := readSimpleYAMLFile(opts.IdentityFile)
		if err != nil {
			return Config{}, err
		}
		applyYAMLValues(&cfg, values)
		if cfg.IPRoyal.SecretFile != "" && !filepath.IsAbs(cfg.IPRoyal.SecretFile) {
			cfg.IPRoyal.SecretFile = filepath.Join(filepath.Dir(opts.IdentityFile), cfg.IPRoyal.SecretFile)
		}
	}
	if opts.SecretFile != "" {
		cfg.IPRoyal.SecretFile = opts.SecretFile
	}
	if opts.ClashDir != "" {
		cfg.Clash.AppDir = opts.ClashDir
	}
	cfg.Clash.AppDir = expandPercentEnv(cfg.Clash.AppDir, env)
	for i, glob := range cfg.KillSwitch.Globs {
		cfg.KillSwitch.Globs[i] = expandPercentEnv(glob, env)
	}

	if cfg.IPRoyal.SecretFile != "" {
		values, err := readKeyValueFile(cfg.IPRoyal.SecretFile)
		if err != nil {
			return Config{}, err
		}
		applySecretValues(&cfg, values)
	}

	return cfg, cfg.validate()
}

func (c Config) validate() error {
	if c.Nodes.StaticNode == "" || c.Nodes.FinalGroup == "" || c.Nodes.BootstrapGroup == "" {
		return fmt.Errorf("node names are required")
	}
	if c.IPRoyal.ProxyMode == "" {
		return fmt.Errorf("iproyal proxy mode is required")
	}
	return nil
}

func applySecretValues(cfg *Config, values map[string]string) {
	cfg.IPRoyal.Host = firstNonEmpty(values["PROXY_HOST"], cfg.IPRoyal.Host)
	cfg.IPRoyal.HTTPPort = firstNonEmpty(values["PROXY_HTTP_PORT"], cfg.IPRoyal.HTTPPort)
	cfg.IPRoyal.SOCKSPort = firstNonEmpty(values["PROXY_SOCKS_PORT"], cfg.IPRoyal.SOCKSPort)
	cfg.IPRoyal.Username = firstNonEmpty(values["PROXY_USER"], cfg.IPRoyal.Username)
	cfg.IPRoyal.Password = firstNonEmpty(values["PROXY_PASS"], cfg.IPRoyal.Password)
	cfg.Expected.ExitIP = firstNonEmpty(values["EXPECTED_EXIT_IP"], cfg.Expected.ExitIP)
	cfg.Expected.ASN = firstNonEmpty(values["EXPECTED_ASN"], cfg.Expected.ASN)
	cfg.Expected.Country = firstNonEmpty(values["EXPECTED_COUNTRY"], cfg.Expected.Country)
	cfg.Clash.ControllerSecret = firstNonEmpty(values["CONTROLLER_SECRET"], cfg.Clash.ControllerSecret)
}

func applyYAMLValues(cfg *Config, values map[string]string) {
	cfg.Name = firstNonEmpty(values["identity.name"], cfg.Name)
	cfg.Expected.ExitIP = firstNonEmpty(values["identity.expected_exit_ip"], cfg.Expected.ExitIP)
	cfg.Expected.ASN = firstNonEmpty(values["identity.expected_asn"], cfg.Expected.ASN)
	cfg.Expected.Country = firstNonEmpty(values["identity.expected_country"], cfg.Expected.Country)
	cfg.IPRoyal.SecretFile = firstNonEmpty(values["iproyal.secret_file"], cfg.IPRoyal.SecretFile)
	cfg.IPRoyal.ProxyMode = firstNonEmpty(values["iproyal.proxy_mode"], cfg.IPRoyal.ProxyMode)
	cfg.Nodes.StaticNode = firstNonEmpty(values["iproyal.node_name"], cfg.Nodes.StaticNode)
	cfg.Clash.AppDir = firstNonEmpty(values["clash.app_dir"], cfg.Clash.AppDir)
	cfg.Clash.MixedPort = firstNonEmpty(values["clash.mixed_port"], cfg.Clash.MixedPort)
	cfg.Clash.Controller = firstNonEmpty(values["clash.controller"], cfg.Clash.Controller)
	cfg.Clash.ControllerPipe = firstNonEmpty(values["clash.controller_pipe"], cfg.Clash.ControllerPipe)
	cfg.Clash.RuntimeConfig = firstNonEmpty(values["clash.runtime_config"], cfg.Clash.RuntimeConfig)
	cfg.Nodes.FinalGroup = firstNonEmpty(values["clash.final_group"], cfg.Nodes.FinalGroup)
	cfg.Nodes.BootstrapGroup = firstNonEmpty(values["clash.bootstrap_group"], cfg.Nodes.BootstrapGroup)
	if v := values["bootstrap.switch_after_failures"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Bootstrap.SwitchAfterFailures = n
		}
	}
	if v := values["watch.poll_interval"]; v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Watch.PollInterval = d
		}
	}
	if v := values["watch.verify_interval"]; v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Watch.VerifyInterval = d
		}
	}
	if v := values["killswitch.enabled"]; v != "" {
		cfg.KillSwitch.Enabled = parseConfigBool(v)
	}
	if v := values["killswitch.block_udp"]; v != "" {
		cfg.KillSwitch.BlockUDP = parseConfigBool(v)
	}
	if v := values["killswitch.appx"]; v != "" {
		cfg.KillSwitch.Appx = []string{v}
	}
	if v := values["killswitch.globs"]; v != "" {
		cfg.KillSwitch.Globs = []string{v}
	}
}

func readSimpleYAMLFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(raw, " ") && strings.HasSuffix(line, ":") {
			section = strings.TrimSuffix(line, ":")
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = trimConfigValue(value)
		if section != "" {
			key = section + "." + key
		}
		values[key] = value
	}
	return values, scanner.Err()
}

func readKeyValueFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return values, scanner.Err()
}

func trimConfigValue(value string) string {
	if i := strings.Index(value, "#"); i >= 0 {
		value = value[:i]
	}
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return value
}

func parseConfigBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func firstNonEmpty(first, fallback string) string {
	if strings.TrimSpace(first) != "" {
		return strings.TrimSpace(first)
	}
	return fallback
}

func processEnv() map[string]string {
	env := map[string]string{}
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}

func expandPercentEnv(path string, env map[string]string) string {
	for key, value := range env {
		path = strings.ReplaceAll(path, "%"+key+"%", value)
	}
	return path
}
