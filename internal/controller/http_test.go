package controller

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestController(t *testing.T, secret string, h http.HandlerFunc) *httpController {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	addr := strings.TrimPrefix(srv.URL, "http://")
	dial := func(ctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", addr)
	}
	return newHTTPController("tcp", addr, secret, 2*time.Second, dial)
}

func TestPingParsesVersion(t *testing.T) {
	c := newTestController(t, "", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":true,"version":"v1.19.20"}`))
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	v, err := c.Ping(ctx)
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if v.Version != "v1.19.20" || !v.Meta {
		t.Fatalf("version = %+v", v)
	}
	if c.Transport() != "tcp" {
		t.Fatalf("transport = %q", c.Transport())
	}
}

func TestConnectionsParsesChainsAndMetadata(t *testing.T) {
	c := newTestController(t, "s", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer s" {
			t.Errorf("auth = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"connections":[
			{"id":"1","chains":["IPRoyal-Static","AI-Static"],"rule":"ProcessName",
			 "upload":10,"download":20,"start":"2026-06-18T13:46:05.7+08:00",
			 "metadata":{"network":"tcp","type":"HTTPS","host":"api.anthropic.com",
			 "process":"claude.exe","processPath":"C:\\claude.exe","destinationIP":"1.2.3.4","destinationIPASN":"AS1"}}
		],"downloadTotal":1,"uploadTotal":2}`))
	})
	ctx := context.Background()
	conns, err := c.Connections(ctx)
	if err != nil {
		t.Fatalf("Connections: %v", err)
	}
	if len(conns) != 1 {
		t.Fatalf("len = %d", len(conns))
	}
	got := conns[0]
	if got.Metadata.Host != "api.anthropic.com" || got.Metadata.Process != "claude.exe" {
		t.Fatalf("metadata = %+v", got.Metadata)
	}
	if len(got.Chains) != 2 || got.Chains[0] != "IPRoyal-Static" {
		t.Fatalf("chains = %v; chains[0] should be the egress node", got.Chains)
	}
}

func TestRuntimeConfigParsesMode(t *testing.T) {
	c := newTestController(t, "", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q", r.Method)
		}
		if r.URL.Path != "/configs" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"mode":"rule","log-level":"info"}`))
	})
	cfg, err := c.RuntimeConfig(context.Background())
	if err != nil {
		t.Fatalf("RuntimeConfig: %v", err)
	}
	if cfg.Mode != "rule" {
		t.Fatalf("mode = %q", cfg.Mode)
	}
}

func TestReloadRuntimeSendsForceAndPath(t *testing.T) {
	var gotMethod, gotQuery, gotBody string
	c := newTestController(t, "", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotQuery = r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	})
	if err := c.ReloadRuntime(context.Background(), `C:\clash-verge.yaml`); err != nil {
		t.Fatalf("ReloadRuntime: %v", err)
	}
	if gotMethod != http.MethodPut || gotQuery != "force=true" {
		t.Fatalf("method=%q query=%q", gotMethod, gotQuery)
	}
	if !strings.Contains(gotBody, `"path"`) || !strings.Contains(gotBody, `clash-verge.yaml`) {
		t.Fatalf("body=%q", gotBody)
	}
}

func TestUnauthorizedMappedToErrUnauthorized(t *testing.T) {
	c := newTestController(t, "wrong", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	if _, err := c.Connections(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestSecretNotLeakedInErrors(t *testing.T) {
	const secret = "s3cr3t-token-15"
	c := newTestController(t, secret, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	_, err := c.Connections(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked secret: %v", err)
	}
}

type deadlinelessConn struct {
	net.Conn
}

func (deadlinelessConn) SetDeadline(time.Time) error {
	return nil
}

func (deadlinelessConn) SetReadDeadline(time.Time) error {
	return nil
}

func (deadlinelessConn) SetWriteDeadline(time.Time) error {
	return nil
}

func TestDoSucceedsOverDeadlinelessBlockingConn(t *testing.T) {
	clientRaw, server := net.Pipe()
	client := deadlinelessConn{Conn: clientRaw}
	go func() {
		defer server.Close()
		if _, err := http.ReadRequest(bufio.NewReader(server)); err != nil {
			return
		}
		body := `{"meta":true,"version":"v1.19.20"}`
		fmt.Fprintf(server, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
	}()
	c := newHTTPController("pipe", "verge-mihomo", "", 2*time.Second,
		func(ctx context.Context) (net.Conn, error) { return client, nil })

	v, err := c.Ping(context.Background())

	if err != nil {
		t.Fatalf("Ping over deadline-less conn: %v", err)
	}
	if v.Version != "v1.19.20" || !v.Meta {
		t.Fatalf("version=%+v", v)
	}
}

func TestConnectionsHandlesChunkedResponse(t *testing.T) {
	c := newTestController(t, "", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fl, ok := w.(http.Flusher)
		if !ok {
			t.Error("ResponseWriter is not a Flusher")
			return
		}
		io.WriteString(w, `{"connections":[`)
		fl.Flush()
		io.WriteString(w, `{"id":"1","chains":["IPRoyal-Static","AI-Static"],"metadata":{"host":"api.anthropic.com"}}`)
		io.WriteString(w, `]}`)
	})

	conns, err := c.Connections(context.Background())

	if err != nil {
		t.Fatalf("Connections (chunked): %v", err)
	}
	if len(conns) != 1 || conns[0].Metadata.Host != "api.anthropic.com" {
		t.Fatalf("conns=%+v", conns)
	}
}
