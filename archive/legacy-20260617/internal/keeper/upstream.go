package keeper

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type upstreamDialer struct {
	cfg            UpstreamConfig
	connectTimeout time.Duration
	setupRetries   int
}

func newUpstreamDialer(cfg Config) *upstreamDialer {
	return &upstreamDialer{
		cfg:            cfg.Upstream,
		connectTimeout: cfg.ConnectTimeout,
		setupRetries:   cfg.Retry.SetupRetries,
	}
}

func (d *upstreamDialer) dialConnect(ctx context.Context, target string) (net.Conn, error) {
	attempts := d.setupRetries + 1
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		conn, err := d.dialConnectOnce(ctx, target)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if !isRetryableSetupError(err) || attempt == attempts-1 {
			break
		}
	}
	return nil, lastErr
}

func (d *upstreamDialer) dialConnectOnce(ctx context.Context, target string) (net.Conn, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, d.connectTimeout)
	defer cancel()

	conn, err := d.dialUpstreamTransport(attemptCtx)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = conn.Close()
		}
	}()

	if deadline, ok := attemptCtx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
		defer conn.SetDeadline(time.Time{})
	}

	if err := writeUpstreamConnect(conn, target, d.cfg); err != nil {
		return nil, err
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, upstreamStatusError{code: resp.StatusCode, status: resp.Status}
	}
	success = true
	return conn, nil
}

func (d *upstreamDialer) dialUpstreamTransport(ctx context.Context) (net.Conn, error) {
	if strings.TrimSpace(d.cfg.BootstrapProxy) == "" {
		dialer := net.Dialer{Timeout: d.connectTimeout}
		return dialer.DialContext(ctx, "tcp", d.cfg.Address)
	}
	return dialViaHTTPBootstrapProxy(ctx, d.cfg.BootstrapProxy, d.cfg.Address, d.connectTimeout)
}

func dialViaHTTPBootstrapProxy(ctx context.Context, proxyURL string, target string, timeout time.Duration) (net.Conn, error) {
	if !strings.Contains(proxyURL, "://") {
		proxyURL = "http://" + proxyURL
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" {
		return nil, fmt.Errorf("unsupported bootstrap proxy scheme %q", parsed.Scheme)
	}
	proxyAddr := parsed.Host
	if _, _, err := net.SplitHostPort(proxyAddr); err != nil {
		proxyAddr = net.JoinHostPort(proxyAddr, "80")
	}

	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = conn.Close()
		}
	}()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if _, err := fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n", target, target); err != nil {
		return nil, err
	}
	if parsed.User != nil {
		password, _ := parsed.User.Password()
		token := base64.StdEncoding.EncodeToString([]byte(parsed.User.Username() + ":" + password))
		if _, err := fmt.Fprintf(conn, "Proxy-Authorization: Basic %s\r\n", token); err != nil {
			return nil, err
		}
	}
	if _, err := fmt.Fprint(conn, "Proxy-Connection: Keep-Alive\r\n\r\n"); err != nil {
		return nil, err
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, upstreamStatusError{code: resp.StatusCode, status: "bootstrap " + resp.Status}
	}
	success = true
	return conn, nil
}

func writeUpstreamConnect(conn net.Conn, target string, cfg UpstreamConfig) error {
	if _, err := fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n", target, target); err != nil {
		return err
	}
	if cfg.Username != "" || cfg.Password != "" {
		token := base64.StdEncoding.EncodeToString([]byte(cfg.Username + ":" + cfg.Password))
		if _, err := fmt.Fprintf(conn, "Proxy-Authorization: Basic %s\r\n", token); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(conn, "Proxy-Connection: Keep-Alive\r\n\r\n")
	return err
}
