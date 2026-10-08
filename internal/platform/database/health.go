package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthCheck reports PostgreSQL connectivity for readiness probes.
type HealthCheck struct {
	pool *pgxpool.Pool
}

// NewHealthCheck checks pool connectivity.
func NewHealthCheck(pool *pgxpool.Pool) HealthCheck {
	return HealthCheck{pool: pool}
}

// Name is the readiness log label.
func (HealthCheck) Name() string { return "postgres" }

// Check pings the database within ctx's deadline.
func (c HealthCheck) Check(ctx context.Context) error {
	return c.pool.Ping(ctx)
}
