// Package databasetest provisions isolated PostgreSQL schemas for
// integration tests.
package databasetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"os"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// URL returns APP_TEST_DATABASE_URL or skips the test when it is unset.
func URL(t testing.TB) string {
	t.Helper()
	databaseURL := os.Getenv("APP_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set APP_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	return databaseURL
}

// Open creates a uniquely named schema, applies every *.up.sql file in
// migrations in lexical (timestamp) order, and returns a pool whose
// search_path is that schema. The schema is dropped when the test ends.
func Open(t testing.TB, migrations fs.FS) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	databaseURL := URL(t)

	suffix := make([]byte, 8)
	_, _ = rand.Read(suffix)
	schema := "test_" + hex.EncodeToString(suffix)

	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connecting to test database: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatalf("creating test schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("dropping test schema: %v", err)
		}
	})

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parsing test database URL: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("opening test pool: %v", err)
	}
	t.Cleanup(pool.Close)

	files, err := fs.Glob(migrations, "*.up.sql")
	if err != nil {
		t.Fatalf("listing migrations: %v", err)
	}
	slices.Sort(files)
	for _, name := range files {
		statements, err := fs.ReadFile(migrations, name)
		if err != nil {
			t.Fatalf("reading migration %s: %v", name, err)
		}
		// Exec without arguments uses the simple protocol, which accepts
		// multi-statement migration files.
		if _, err := pool.Exec(ctx, string(statements)); err != nil {
			t.Fatalf("applying migration %s: %v", name, err)
		}
	}
	return pool
}
