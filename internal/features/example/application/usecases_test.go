package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/svaan1/go-api-boilerplate/internal/features/example/adapters/memory"
	"github.com/svaan1/go-api-boilerplate/internal/features/example/application"
	"github.com/svaan1/go-api-boilerplate/internal/features/example/domain"
)

type fixedClock struct{ now time.Time }

func (c *fixedClock) Now() time.Time { return c.now }

type fixedID string

func (id fixedID) NewID() string { return string(id) }

type txKey struct{}

// spyTransactor marks the callback context so tests can assert repository
// calls ran inside the unit of work, and can simulate a failing commit.
type spyTransactor struct {
	calls     int
	commitErr error
}

func (s *spyTransactor) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	s.calls++
	if err := fn(context.WithValue(ctx, txKey{}, true)); err != nil {
		return err
	}
	return s.commitErr
}

// txCheckingRepository fails any call made outside spyTransactor.WithinTx.
type txCheckingRepository struct{ application.Repository }

func requireTx(ctx context.Context) error {
	if inTx, _ := ctx.Value(txKey{}).(bool); !inTx {
		return errors.New("repository called outside transaction")
	}
	return nil
}

func (r txCheckingRepository) GetForUpdate(ctx context.Context, id domain.ID) (domain.Example, error) {
	if err := requireTx(ctx); err != nil {
		return domain.Example{}, err
	}
	return r.Repository.GetForUpdate(ctx, id)
}

func (r txCheckingRepository) Update(ctx context.Context, example domain.Example) error {
	if err := requireTx(ctx); err != nil {
		return err
	}
	return r.Repository.Update(ctx, example)
}

func createdAt() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }

func TestCreateExample(t *testing.T) {
	repository := memory.NewRepository()
	create := application.NewCreateExample(repository, &fixedClock{now: createdAt()}, fixedID("id-1"))

	got, err := create.Execute(context.Background(), application.CreateExampleInput{Name: "  first  "})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.ID() != "id-1" || got.Name().String() != "first" || !got.CreatedAt().Equal(createdAt()) || !got.UpdatedAt().Equal(createdAt()) {
		t.Fatalf("created = {%s %q %s %s}", got.ID(), got.Name(), got.CreatedAt(), got.UpdatedAt())
	}
	stored, err := repository.GetForUpdate(context.Background(), "id-1")
	if err != nil || stored.Name() != got.Name() {
		t.Fatalf("stored = %v, %v; want persisted example", stored.Name(), err)
	}
}

func TestCreateExampleRejectsInput(t *testing.T) {
	repository := memory.NewRepository()
	create := application.NewCreateExample(repository, &fixedClock{now: createdAt()}, fixedID("id-1"))
	if _, err := create.Execute(context.Background(), application.CreateExampleInput{Name: "taken"}); err != nil {
		t.Fatal(err)
	}
	create = application.NewCreateExample(repository, &fixedClock{now: createdAt()}, fixedID("id-2"))
	for _, tc := range []struct {
		name string
		want error
	}{
		{name: "   ", want: domain.ErrInvalidName},
		{name: strings.Repeat("a", domain.MaxNameLength+1), want: domain.ErrInvalidName},
		{name: "taken", want: domain.ErrDuplicateName},
	} {
		if _, err := create.Execute(context.Background(), application.CreateExampleInput{Name: tc.name}); !errors.Is(err, tc.want) {
			t.Fatalf("Execute(%q) error = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func seeded(t *testing.T) *memory.Repository {
	t.Helper()
	repository := memory.NewRepository()
	for id, name := range map[string]string{"id-1": "first", "id-2": "second"} {
		if _, err := application.NewCreateExample(repository, &fixedClock{now: createdAt()}, fixedID(id)).
			Execute(context.Background(), application.CreateExampleInput{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	return repository
}

func TestRenameExampleRunsInTransaction(t *testing.T) {
	repository := seeded(t)
	transactor := &spyTransactor{}
	renamedAt := createdAt().Add(time.Hour)
	rename := application.NewRenameExample(txCheckingRepository{repository}, transactor, &fixedClock{now: renamedAt})

	got, err := rename.Execute(context.Background(), application.RenameExampleInput{ID: "id-1", Name: "renamed"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if transactor.calls != 1 {
		t.Fatalf("WithinTx calls = %d, want 1", transactor.calls)
	}
	if got.Name().String() != "renamed" || !got.CreatedAt().Equal(createdAt()) || !got.UpdatedAt().Equal(renamedAt) {
		t.Fatalf("renamed = {%q %s %s}", got.Name(), got.CreatedAt(), got.UpdatedAt())
	}
	stored, _ := repository.GetForUpdate(context.Background(), "id-1")
	if stored.Name().String() != "renamed" {
		t.Fatalf("stored name = %q, want renamed", stored.Name())
	}
}

func TestRenameExampleErrors(t *testing.T) {
	commitErr := errors.New("commit failed")
	for _, tc := range []struct {
		name       string
		input      application.RenameExampleInput
		commitErr  error
		want       error
		wantTxCall int
	}{
		{name: "invalid name skips transaction", input: application.RenameExampleInput{ID: "id-1", Name: ""}, want: domain.ErrInvalidName},
		{name: "unknown id", input: application.RenameExampleInput{ID: "missing", Name: "x"}, want: domain.ErrNotFound, wantTxCall: 1},
		{name: "name taken", input: application.RenameExampleInput{ID: "id-1", Name: "second"}, want: domain.ErrDuplicateName, wantTxCall: 1},
		{name: "commit failure", input: application.RenameExampleInput{ID: "id-1", Name: "x"}, commitErr: commitErr, want: commitErr, wantTxCall: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transactor := &spyTransactor{commitErr: tc.commitErr}
			rename := application.NewRenameExample(txCheckingRepository{seeded(t)}, transactor, &fixedClock{now: createdAt()})
			got, err := rename.Execute(context.Background(), tc.input)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if got.ID() != "" {
				t.Fatalf("result = %q, want zero example on error", got.ID())
			}
			if transactor.calls != tc.wantTxCall {
				t.Fatalf("WithinTx calls = %d, want %d", transactor.calls, tc.wantTxCall)
			}
		})
	}
}
