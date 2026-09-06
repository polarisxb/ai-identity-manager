package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"legacy-identity-keeper/internal/keeper"
)

func main() {
	configPath := flag.String("config", "proxy-secret.local", "path to local proxy credential file")
	listenAddr := flag.String("listen", "127.0.0.1:18080", "local HTTP CONNECT listen address")
	connectTimeout := flag.Duration("connect-timeout", 10*time.Second, "upstream dial and CONNECT timeout")
	breakerThreshold := flag.Int("breaker-threshold", 3, "consecutive setup failures before opening circuit")
	breakerOpen := flag.Duration("breaker-open", 5*time.Second, "circuit open duration")
	setupRetries := flag.Int("setup-retries", 1, "retry count for upstream setup failures")
	flag.Parse()

	cfg, err := keeper.LoadConfigFile(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	cfg.ListenAddr = *listenAddr
	cfg.ConnectTimeout = *connectTimeout
	cfg.Retry.SetupRetries = *setupRetries
	cfg.Breaker.FailureThreshold = *breakerThreshold
	cfg.Breaker.OpenDuration = *breakerOpen

	server, err := keeper.NewServer(cfg)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		log.Fatalf("listen %s: %v", cfg.ListenAddr, err)
	}

	done := make(chan error, 1)
	go func() {
		done <- server.Serve(listener)
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	fmt.Printf("identity-keeper listening on %s\n", listener.Addr())
	fmt.Println("upstream identity is fixed; no rotation or health-check traffic is enabled")

	select {
	case sig := <-signals:
		fmt.Printf("received %s, shutting down\n", sig)
		_ = server.Close()
		if err := <-done; err != nil {
			log.Fatal(err)
		}
	case err := <-done:
		if err != nil {
			log.Fatal(err)
		}
	}
}
