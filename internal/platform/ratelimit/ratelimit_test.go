package ratelimit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/svaan1/go-api-boilerplate/internal/platform/config"
)

func TestMemoryRefillAndCapacity(t *testing.T) {
	now := time.Unix(100, 0)
	limits := New(config.RateLimit{RequestsPerSecond: 1, Burst: 2, MaxClients: 1, IdleTTL: 10 * time.Second, CleanupInterval: time.Minute}, func() time.Time { return now })
	ctx := context.Background()
	for n := range 2 {
		decision, err := limits.Allow(ctx, "first")
		if err != nil || !decision.Allowed || decision.Remaining != 1-n {
			t.Fatalf("request %d: %+v %v", n, decision, err)
		}
	}
	denied, err := limits.Allow(ctx, "first")
	if err != nil || denied.Allowed || denied.RetryAfter != time.Second {
		t.Fatalf("missing retry: %+v %v", denied, err)
	}
	if _, err := limits.Allow(ctx, "second"); !errors.Is(err, ErrCapacity) {
		t.Fatalf("capacity error=%v", err)
	}
	now = now.Add(time.Second)
	refill, err := limits.Allow(ctx, "first")
	if err != nil || !refill.Allowed {
		t.Fatalf("refill %+v %v", refill, err)
	}
	now = now.Add(11 * time.Second)
	fresh, err := limits.Allow(ctx, "second")
	if err != nil || !fresh.Allowed {
		t.Fatalf("expired eviction %+v %v", fresh, err)
	}
}

func TestMemoryConcurrentClients(t *testing.T) {
	limiter := New(config.RateLimit{RequestsPerSecond: 1000, Burst: 10, MaxClients: 10, IdleTTL: time.Minute, CleanupInterval: time.Minute}, nil)
	var group sync.WaitGroup
	for range 20 {
		group.Go(func() { _, _ = limiter.Allow(context.Background(), "same-client") })
	}
	group.Wait()
}
