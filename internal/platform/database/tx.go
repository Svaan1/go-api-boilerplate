package database

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type txKey struct{}

// TxManager runs units of work atomically. The transaction travels in the
// context passed to the callback; repositories reach it through [Conn].
type TxManager struct {
	pool *pgxpool.Pool
}

// NewTxManager runs transactions on pool.
func NewTxManager(pool *pgxpool.Pool) *TxManager {
	return &TxManager{pool: pool}
}

// WithinTx runs fn inside a transaction, committing when fn returns nil and
// rolling back otherwise. A call made with a context that already carries a
// transaction joins it instead of nesting, so the outermost caller decides
// the commit.
func (m *TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
	return WithTx(ctx, m.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
}

// Conn returns the transaction carried by ctx, or fallback outside one.
func Conn(ctx context.Context, fallback DBTX) DBTX {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return fallback
}
