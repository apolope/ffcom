package httpapi

import (
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToBurstThenBlocks(t *testing.T) {
	l := &rateLimiter{
		buckets:        make(map[string]*bucket),
		ratePerSecond:  1,
		burst:          3,
		idleExpiration: time.Minute,
	}

	for i := 0; i < 3; i++ {
		if !l.allow("1.2.3.4") {
			t.Fatalf("requisição %d deveria ser permitida (dentro do burst)", i+1)
		}
	}
	if l.allow("1.2.3.4") {
		t.Fatal("4ª requisição deveria ser bloqueada (burst esgotado)")
	}
}

func TestRateLimiterRefillsOverTime(t *testing.T) {
	l := &rateLimiter{
		buckets:        make(map[string]*bucket),
		ratePerSecond:  1,
		burst:          1,
		idleExpiration: time.Minute,
	}

	if !l.allow("1.2.3.4") {
		t.Fatal("primeira requisição deveria ser permitida")
	}
	if l.allow("1.2.3.4") {
		t.Fatal("segunda requisição imediata deveria ser bloqueada")
	}

	// Simula 1s de espera recuando lastSeen, em vez de dormir de verdade.
	l.buckets["1.2.3.4"].lastSeen = time.Now().Add(-1 * time.Second)

	if !l.allow("1.2.3.4") {
		t.Fatal("requisição após 1s deveria ser permitida (bucket recarregado)")
	}
}

func TestRateLimiterTracksKeysIndependently(t *testing.T) {
	l := &rateLimiter{
		buckets:        make(map[string]*bucket),
		ratePerSecond:  1,
		burst:          1,
		idleExpiration: time.Minute,
	}

	if !l.allow("1.2.3.4") {
		t.Fatal("primeiro IP deveria ser permitido")
	}
	if !l.allow("5.6.7.8") {
		t.Fatal("segundo IP não deveria ser afetado pelo bucket do primeiro")
	}
}
