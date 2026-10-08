// Package memory is an in-memory implementation of the example ports for
// unit tests. It provides no row locking or transactional isolation.
package memory

import (
	"context"
	"sync"

	"github.com/svaan1/go-api-boilerplate/internal/features/example/application"
	"github.com/svaan1/go-api-boilerplate/internal/features/example/domain"
)

// Repository stores examples in a map. It is safe for concurrent use.
type Repository struct {
	mu       sync.Mutex
	examples map[domain.ID]domain.Example
}

var _ application.Repository = (*Repository)(nil)

// NewRepository returns an empty repository.
func NewRepository() *Repository {
	return &Repository{examples: make(map[domain.ID]domain.Example)}
}

// Create implements [application.Repository].
func (r *Repository) Create(_ context.Context, example domain.Example) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.nameTaken(example) {
		return domain.ErrDuplicateName
	}
	r.examples[example.ID()] = example
	return nil
}

// GetForUpdate implements [application.Repository].
func (r *Repository) GetForUpdate(_ context.Context, id domain.ID) (domain.Example, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	example, ok := r.examples[id]
	if !ok {
		return domain.Example{}, domain.ErrNotFound
	}
	return example, nil
}

// Update implements [application.Repository].
func (r *Repository) Update(_ context.Context, example domain.Example) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.examples[example.ID()]; !ok {
		return domain.ErrNotFound
	}
	if r.nameTaken(example) {
		return domain.ErrDuplicateName
	}
	r.examples[example.ID()] = example
	return nil
}

func (r *Repository) nameTaken(example domain.Example) bool {
	for id, existing := range r.examples {
		if id != example.ID() && existing.Name() == example.Name() {
			return true
		}
	}
	return false
}
