package keeper

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

type Config struct {
	ListenAddr     string
	Upstream       UpstreamConfig
	ConnectTimeout time.Duration
	Retry          RetryConfig
	Breaker        CircuitBreakerConfig
}

type UpstreamConfig struct {
	Address        string
	Username       string
	Password       string
	BootstrapProxy string
}

type RetryConfig struct {
	SetupRetries int
}

type CircuitBreakerConfig struct {
	FailureThreshold int
	OpenDuration     time.Duration
}

func (c Config) withDefaults() Config {
	if c.ListenAddr == "" {
		c.ListenAddr = "127.0.0.1:18080"
	}
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 10 * time.Second
	}
	if c.Retry.SetupRetries < 0 {
		c.Retry.SetupRetries = 0
	}
	if c.Breaker.FailureThreshold <= 0 {
		c.Breaker.FailureThreshold = 3
	}
	if c.Breaker.OpenDuration <= 0 {
		c.Breaker.OpenDuration = 5 * time.Second
	}
	return c
}

func (c Config) validate() error {
	if c.Upstream.Address == "" {
		return fmt.Errorf("upstream address is required")
	}
	if _, _, err := net.SplitHostPort(c.Upstream.Address); err != nil {
		return fmt.Errorf("invalid upstream address %q: %w", c.Upstream.Address, err)
	}
	return nil
}

func LoadConfigFile(path string) (Config, error) {
	values, err := readKeyValueFile(path)
	if err != nil {
		return Config{}, err
	}
	host := strings.TrimSpace(values["PROXY_HOST"])
	port := strings.TrimSpace(values["PROXY_HTTP_PORT"])
	if host == "" {
		return Config{}, fmt.Errorf("PROXY_HOST is required")
	}
	if port == "" {
		return Config{}, fmt.Errorf("PROXY_HTTP_PORT is required")
	}
	cfg := Config{
		Upstream: UpstreamConfig{
			Address:        net.JoinHostPort(host, port),
			Username:       values["PROXY_USER"],
			Password:       values["PROXY_PASS"],
			BootstrapProxy: strings.TrimSpace(values["BOOTSTRAP_PROXY"]),
		},
	}
	return cfg.withDefaults(), nil
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
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}
