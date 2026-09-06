package controller

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type Endpoint struct {
	TCPAddr  string
	PipePath string
	Secret   string
}

type Version struct {
	Version string
	Meta    bool
}

type RuntimeConfig struct {
	Mode string
}

type Connection struct {
	ID       string
	Chains   []string
	Rule     string
	Upload   int64
	Download int64
	Start    time.Time
	Metadata Metadata
}

type Metadata struct {
	Network          string
	Type             string
	Host             string
	Process          string
	ProcessPath      string
	DestinationIP    string
	DestinationIPASN string
}

type Controller interface {
	Ping(ctx context.Context) (Version, error)
	Connections(ctx context.Context) ([]Connection, error)
	RuntimeConfig(ctx context.Context) (RuntimeConfig, error)
	ReloadRuntime(ctx context.Context, configPath string) error
	Transport() string
	Close() error
}

var (
	ErrNoController = errors.New("no reachable mihomo controller (tcp and pipe both down)")
	ErrUnsupported  = errors.New("named pipe controller is only supported on windows")
	ErrUnauthorized = errors.New("controller rejected credentials (401)")
)

var errNotAttempted = errors.New("not attempted")

var _ Controller = (*httpController)(nil)

func Open(ctx context.Context, ep Endpoint) (Controller, error) {
	tcpErr, pipeErr := errNotAttempted, errNotAttempted
	if ep.TCPAddr != "" {
		c := newTCPController(ep)
		_, tcpErr = c.Ping(ctx)
		if tcpErr == nil {
			return c, nil
		}
	}
	if ep.PipePath != "" {
		c, err := newPipeController(ep)
		if err != nil {
			pipeErr = err
		} else {
			_, pipeErr = c.Ping(ctx)
			if pipeErr == nil {
				return c, nil
			}
		}
	}
	return nil, fmt.Errorf("%w (tcp: %v; pipe: %v)", ErrNoController, tcpErr, pipeErr)
}
