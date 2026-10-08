package application

import (
	"context"
	"fmt"

	"github.com/svaan1/go-api-boilerplate/internal/features/example/domain"
)

// CreateExampleInput is the create request.
type CreateExampleInput struct {
	Name string
}

// CreateExample creates a new example.
type CreateExample struct {
	repository Repository
	clock      Clock
	ids        IDGenerator
}

// NewCreateExample wires the use case.
func NewCreateExample(repository Repository, clock Clock, ids IDGenerator) *CreateExample {
	return &CreateExample{repository: repository, clock: clock, ids: ids}
}

// Execute validates the name and stores a new example. It returns
// domain.ErrInvalidName or domain.ErrDuplicateName for rejected input.
func (uc *CreateExample) Execute(ctx context.Context, input CreateExampleInput) (domain.Example, error) {
	name, err := domain.NewName(input.Name)
	if err != nil {
		return domain.Example{}, err
	}
	example := domain.New(domain.ID(uc.ids.NewID()), name, uc.clock.Now())
	if err := uc.repository.Create(ctx, example); err != nil {
		return domain.Example{}, fmt.Errorf("creating example: %w", err)
	}
	return example, nil
}
