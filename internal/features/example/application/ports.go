// Package application holds example use cases and the outbound ports they
// depend on. It imports its own domain and the standard library only.
package application

import (
	"context"
	"time"

	"github.com/svaan1/go-api-boilerplate/internal/features/example/domain"
)

// Repository persists examples. Implementations return domain errors:
// ErrNotFound for unknown (including malformed) IDs and ErrDuplicateName for
// name conflicts. Every implementation must pass repositorytest.Run.
type Repository interface {
	Create(ctx context.Context, example domain.Example) error
	// GetForUpdate loads an example and, inside Transactor.WithinTx, locks it
	// until the transaction ends.
	GetForUpdate(ctx context.Context, id domain.ID) (domain.Example, error)
	Update(ctx context.Context, example domain.Example) error
}

// Transactor runs fn atomically. Repository calls made with the ctx passed to
// fn take part in the transaction.
type Transactor interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Clock supplies the current time.
type Clock interface {
	Now() time.Time
}

// IDGenerator supplies new unique identifiers.
type IDGenerator interface {
	NewID() string
}
