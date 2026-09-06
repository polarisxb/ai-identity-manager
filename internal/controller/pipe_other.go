//go:build !windows

package controller

func newPipeController(ep Endpoint) (*httpController, error) {
	return nil, ErrUnsupported
}
