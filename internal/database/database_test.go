//go:build integration

package database

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/svaan1/go-api-boilerplate/internal/config"
)

func TestOpenRejectsInvalidURLWithoutExposingCredentials(t *testing.T) {
	databaseURL := "postgres://parse-test-user:parse-test-password@localhost/db%zz"
	_, err := Open(context.Background(), config.Database{URL: databaseURL}, nil)
	if err == nil {
		t.Fatal("Open succeeded for invalid database URL")
	}
	if strings.Contains(err.Error(), databaseURL) ||
		strings.Contains(err.Error(), "parse-test-user") ||
		strings.Contains(err.Error(), "parse-test-password") {
		t.Fatal("Open error exposed database URL or credentials")
	}
}

func TestOpenRejectsUnreachableDatabase(t *testing.T) {
	databaseURL := testDatabaseURL(t)
	parsedURL, err := url.Parse(databaseURL)
	if err != nil || parsedURL.Scheme == "" {
		t.Fatal("APP_TEST_DATABASE_URL must be a PostgreSQL URL")
	}
	parsedURL.Host = net.JoinHostPort("127.0.0.1", "1")
	parsedURL.User = url.UserPassword("database-test-user", "database-test-password")

	settings := testDatabaseSettings(parsedURL.String())
	settings.PingTimeout = 500 * time.Millisecond
	pool, err := Open(context.Background(), settings, nil)
	if err == nil {
		pool.Close()
		t.Fatal("Open succeeded for unreachable PostgreSQL")
	}
	if strings.Contains(err.Error(), parsedURL.String()) ||
		strings.Contains(err.Error(), "database-test-user") ||
		strings.Contains(err.Error(), "database-test-password") {
		t.Fatal("Open error exposed database URL or credentials")
	}
}

func TestWithTxCommitAndRollback(t *testing.T) {
	pool, err := Open(context.Background(), testDatabaseSettings(testDatabaseURL(t)), nil)
	if err != nil {
		t.Fatalf("Open test database: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	err = WithTx(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "CREATE TEMP TABLE transaction_probe (value text PRIMARY KEY)"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "INSERT INTO transaction_probe (value) VALUES ($1)", "committed")
		return err
	})
	if err != nil {
		t.Fatalf("commit transaction: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM transaction_probe").Scan(&count); err != nil {
		t.Fatalf("read committed row: %v", err)
	}
	if count != 1 {
		t.Fatalf("committed row count = %d, want 1", count)
	}

	rollbackCause := errors.New("abort transaction")
	err = WithTx(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO transaction_probe (value) VALUES ($1)", "rolled-back"); err != nil {
			return err
		}
		return rollbackCause
	})
	if !errors.Is(err, rollbackCause) {
		t.Fatalf("WithTx error = %v, want callback cause", err)
	}

	if err := pool.QueryRow(ctx, "SELECT count(*) FROM transaction_probe").Scan(&count); err != nil {
		t.Fatalf("read rows after rollback: %v", err)
	}
	if count != 1 {
		t.Fatalf("row count after rollback = %d, want 1", count)
	}

	closedTransactionCause := errors.New("callback closed transaction")
	err = WithTx(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := tx.Rollback(ctx); err != nil {
			return err
		}
		return closedTransactionCause
	})
	if !errors.Is(err, closedTransactionCause) || !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("WithTx error = %v, want callback and rollback causes", err)
	}
}

func testDatabaseURL(t *testing.T) string {
	t.Helper()

	databaseURL := os.Getenv("APP_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set APP_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	return databaseURL
}

func testDatabaseSettings(databaseURL string) config.Database {
	return config.Database{
		URL:               databaseURL,
		MaxConnections:    1,
		MinConnections:    0,
		MaxLifetime:       time.Hour,
		MaxIdleTime:       time.Hour,
		HealthCheckPeriod: time.Minute,
		PingTimeout:       5 * time.Second,
	}
}
