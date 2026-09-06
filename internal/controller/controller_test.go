package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenSelectsTcpWhenReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"meta":true,"version":"v1.19.20"}`))
	}))
	defer srv.Close()
	ep := Endpoint{TCPAddr: strings.TrimPrefix(srv.URL, "http://")}
	c, err := Open(context.Background(), ep)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer c.Close()
	if c.Transport() != "tcp" {
		t.Fatalf("transport = %q", c.Transport())
	}
}

func TestOpenReturnsErrNoControllerWhenBothDown(t *testing.T) {
	ep := Endpoint{TCPAddr: "127.0.0.1:1", PipePath: ""}
	if _, err := Open(context.Background(), ep); !errors.Is(err, ErrNoController) {
		t.Fatalf("err = %v, want ErrNoController", err)
	}
}

func TestOpenSurfacesPipeErrorDetail(t *testing.T) {
	ep := Endpoint{TCPAddr: "", PipePath: `\\.\pipe\definitely-not-here-ai-identity`}
	_, err := Open(context.Background(), ep)
	if !errors.Is(err, ErrNoController) {
		t.Fatalf("err = %v, want errors.Is ErrNoController", err)
	}
	if !strings.Contains(err.Error(), "pipe:") {
		t.Fatalf("err should surface pipe detail, got: %v", err)
	}
}
