//go:build windows

package controller

import (
	"net"
	"testing"
)

func TestPipeConnImplementsNetConn(t *testing.T) {
	var _ net.Conn = (*pipeConn)(nil)
}
