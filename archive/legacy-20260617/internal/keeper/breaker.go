package keeper

import (
	"sync"
	"time"
)

type circuitBreaker struct {
	mu           sync.Mutex
	threshold    int
	openDuration time.Duration
	failures     int
	openUntil    time.Time
}

func newCircuitBreaker(cfg CircuitBreakerConfig) *circuitBreaker {
	return &circuitBreaker{
		threshold:    cfg.FailureThreshold,
		openDuration: cfg.OpenDuration,
	}
}

func (b *circuitBreaker) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.openUntil.IsZero() || time.Now().After(b.openUntil)
}

func (b *circuitBreaker) success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.openUntil = time.Time{}
}

func (b *circuitBreaker) failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	if b.failures >= b.threshold {
		b.openUntil = time.Now().Add(b.openDuration)
	}
}
