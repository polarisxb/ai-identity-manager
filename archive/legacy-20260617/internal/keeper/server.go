package keeper

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Server struct {
	cfg      Config
	upstream *upstreamDialer
	breaker  *circuitBreaker

	mu       sync.Mutex
	listener net.Listener
	closed   chan struct{}
	once     sync.Once
}

func NewServer(cfg Config) (*Server, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Server{
		cfg:      cfg,
		upstream: newUpstreamDialer(cfg),
		breaker:  newCircuitBreaker(cfg.Breaker),
		closed:   make(chan struct{}),
	}, nil
}

func (s *Server) Serve(listener net.Listener) error {
	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.closed:
				return nil
			default:
				return err
			}
		}
		go s.handleConn(conn)
	}
}

func (s *Server) Close() error {
	s.once.Do(func() {
		close(s.closed)
		s.mu.Lock()
		if s.listener != nil {
			_ = s.listener.Close()
		}
		s.mu.Unlock()
	})
	return nil
}

func (s *Server) handleConn(client net.Conn) {
	defer client.Close()
	_ = client.SetReadDeadline(time.Now().Add(s.cfg.ConnectTimeout))
	reader := bufio.NewReader(client)
	req, err := http.ReadRequest(reader)
	if err != nil {
		writeHTTPStatus(client, http.StatusBadRequest, "Bad Request")
		return
	}
	_ = client.SetReadDeadline(time.Time{})

	if req.Method != http.MethodConnect {
		writeHTTPStatus(client, http.StatusMethodNotAllowed, "CONNECT Only")
		return
	}
	target := normalizeConnectTarget(req)
	if target == "" {
		writeHTTPStatus(client, http.StatusBadRequest, "Missing CONNECT Target")
		return
	}

	if !s.breaker.allow() {
		writeHTTPStatus(client, http.StatusServiceUnavailable, "Circuit Open")
		return
	}

	upstream, err := s.upstream.dialConnect(context.Background(), target)
	if err != nil {
		s.breaker.failure()
		if errors.Is(err, errCircuitOpen) {
			writeHTTPStatus(client, http.StatusServiceUnavailable, "Circuit Open")
			return
		}
		writeHTTPStatus(client, http.StatusBadGateway, "Bad Gateway")
		return
	}
	defer upstream.Close()
	s.breaker.success()

	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	_ = client.SetDeadline(time.Time{})
	_ = upstream.SetDeadline(time.Time{})
	tunnel(client, upstream, reader)
}

func normalizeConnectTarget(req *http.Request) string {
	target := req.Host
	if target == "" {
		target = req.RequestURI
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	if !strings.Contains(target, ":") {
		target += ":443"
	}
	return target
}

func writeHTTPStatus(conn net.Conn, code int, text string) {
	if text == "" {
		text = http.StatusText(code)
	}
	body := fmt.Sprintf("%d %s\n", code, text)
	_, _ = fmt.Fprintf(conn, "HTTP/1.1 %d %s\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", code, text, len(body), body)
}

func tunnel(client net.Conn, upstream net.Conn, bufferedClient *bufio.Reader) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if bufferedClient.Buffered() > 0 {
			_, _ = io.CopyN(upstream, bufferedClient, int64(bufferedClient.Buffered()))
		}
		_, _ = io.Copy(upstream, client)
		closeWrite(upstream)
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(client, upstream)
		closeWrite(client)
	}()

	wg.Wait()
}

func closeWrite(conn net.Conn) {
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
		return
	}
	_ = conn.Close()
}
