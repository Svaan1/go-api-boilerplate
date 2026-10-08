// Package repositorytest is the behavioral contract every
// application.Repository implementation must satisfy.
package repositorytest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/svaan1/go-api-boilerplate/internal/features/example/application"
	"github.com/svaan1/go-api-boilerplate/internal/features/example/domain"
)

// Fixed UUIDs keep the contract valid for UUID-typed storage.
const (
	firstID   domain.ID = "01890a5d-ac96-774b-bcce-b302099a8057"
	secondID  domain.ID = "01890a5d-ac96-774b-bcce-b302099a8058"
	missingID domain.ID = "01890a5d-ac96-774b-bcce-b302099a8059"
)

// Run executes the contract. newRepository must return an empty repository
// isolated from other calls.
func Run(t *testing.T, newRepository func(t *testing.T) application.Repository) {
	t.Helper()
	// Microsecond precision matches PostgreSQL timestamptz.
	created := time.Date(2026, 1, 2, 3, 4, 5, 6000, time.UTC)
	updated := created.Add(time.Hour)

	t.Run("create then get returns stored fields", func(t *testing.T) {
		repository := newRepository(t)
		want := domain.New(firstID, name(t, "first"), created)
		mustCreate(t, repository, want)
		got, err := repository.GetForUpdate(context.Background(), firstID)
		if err != nil {
			t.Fatalf("GetForUpdate: %v", err)
		}
		assertEqual(t, got, want)
	})

	t.Run("create rejects duplicate name", func(t *testing.T) {
		repository := newRepository(t)
		mustCreate(t, repository, domain.New(firstID, name(t, "taken"), created))
		err := repository.Create(context.Background(), domain.New(secondID, name(t, "taken"), created))
		if !errors.Is(err, domain.ErrDuplicateName) {
			t.Fatalf("Create duplicate error = %v, want ErrDuplicateName", err)
		}
	})

	t.Run("get unknown or malformed id is not found", func(t *testing.T) {
		repository := newRepository(t)
		for _, id := range []domain.ID{missingID, "not-a-uuid", ""} {
			if _, err := repository.GetForUpdate(context.Background(), id); !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("GetForUpdate(%q) error = %v, want ErrNotFound", id, err)
			}
		}
	})

	t.Run("update persists changes", func(t *testing.T) {
		repository := newRepository(t)
		example := domain.New(firstID, name(t, "before"), created)
		mustCreate(t, repository, example)
		example.Rename(name(t, "after"), updated)
		if err := repository.Update(context.Background(), example); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, err := repository.GetForUpdate(context.Background(), firstID)
		if err != nil {
			t.Fatalf("GetForUpdate: %v", err)
		}
		assertEqual(t, got, example)
	})

	t.Run("update unknown id is not found", func(t *testing.T) {
		repository := newRepository(t)
		err := repository.Update(context.Background(), domain.New(missingID, name(t, "ghost"), created))
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Update unknown error = %v, want ErrNotFound", err)
		}
	})

	t.Run("update rejects name used by another example", func(t *testing.T) {
		repository := newRepository(t)
		mustCreate(t, repository, domain.New(firstID, name(t, "one"), created))
		second := domain.New(secondID, name(t, "two"), created)
		mustCreate(t, repository, second)
		second.Rename(name(t, "one"), updated)
		if err := repository.Update(context.Background(), second); !errors.Is(err, domain.ErrDuplicateName) {
			t.Fatalf("Update duplicate error = %v, want ErrDuplicateName", err)
		}
	})
}

func name(t *testing.T, raw string) domain.Name {
	t.Helper()
	n, err := domain.NewName(raw)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func mustCreate(t *testing.T, repository application.Repository, example domain.Example) {
	t.Helper()
	if err := repository.Create(context.Background(), example); err != nil {
		t.Fatalf("Create: %v", err)
	}
}

func assertEqual(t *testing.T, got, want domain.Example) {
	t.Helper()
	if got.ID() != want.ID() || got.Name() != want.Name() ||
		!got.CreatedAt().Equal(want.CreatedAt()) || !got.UpdatedAt().Equal(want.UpdatedAt()) {
		t.Fatalf("example = {%s %s %s %s}, want {%s %s %s %s}",
			got.ID(), got.Name(), got.CreatedAt(), got.UpdatedAt(),
			want.ID(), want.Name(), want.CreatedAt(), want.UpdatedAt())
	}
}
