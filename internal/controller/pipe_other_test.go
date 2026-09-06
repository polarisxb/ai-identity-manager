//go:build !windows

package controller

import (
	"errors"
	"testing"
)

func TestNewPipeControllerUnsupportedOffWindows(t *testing.T) {
	if _, err := newPipeController(Endpoint{PipePath: `\\.\pipe\verge-mihomo`}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}
