package application

import (
	"context"
	"fmt"

	"github.com/svaan1/go-api-boilerplate/internal/features/example/domain"
)

// RenameExampleInput is the rename request.
type RenameExampleInput struct {
	ID   string
	Name string
}

// RenameExample changes an existing example's name.
type RenameExample struct {
	repository Repository
	transactor Transactor
	clock      Clock
}

// NewRenameExample wires the use case.
func NewRenameExample(repository Repository, transactor Transactor, clock Clock) *RenameExample {
	return &RenameExample{repository: repository, transactor: transactor, clock: clock}
}

// Execute loads, renames, and saves the example in one transaction so a
// concurrent rename cannot interleave between read and write. It returns
// domain.ErrInvalidName, domain.ErrNotFound, or domain.ErrDuplicateName for
// rejected input.
func (uc *RenameExample) Execute(ctx context.Context, input RenameExampleInput) (domain.Example, error) {
	name, err := domain.NewName(input.Name)
	if err != nil {
		return domain.Example{}, err
	}
	var renamed domain.Example
	err = uc.transactor.WithinTx(ctx, func(ctx context.Context) error {
		example, err := uc.repository.GetForUpdate(ctx, domain.ID(input.ID))
		if err != nil {
			return err
		}
		example.Rename(name, uc.clock.Now())
		if err := uc.repository.Update(ctx, example); err != nil {
			return err
		}
		renamed = example
		return nil
	})
	if err != nil {
		return domain.Example{}, fmt.Errorf("renaming example: %w", err)
	}
	return renamed, nil
}
