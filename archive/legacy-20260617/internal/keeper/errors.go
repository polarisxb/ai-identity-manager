package keeper

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

var errCircuitOpen = errors.New("circuit breaker is open")

type upstreamStatusError struct {
	code   int
	status string
}

func (e upstreamStatusError) Error() string {
	return fmt.Sprintf("upstream proxy returned %s", e.status)
}

func isRetryableSetupError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var statusErr upstreamStatusError
	if errors.As(err, &statusErr) {
		return statusErr.code == 502 || statusErr.code == 503 || statusErr.code == 504
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "forcibly closed") ||
		strings.Contains(msg, "connection was aborted") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "wsarecv") ||
		strings.Contains(msg, "wsasend") ||
		strings.Contains(msg, "wsaecconnreset") ||
		strings.Contains(msg, "10054") ||
		strings.Contains(msg, "强迫关闭") ||
		strings.Contains(msg, "远程主机")
}
