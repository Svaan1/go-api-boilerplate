// Package postgres is the outbound PostgreSQL adapter for examples: sqlc
// queries, row mapping, driver-error translation, and migrations.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/svaan1/go-api-boilerplate/internal/features/example/adapters/postgres/sqlc"
	"github.com/svaan1/go-api-boilerplate/internal/features/example/application"
	"github.com/svaan1/go-api-boilerplate/internal/features/example/domain"
	"github.com/svaan1/go-api-boilerplate/internal/platform/database"
)

const (
	uniqueViolation  = "23505"
	nameUniqueConstr = "examples_name_key"
)

// Repository stores examples in PostgreSQL. Calls made inside
// database.TxManager.WithinTx use that transaction.
type Repository struct {
	pool *pgxpool.Pool
}

var (
	_ application.Repository = (*Repository)(nil)
	_ application.Transactor = (*database.TxManager)(nil)
)

// NewRepository uses pool outside transactions.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) queries(ctx context.Context) *sqlc.Queries {
	return sqlc.New(database.Conn(ctx, r.pool))
}

// Create implements [application.Repository].
func (r *Repository) Create(ctx context.Context, example domain.Example) error {
	id, ok := parseID(example.ID())
	if !ok {
		return fmt.Errorf("creating example: invalid id %q", example.ID())
	}
	err := r.queries(ctx).CreateExample(ctx, sqlc.CreateExampleParams{
		ID:        id,
		Name:      example.Name().String(),
		CreatedAt: example.CreatedAt(),
		UpdatedAt: example.UpdatedAt(),
	})
	if err != nil {
		return translate("creating example", err)
	}
	return nil
}

// GetForUpdate implements [application.Repository].
func (r *Repository) GetForUpdate(ctx context.Context, id domain.ID) (domain.Example, error) {
	uuid, ok := parseID(id)
	if !ok {
		return domain.Example{}, domain.ErrNotFound
	}
	row, err := r.queries(ctx).GetExampleForUpdate(ctx, uuid)
	if err != nil {
		return domain.Example{}, translate("loading example", err)
	}
	return toDomain(row)
}

// Update implements [application.Repository].
func (r *Repository) Update(ctx context.Context, example domain.Example) error {
	id, ok := parseID(example.ID())
	if !ok {
		return domain.ErrNotFound
	}
	rows, err := r.queries(ctx).UpdateExample(ctx, sqlc.UpdateExampleParams{
		ID:        id,
		Name:      example.Name().String(),
		UpdatedAt: example.UpdatedAt(),
	})
	if err != nil {
		return translate("updating example", err)
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// parseID reports false for IDs that cannot be a PostgreSQL uuid; such rows
// cannot exist.
func parseID(id domain.ID) (pgtype.UUID, bool) {
	var uuid pgtype.UUID
	if err := uuid.Scan(string(id)); err != nil {
		return pgtype.UUID{}, false
	}
	return uuid, true
}

func toDomain(row sqlc.Example) (domain.Example, error) {
	name, err := domain.NewName(row.Name)
	if err != nil {
		return domain.Example{}, fmt.Errorf("mapping example row: %w", err)
	}
	return domain.Restore(domain.ID(row.ID.String()), name, row.CreatedAt.UTC(), row.UpdatedAt.UTC()), nil
}

// translate converts driver errors into domain errors where the domain has a
// name for them and wraps everything else with operation context.
func translate(operation string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == nameUniqueConstr {
		return domain.ErrDuplicateName
	}
	return fmt.Errorf("%s: %w", operation, err)
}
