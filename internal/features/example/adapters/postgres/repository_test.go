//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/svaan1/go-api-boilerplate/internal/features/example/adapters/postgres"
	"github.com/svaan1/go-api-boilerplate/internal/features/example/application"
	"github.com/svaan1/go-api-boilerplate/internal/features/example/application/repositorytest"
	"github.com/svaan1/go-api-boilerplate/internal/features/example/domain"
	"github.com/svaan1/go-api-boilerplate/internal/platform/database"
	"github.com/svaan1/go-api-boilerplate/internal/platform/database/databasetest"
)

func TestRepositoryContract(t *testing.T) {
	repositorytest.Run(t, func(t *testing.T) application.Repository {
		return postgres.NewRepository(databasetest.Open(t, os.DirFS("migrations")))
	})
}

func TestRenameRollsBackWithTransaction(t *testing.T) {
	pool := databasetest.Open(t, os.DirFS("migrations"))
	repository := postgres.NewRepository(pool)
	transactor := database.NewTxManager(pool)
	ctx := context.Background()

	name, _ := domain.NewName("original")
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	const id = "01890a5d-ac96-774b-bcce-b302099a8057"
	if err := repository.Create(ctx, domain.New(id, name, created)); err != nil {
		t.Fatal(err)
	}

	abort := errors.New("abort")
	err := transactor.WithinTx(ctx, func(ctx context.Context) error {
		example, err := repository.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		renamed, _ := domain.NewName("renamed")
		example.Rename(renamed, created.Add(time.Hour))
		if err := repository.Update(ctx, example); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatalf("WithinTx error = %v, want abort", err)
	}

	stored, err := repository.GetForUpdate(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name().String() != "original" || !stored.UpdatedAt().Equal(created) {
		t.Fatalf("stored = {%q %s}; rollback did not discard update", stored.Name(), stored.UpdatedAt())
	}
}
