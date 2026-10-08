// Package ratelimit bounds client request rates and per-client state.
package ratelimit

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/svaan1/go-api-boilerplate/internal/platform/config"
	"golang.org/x/time/rate"
)

// Decision describes one request's token bucket outcome.
type Decision struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
}

// Limiter is the transport's replaceable admission contract.
type Limiter interface {
	Allow(context.Context, string) (Decision, error)
}

// ErrCapacity refuses new client state instead of growing memory without bound.
var ErrCapacity = errors.New("rate limit client capacity reached")

type bucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// Memory stores bounded per-client buckets with synchronized operations.
type Memory struct {
	mu                       sync.Mutex
	entries                  map[string]*bucket
	requestsPerSecond        rate.Limit
	burst, maxClients        int
	idleTTL, cleanupInterval time.Duration
	now                      func() time.Time
}

// New constructs a bounded in-memory limiter; nil clock uses real time.
func New(cfg config.RateLimit, now func() time.Time) *Memory {
	if now == nil {
		now = time.Now
	}
	return &Memory{entries: make(map[string]*bucket), requestsPerSecond: rate.Limit(cfg.RequestsPerSecond), burst: cfg.Burst, maxClients: cfg.MaxClients, idleTTL: cfg.IdleTTL, cleanupInterval: cfg.CleanupInterval, now: now}
}

// Allow charges one token or returns bounded RetryAfter on denial.
func (m *Memory) Allow(ctx context.Context, client string) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	current := m.entries[client]
	if current == nil {
		if len(m.entries) >= m.maxClients {
			m.expire(now)
		}
		if len(m.entries) >= m.maxClients {
			return Decision{}, ErrCapacity
		}
		current = &bucket{limiter: rate.NewLimiter(m.requestsPerSecond, m.burst)}
		m.entries[client] = current
	}
	current.lastSeen = now
	decision := Decision{Limit: m.burst}
	if current.limiter.AllowN(now, 1) {
		decision.Allowed = true
		decision.Remaining = max(0, int(math.Floor(current.limiter.TokensAt(now))))
		if decision.Remaining > 0 {
			return decision, nil
		}
	}
	reservation := current.limiter.ReserveN(now, 1)
	if !reservation.OK() {
		return Decision{}, errors.New("rate limit burst cannot reserve token")
	}
	decision.RetryAfter = reservation.DelayFrom(now)
	reservation.CancelAt(now)
	return decision, nil
}

func (m *Memory) expire(now time.Time) {
	for key, item := range m.entries {
		if now.Sub(item.lastSeen) >= m.idleTTL {
			delete(m.entries, key)
		}
	}
}

// Run removes expired buckets until cancellation.
func (m *Memory) Run(ctx context.Context) error {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.mu.Lock()
			m.expire(m.now())
			m.mu.Unlock()
		}
	}
}
