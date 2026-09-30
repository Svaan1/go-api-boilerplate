GO ?= go
SQLC_VERSION := v1.29.0
MIGRATE_VERSION := v4.18.3
MIGRATE_RUN = $(GO) run -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)

.PHONY: fmt lint vet test test-race test-integration build generate generate-check migrate-create migrate-up migrate-down compose-up compose-down run

fmt:
	$(GO) fmt ./...

lint:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.3.0 run ./...

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
	@if ! find migrations -maxdepth 1 -type f -name '*.up.sql' -print -quit | grep -q .; then echo 'sqlc generation skipped: no application schema exists'; \
	elif ! find internal/database/query -type f -name '*.sql' -print -quit | grep -q .; then echo 'sqlc generation skipped: no SQL queries exist'; \
	else $(GO) run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate -f sqlc.yaml; fi

generate-check:
	@set -eu; \
	if ! find migrations -maxdepth 1 -type f -name '*.up.sql' -print -quit | grep -q .; then echo 'sqlc drift check skipped: no application schema exists'; exit 0; fi; \
	if ! find internal/database/query -type f -name '*.sql' -print -quit | grep -q .; then echo 'sqlc drift check skipped: no SQL queries exist'; exit 0; fi; \
	check_config=.sqlc-check.$$$$.yaml; check_output=.sqlc-check-output.$$$$; \
	trap 'rm -f "$$check_config"; rm -rf "$$check_output"' EXIT HUP INT TERM; \
	sed "s|out: \"internal/database/sqlc\"|out: \"$$check_output\"|" sqlc.yaml > "$$check_config"; \
	$(GO) run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate -f "$$check_config"; \
	if [ -d internal/database/sqlc ]; then diff -ru --exclude=README.md internal/database/sqlc "$$check_output"; \
	else echo 'generated sqlc output missing; run make generate' >&2; exit 1; fi

migrate-create:
	@test -n "$(name)" || { echo 'usage: make migrate-create name=description' >&2; exit 2; }
	$(MIGRATE_RUN) create -ext sql -dir migrations -format 20060102150405 -tz UTC $(name)

migrate-up:
	@if ! find migrations -maxdepth 1 -type f -name '*.up.sql' -print -quit | grep -q .; then echo 'migration skipped: no application schema exists; database unchanged'; \
	elif [ -z "$$APP_DATABASE_URL" ]; then echo 'APP_DATABASE_URL is required before applying migrations' >&2; exit 1; \
	else $(MIGRATE_RUN) -path migrations -database "$$APP_DATABASE_URL" up; fi

migrate-down:
	@if ! find migrations -maxdepth 1 -type f -name '*.up.sql' -print -quit | grep -q .; then echo 'migration rollback skipped: no application schema exists; database unchanged'; \
	elif [ -z "$$APP_DATABASE_URL" ]; then echo 'APP_DATABASE_URL is required before rolling back migrations' >&2; exit 1; \
	else $(MIGRATE_RUN) -path migrations -database "$$APP_DATABASE_URL" down 1; fi

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down

run:
	$(GO) run ./cmd/api
