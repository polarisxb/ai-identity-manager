package controller

import (
	"context"
	"net"
	"time"
)

func newTCPController(ep Endpoint) *httpController {
	addr := ep.TCPAddr
	dial := func(ctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", addr)
	}
	return newHTTPController("tcp", addr, ep.Secret, 3*time.Second, dial)
}
