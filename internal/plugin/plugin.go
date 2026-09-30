// Package plugin registers explicitly enabled HTTP API extensions.
package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/svaan1/go-api-boilerplate/internal/auth"
	"github.com/svaan1/go-api-boilerplate/internal/database"
)

// Plugin owns its route registration and names itself for configuration.
type Plugin interface {
	Name() string
	RegisterRoutes(*http.ServeMux, Dependencies) error
}

// Dependencies contains only capabilities exposed to route plugins.
type Dependencies struct {
	Logger               *slog.Logger
	Pool                 *pgxpool.Pool
	WithTx               func(context.Context, pgx.TxOptions, func(pgx.Tx) error) error
	RequireAuthenticated func(http.Handler) http.Handler
	RequireRole          func(string, http.Handler) http.Handler
	RequirePermission    func(string, http.Handler) http.Handler
}

// Register installs routes for enabled built-in plugins. Unknown and repeated
// names fail before any plugin route is registered.
func Register(mux *http.ServeMux, enabled []string, deps Dependencies) error {
	if mux == nil {
		return errors.New("registering plugins: nil ServeMux")
	}

	available := map[string]Plugin{"system": systemPlugin{}}
	selected := make([]Plugin, 0, len(enabled))
	seen := make(map[string]struct{}, len(enabled))
	for _, name := range enabled {
		if _, exists := seen[name]; exists {
			return fmt.Errorf("registering plugins: duplicate plugin %q", name)
		}
		seen[name] = struct{}{}

		plugin, exists := available[name]
		if !exists {
			return fmt.Errorf("registering plugins: unknown plugin %q", name)
		}
		selected = append(selected, plugin)
	}

	deps = withDefaultGuards(deps)
	for _, plugin := range selected {
		if err := registerRoutes(mux, plugin, deps); err != nil {
			return err
		}
	}
	return nil
}

func withDefaultGuards(deps Dependencies) Dependencies {
	if deps.RequireAuthenticated == nil {
		deps.RequireAuthenticated = auth.RequireAuthenticated
	}
	if deps.RequireRole == nil {
		deps.RequireRole = auth.RequireRole
	}
	if deps.RequirePermission == nil {
		deps.RequirePermission = auth.RequirePermission
	}
	if deps.WithTx == nil && deps.Pool != nil {
		deps.WithTx = func(ctx context.Context, options pgx.TxOptions, fn func(pgx.Tx) error) error {
			return database.WithTx(ctx, deps.Pool, options, fn)
		}
	}
	return deps
}

func registerRoutes(mux *http.ServeMux, plugin Plugin, deps Dependencies) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("registering plugin %q: invalid route registration", plugin.Name())
		}
	}()
	if err := plugin.RegisterRoutes(mux, deps); err != nil {
		return fmt.Errorf("registering plugin %q: %w", plugin.Name(), err)
	}
	return nil
}
