GO ?= go
SQLC_VERSION := v1.29.0
MIGRATE_VERSION := v4.18.3
GOLANGCI_LINT_VERSION := v2.3.0
GO_ARCH_LINT_VERSION := v1.14.0
MIGRATE_RUN = $(GO) run -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)
SQLC_RUN = $(GO) run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
# Each feature owns its migrations and tracks them in <feature>_schema_migrations.
MIGRATION_DIRS := $(wildcard internal/features/*/adapters/postgres/migrations)

.PHONY: check fmt lint arch vet test test-race test-integration build generate generate-check migrate-create migrate-up migrate-down compose-up compose-down run

# Single gate: lint (incl. depguard/forbidigo/gochecknoglobals/gochecknoinits), architecture rules, tests.
check: lint arch test

fmt:
	$(GO) fmt ./...

lint:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

arch:
	$(GO) run github.com/fe3dback/go-arch-lint@$(GO_ARCH_LINT_VERSION) check --project-path .

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

test-integration:
	@if [ -z "$$APP_TEST_DATABASE_URL" ]; then echo 'APP_TEST_DATABASE_URL is required for integration tests' >&2; exit 1; fi
	$(GO) test -tags=integration ./...
# Build binary into ignored local output directory.
build:
	mkdir -p bin
	$(GO) build -o bin/api ./cmd/api


# sqlc is invoked at a pinned version without adding tool dependencies to go.mod.
generate:
	$(SQLC_RUN) generate -f sqlc.yaml

generate-check:
	$(SQLC_RUN) diff -f sqlc.yaml

migrate-create:
	@test -n "$(feature)" && test -n "$(name)" || { echo 'usage: make migrate-create feature=example name=description' >&2; exit 2; }
	@test -d "internal/features/$(feature)" || { echo 'unknown feature: $(feature)' >&2; exit 2; }
	$(MIGRATE_RUN) create -ext sql -dir internal/features/$(feature)/adapters/postgres/migrations -format 20060102150405 -tz UTC $(name)

migrate-up:
	@if [ -z "$$APP_DATABASE_URL" ]; then echo 'APP_DATABASE_URL is required before applying migrations' >&2; exit 1; fi
	@set -e; for dir in $(MIGRATION_DIRS); do \
		feature=$$(echo "$$dir" | cut -d/ -f3); \
		case "$$APP_DATABASE_URL" in *\?*) sep='&';; *) sep='?';; esac; \
		echo "migrating $$feature"; \
		$(MIGRATE_RUN) -path "$$dir" -database "$$APP_DATABASE_URL$${sep}x-migrations-table=$${feature}_schema_migrations" up; \
	done

# Rolls back one version of one feature.
migrate-down:
	@test -n "$(feature)" || { echo 'usage: make migrate-down feature=example' >&2; exit 2; }
	@test -d "internal/features/$(feature)/adapters/postgres/migrations" || { echo 'feature has no migrations: $(feature)' >&2; exit 2; }
	@if [ -z "$$APP_DATABASE_URL" ]; then echo 'APP_DATABASE_URL is required before rolling back migrations' >&2; exit 1; fi
	@case "$$APP_DATABASE_URL" in *\?*) sep='&';; *) sep='?';; esac; \
	$(MIGRATE_RUN) -path "internal/features/$(feature)/adapters/postgres/migrations" -database "$$APP_DATABASE_URL$${sep}x-migrations-table=$(feature)_schema_migrations" down 1

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down

run:
	$(GO) run ./cmd/api
