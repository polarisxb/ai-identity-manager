//go:build !windows

package killswitch

import "context"

type unsupportedRunner struct{}

func NewSystemRunner() CommandRunner {
	return unsupportedRunner{}
}

func (unsupportedRunner) Run(ctx context.Context, script string) (string, error) {
	return "", ErrUnsupported
}
