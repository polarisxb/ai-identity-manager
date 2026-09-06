//go:build windows

package controller

import (
	"context"
	"net"
	"os"
	"time"
)

func newPipeController(ep Endpoint) (*httpController, error) {
	path := ep.PipePath
	dial := func(ctx context.Context) (net.Conn, error) {
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			return nil, err
		}
		return &pipeConn{f: f}, nil
	}
	return newHTTPController("pipe", "verge-mihomo", ep.Secret, 3*time.Second, dial), nil
}

type pipeConn struct {
	f *os.File
}

func (p *pipeConn) Read(b []byte) (int, error) {
	return p.f.Read(b)
}

func (p *pipeConn) Write(b []byte) (int, error) {
	return p.f.Write(b)
}

func (p *pipeConn) Close() error {
	return p.f.Close()
}

func (p *pipeConn) LocalAddr() net.Addr {
	return pipeAddr{}
}

func (p *pipeConn) RemoteAddr() net.Addr {
	return pipeAddr{}
}

func (p *pipeConn) SetDeadline(time.Time) error {
	return nil
}

func (p *pipeConn) SetReadDeadline(time.Time) error {
	return nil
}

func (p *pipeConn) SetWriteDeadline(time.Time) error {
	return nil
}

type pipeAddr struct{}

func (pipeAddr) Network() string {
	return "pipe"
}

func (pipeAddr) String() string {
	return "verge-mihomo"
}
