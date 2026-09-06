package keeper

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestServerTunnelsConnectThroughUpstream(t *testing.T) {
	upstream := newFakeUpstream(t, respondOKAndEcho(t))
	server := startTestServer(t, Config{
		Upstream:       UpstreamConfig{Address: upstream.addr},
		ConnectTimeout: 2 * time.Second,
		Retry:          RetryConfig{SetupRetries: 1},
		Breaker:        CircuitBreakerConfig{FailureThreshold: 3, OpenDuration: time.Second},
	})

	conn, status := dialLocalConnect(t, server.addr, "claude.ai:443")
	if status != "HTTP/1.1 200 Connection Established" {
		t.Fatalf("unexpected status: %s", status)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write tunnel payload: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("unexpected echo: %q", string(buf))
	}
	if got := upstream.attempts.Load(); got != 1 {
		t.Fatalf("upstream attempts = %d, want 1", got)
	}
}

func TestServerDialsUpstreamThroughBootstrapProxy(t *testing.T) {
	upstreamAddr := "ipr.example:12323"
	bootstrap := newFakeUpstream(t, respondBootstrapThenUpstreamOK(t, upstreamAddr, "claude.ai:443"))
	server := startTestServer(t, Config{
		Upstream:       UpstreamConfig{Address: upstreamAddr, BootstrapProxy: "http://" + bootstrap.addr},
		ConnectTimeout: 2 * time.Second,
		Retry:          RetryConfig{SetupRetries: 1},
		Breaker:        CircuitBreakerConfig{FailureThreshold: 3, OpenDuration: time.Second},
	})

	conn, status := dialLocalConnect(t, server.addr, "claude.ai:443")
	if status != "HTTP/1.1 200 Connection Established" {
		t.Fatalf("unexpected status: %s", status)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write tunnel payload: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("unexpected echo: %q", string(buf))
	}
	if got := bootstrap.attempts.Load(); got != 1 {
		t.Fatalf("bootstrap attempts = %d, want 1", got)
	}
}

func TestServerRetriesOnceWhenUpstreamClosesBeforeResponse(t *testing.T) {
	upstream := newFakeUpstream(t, closeImmediately(), respondOKAndEcho(t))
	server := startTestServer(t, Config{
		Upstream:       UpstreamConfig{Address: upstream.addr},
		ConnectTimeout: 2 * time.Second,
		Retry:          RetryConfig{SetupRetries: 1},
		Breaker:        CircuitBreakerConfig{FailureThreshold: 3, OpenDuration: time.Second},
	})

	conn, status := dialLocalConnect(t, server.addr, "claude.ai:443")
	if status != "HTTP/1.1 200 Connection Established" {
		t.Fatalf("unexpected status after retry: %s", status)
	}
	defer conn.Close()

	if got := upstream.attempts.Load(); got != 2 {
		t.Fatalf("upstream attempts = %d, want 2", got)
	}
}

func TestServerRetriesOnceOnUpstreamBadGateway(t *testing.T) {
	upstream := newFakeUpstream(t, respondStatus(t, 502, "Bad Gateway"), respondOKAndEcho(t))
	server := startTestServer(t, Config{
		Upstream:       UpstreamConfig{Address: upstream.addr},
		ConnectTimeout: 2 * time.Second,
		Retry:          RetryConfig{SetupRetries: 1},
		Breaker:        CircuitBreakerConfig{FailureThreshold: 3, OpenDuration: time.Second},
	})

	conn, status := dialLocalConnect(t, server.addr, "claude.ai:443")
	if status != "HTTP/1.1 200 Connection Established" {
		t.Fatalf("unexpected status after 502 retry: %s", status)
	}
	_ = conn.Close()

	if got := upstream.attempts.Load(); got != 2 {
		t.Fatalf("upstream attempts = %d, want 2", got)
	}
}

func TestServerCircuitBreakerStopsDialingAfterFailures(t *testing.T) {
	upstream := newFakeUpstream(t, closeImmediately(), closeImmediately())
	server := startTestServer(t, Config{
		Upstream:       UpstreamConfig{Address: upstream.addr},
		ConnectTimeout: 200 * time.Millisecond,
		Retry:          RetryConfig{SetupRetries: 1},
		Breaker:        CircuitBreakerConfig{FailureThreshold: 1, OpenDuration: time.Minute},
	})

	conn, status := dialLocalConnect(t, server.addr, "claude.ai:443")
	_ = conn.Close()
	if !strings.HasPrefix(status, "HTTP/1.1 502 ") {
		t.Fatalf("first failure status = %s, want local 502", status)
	}
	if got := upstream.attempts.Load(); got != 2 {
		t.Fatalf("attempts after first request = %d, want 2", got)
	}

	conn, status = dialLocalConnect(t, server.addr, "claude.ai:443")
	_ = conn.Close()
	if !strings.HasPrefix(status, "HTTP/1.1 503 ") {
		t.Fatalf("second failure status = %s, want circuit-open 503", status)
	}
	if got := upstream.attempts.Load(); got != 2 {
		t.Fatalf("attempts after circuit open = %d, want still 2", got)
	}
}

type testServer struct {
	addr string
	srv  *Server
}

func startTestServer(t *testing.T, cfg Config) testServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen local server: %v", err)
	}
	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- srv.Serve(ln)
	}()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = ln.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("server did not stop")
		}
	})
	return testServer{addr: ln.Addr().String(), srv: srv}
}

func dialLocalConnect(t *testing.T, addr, target string) (net.Conn, string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial local keeper: %v", err)
	}
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	if _, err := conn.Write([]byte(req)); err != nil {
		_ = conn.Close()
		t.Fatalf("write local CONNECT: %v", err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		_ = conn.Close()
		t.Fatalf("read local response: %v", err)
	}
	return conn, strings.TrimSpace(line)
}

type fakeUpstream struct {
	addr     string
	ln       net.Listener
	handlers []func(net.Conn)
	attempts atomic.Int64
}

func newFakeUpstream(t *testing.T, handlers ...func(net.Conn)) *fakeUpstream {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen fake upstream: %v", err)
	}
	f := &fakeUpstream{addr: ln.Addr().String(), ln: ln, handlers: handlers}
	go f.serve()
	t.Cleanup(func() {
		_ = ln.Close()
	})
	return f
}

func (f *fakeUpstream) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		attempt := int(f.attempts.Add(1)) - 1
		handler := closeImmediately()
		if attempt < len(f.handlers) {
			handler = f.handlers[attempt]
		}
		go handler(conn)
	}
}

func closeImmediately() func(net.Conn) {
	return func(conn net.Conn) {
		_ = conn.Close()
	}
}

func respondStatus(t *testing.T, code int, text string) func(net.Conn) {
	t.Helper()
	return func(conn net.Conn) {
		defer conn.Close()
		_, _ = http.ReadRequest(bufio.NewReader(conn))
		_, _ = fmt.Fprintf(conn, "HTTP/1.1 %d %s\r\nContent-Length: 0\r\n\r\n", code, text)
	}
}

func respondOKAndEcho(t *testing.T) func(net.Conn) {
	t.Helper()
	return func(conn net.Conn) {
		defer conn.Close()
		reader := bufio.NewReader(conn)
		req, err := http.ReadRequest(reader)
		if err != nil {
			t.Errorf("fake upstream read CONNECT: %v", err)
			return
		}
		if req.Method != http.MethodConnect {
			t.Errorf("fake upstream method = %s, want CONNECT", req.Method)
			return
		}
		if _, err := conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
			return
		}
		buf := make([]byte, 4)
		n, err := reader.Read(buf)
		if err != nil {
			return
		}
		_, _ = conn.Write(buf[:n])
	}
}

func respondBootstrapThenUpstreamOK(t *testing.T, upstreamTarget, finalTarget string) func(net.Conn) {
	t.Helper()
	return func(conn net.Conn) {
		defer conn.Close()
		reader := bufio.NewReader(conn)

		req, err := http.ReadRequest(reader)
		if err != nil {
			t.Errorf("bootstrap read CONNECT: %v", err)
			return
		}
		if req.Method != http.MethodConnect {
			t.Errorf("bootstrap method = %s, want CONNECT", req.Method)
			return
		}
		if req.Host != upstreamTarget {
			t.Errorf("bootstrap target = %s, want %s", req.Host, upstreamTarget)
			return
		}
		if _, err := conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
			return
		}

		req, err = http.ReadRequest(reader)
		if err != nil {
			t.Errorf("upstream read CONNECT over bootstrap tunnel: %v", err)
			return
		}
		if req.Method != http.MethodConnect {
			t.Errorf("upstream method = %s, want CONNECT", req.Method)
			return
		}
		if req.Host != finalTarget {
			t.Errorf("upstream target = %s, want %s", req.Host, finalTarget)
			return
		}
		if _, err := conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
			return
		}

		buf := make([]byte, 4)
		n, err := reader.Read(buf)
		if err != nil {
			return
		}
		_, _ = conn.Write(buf[:n])
	}
}
