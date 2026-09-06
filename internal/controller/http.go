package controller

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

type dialFunc func(ctx context.Context) (net.Conn, error)

type httpController struct {
	transport string
	host      string
	secret    string
	timeout   time.Duration
	dial      dialFunc
}

func newHTTPController(transport, host, secret string, timeout time.Duration, dial dialFunc) *httpController {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &httpController{transport: transport, host: host, secret: secret, timeout: timeout, dial: dial}
}

func (c *httpController) Transport() string { return c.transport }

func (c *httpController) Close() error { return nil }

// do avoids net/http.Transport because Windows named-pipe deadlines are no-op.
// A watchdog closes the conn on timeout so blocking reads are unblocked.
func (c *httpController) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	stop := context.AfterFunc(reqCtx, func() { _ = conn.Close() })
	var once sync.Once
	teardown := func() {
		once.Do(func() {
			stop()
			cancel()
			_ = conn.Close()
		})
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s %s HTTP/1.1\r\n", method, path)
	fmt.Fprintf(&buf, "Host: %s\r\n", c.host)
	buf.WriteString("Accept-Encoding: identity\r\n")
	if c.secret != "" {
		fmt.Fprintf(&buf, "Authorization: Bearer %s\r\n", c.secret)
	}
	if body != nil {
		buf.WriteString("Content-Type: application/json\r\n")
		fmt.Fprintf(&buf, "Content-Length: %d\r\n", len(body))
	}
	buf.WriteString("Connection: close\r\n\r\n")
	if body != nil {
		buf.Write(body)
	}

	if _, err := conn.Write(buf.Bytes()); err != nil {
		teardown()
		return nil, fmt.Errorf("controller %s %s: write: %w", method, path, err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		teardown()
		return nil, fmt.Errorf("controller %s %s: read: %w", method, path, err)
	}
	resp.Body = &connBody{ReadCloser: resp.Body, teardown: teardown}

	if resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()
		return nil, ErrUnauthorized
	}
	if resp.StatusCode/100 != 2 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("controller %s %s: status %d", method, path, resp.StatusCode)
	}
	return resp, nil
}

type connBody struct {
	io.ReadCloser
	teardown func()
}

func (b *connBody) Close() error {
	err := b.ReadCloser.Close()
	b.teardown()
	return err
}

func (c *httpController) Ping(ctx context.Context) (Version, error) {
	resp, err := c.do(ctx, http.MethodGet, "/version", nil)
	if err != nil {
		return Version{}, err
	}
	defer resp.Body.Close()
	var dto struct {
		Meta    bool   `json:"meta"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		return Version{}, err
	}
	return Version{Version: dto.Version, Meta: dto.Meta}, nil
}

func (c *httpController) Connections(ctx context.Context) ([]Connection, error) {
	resp, err := c.do(ctx, http.MethodGet, "/connections", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var dto struct {
		Connections []struct {
			ID       string   `json:"id"`
			Chains   []string `json:"chains"`
			Rule     string   `json:"rule"`
			Upload   int64    `json:"upload"`
			Download int64    `json:"download"`
			Start    string   `json:"start"`
			Metadata struct {
				Network          string `json:"network"`
				Type             string `json:"type"`
				Host             string `json:"host"`
				Process          string `json:"process"`
				ProcessPath      string `json:"processPath"`
				DestinationIP    string `json:"destinationIP"`
				DestinationIPASN string `json:"destinationIPASN"`
			} `json:"metadata"`
		} `json:"connections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		return nil, err
	}
	conns := make([]Connection, 0, len(dto.Connections))
	for _, in := range dto.Connections {
		var start time.Time
		if in.Start != "" {
			start, _ = time.Parse(time.RFC3339Nano, in.Start)
		}
		conns = append(conns, Connection{
			ID:       in.ID,
			Chains:   in.Chains,
			Rule:     in.Rule,
			Upload:   in.Upload,
			Download: in.Download,
			Start:    start,
			Metadata: Metadata{
				Network:          in.Metadata.Network,
				Type:             in.Metadata.Type,
				Host:             in.Metadata.Host,
				Process:          in.Metadata.Process,
				ProcessPath:      in.Metadata.ProcessPath,
				DestinationIP:    in.Metadata.DestinationIP,
				DestinationIPASN: in.Metadata.DestinationIPASN,
			},
		})
	}
	return conns, nil
}

func (c *httpController) RuntimeConfig(ctx context.Context) (RuntimeConfig, error) {
	resp, err := c.do(ctx, http.MethodGet, "/configs", nil)
	if err != nil {
		return RuntimeConfig{}, err
	}
	defer resp.Body.Close()
	var dto struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		return RuntimeConfig{}, err
	}
	return RuntimeConfig{Mode: dto.Mode}, nil
}

func (c *httpController) ReloadRuntime(ctx context.Context, configPath string) error {
	body, err := json.Marshal(struct {
		Path string `json:"path"`
	}{Path: configPath})
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, http.MethodPut, "/configs?force=true", body)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
