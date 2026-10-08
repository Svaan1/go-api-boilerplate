package memory_test

import (
	"testing"

	"github.com/svaan1/go-api-boilerplate/internal/features/example/adapters/memory"
	"github.com/svaan1/go-api-boilerplate/internal/features/example/application"
	"github.com/svaan1/go-api-boilerplate/internal/features/example/application/repositorytest"
)

func TestRepositoryContract(t *testing.T) {
	repositorytest.Run(t, func(*testing.T) application.Repository {
		return memory.NewRepository()
	})
}
