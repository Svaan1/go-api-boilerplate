// Package database owns PostgreSQL connection and transaction primitives.
package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/svaan1/go-api-boilerplate/internal/platform/config"
)

// DBTX is the database interface implemented by pgx transactions and pools,
// including the methods required by sqlc's pgx/v5 generated code.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Open parses database settings, applies pool limits, and verifies connectivity
// before returning. Errors intentionally omit driver details, which may contain
// the configured database URL or credentials.
func Open(ctx context.Context, settings config.Database, logger *slog.Logger) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(settings.URL)
	if err != nil {
		return nil, errors.New("parsing database configuration: invalid database URL")
	}

	poolConfig.MaxConns = settings.MaxConnections
	poolConfig.MinConns = settings.MinConnections
	poolConfig.MaxConnLifetime = settings.MaxLifetime
	poolConfig.MaxConnIdleTime = settings.MaxIdleTime
	poolConfig.HealthCheckPeriod = settings.HealthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("creating database pool: %w", ctxErr)
		}
		return nil, errors.New("creating database pool: invalid pool configuration")
	}

	pingCtx, cancel := context.WithTimeout(ctx, settings.PingTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		if ctxErr := pingCtx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("pinging database: %w", ctxErr)
		}
		return nil, errors.New("pinging database: database is unreachable")
	}

	if logger != nil {
		logger.DebugContext(ctx, "database pool opened")
	}
	return pool, nil
}

// WithTx runs fn inside a transaction, committing on success and rolling back
// on callback failure. It joins callback and rollback errors when both occur.
func WithTx(
	ctx context.Context,
	pool *pgxpool.Pool,
	options pgx.TxOptions,
	fn func(pgx.Tx) error,
) error {
	if pool == nil {
		return errors.New("beginning database transaction: database pool is nil")
	}
	if fn == nil {
		return errors.New("running database transaction: callback is nil")
	}

	tx, err := pool.BeginTx(ctx, options)
	if err != nil {
		return fmt.Errorf("beginning database transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		rollbackErr := tx.Rollback(ctx)
		if rollbackErr != nil {
			return errors.Join(
				err,
				fmt.Errorf("rolling back database transaction: %w", rollbackErr),
			)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return errors.Join(
				fmt.Errorf("committing database transaction: %w", err),
				fmt.Errorf("rolling back database transaction: %w", rollbackErr),
			)
		}
		return fmt.Errorf("committing database transaction: %w", err)
	}
	return nil
}
