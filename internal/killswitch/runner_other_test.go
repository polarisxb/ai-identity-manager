//go:build !windows

package killswitch

import (
	"context"
	"errors"
	"testing"
)

func TestSystemRunnerUnsupportedOffWindows(t *testing.T) {
	runner := NewSystemRunner()

	_, err := runner.Run(context.Background(), "Get-NetFirewallRule")

	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err=%v", err)
	}
}
