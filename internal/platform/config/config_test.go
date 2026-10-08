package config

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadEnvironmentOverrides(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_DATABASE_URL", "postgres://local:local@localhost:5432/api")
	t.Setenv("APP_AUTH_HMAC_SECRET", strings.Repeat("x", 32))
	t.Setenv("APP_HTTP_ADDRESS", "127.0.0.1:9182")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != "test" || cfg.HTTP.Address != "127.0.0.1:9182" || cfg.Auth.HMACSecret != strings.Repeat("x", 32) {
		t.Fatalf("environment override lost: env=%q address=%q secret length=%d", cfg.Env, cfg.HTTP.Address, len(cfg.Auth.HMACSecret))
	}
}

func TestLoadStageFilePrecedence(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "test.yaml"),
		[]byte("http:\n  address: 127.0.0.1:9300\n  idle_timeout: 7s\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_DATABASE_URL", "postgres://local:local@localhost:5432/api")
	t.Setenv("APP_AUTH_HMAC_SECRET", strings.Repeat("x", 32))
	t.Setenv("APP_HTTP_ADDRESS", "127.0.0.1:9400")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Address != "127.0.0.1:9400" || cfg.HTTP.IdleTimeout != 7*time.Second ||
		cfg.HTTP.MaxBodyBytes != 1048576 {
		t.Fatalf("incorrect env/file/default precedence: address=%q idle=%s body=%d",
			cfg.HTTP.Address, cfg.HTTP.IdleTimeout, cfg.HTTP.MaxBodyBytes)
	}
}

func TestLoadCommaSeparatedEnvironmentLists(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_DATABASE_URL", "postgres://local:local@localhost:5432/api")
	t.Setenv("APP_AUTH_HMAC_SECRET", strings.Repeat("x", 32))
	t.Setenv("APP_CORS_ORIGINS", "https://one.example,https://two.example")
	t.Setenv("APP_HTTP_TRUSTED_PROXIES", "127.0.0.1/32,10.0.0.0/8")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.CORS.Origins) != 2 || len(cfg.HTTP.TrustedProxies) != 2 {
		t.Fatalf("environment lists not decoded: cors=%v proxies=%v", cfg.CORS.Origins, cfg.HTTP.TrustedProxies)
	}
}

func TestLoadRejectsProductionDebug(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_DATABASE_URL", "postgres://local:local@localhost:5432/api")
	t.Setenv("APP_AUTH_HMAC_SECRET", strings.Repeat("x", 32))
	t.Setenv("APP_OBSERVABILITY_DEBUG", "true")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "debug logging") {
		t.Fatalf("expected production logging rejection, got %v", err)
	}
}

func TestDevelopmentDebugLoggingOverride(t *testing.T) {
	cfg := Config{Env: "development", Observability: Observability{Debug: true}}
	if !cfg.Logger().Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("development debug messages suppressed")
	}
	cfg.Observability.Debug = false
	if cfg.Logger().Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("explicit debug override did not suppress debug messages")
	}
}
