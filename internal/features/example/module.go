// Package example is the reference feature slice. Copy it to start a new
// feature: domain (entities, stdlib only), application (use cases and ports),
// adapters (http inbound; postgres and memory outbound), and this module.
package example

import (
	"log/slog"
	"net/http"

	examplehttp "github.com/svaan1/go-api-boilerplate/internal/features/example/adapters/http"
	"github.com/svaan1/go-api-boilerplate/internal/features/example/application"
	"github.com/svaan1/go-api-boilerplate/internal/platform/httpserver"
)

// Dependencies are the outbound ports and shared services supplied by the
// composition root.
type Dependencies struct {
	Logger     *slog.Logger
	Repository application.Repository
	Transactor application.Transactor
	Clock      application.Clock
	IDs        application.IDGenerator
}

// Module is the example feature.
type Module struct {
	handler *examplehttp.Handler
}

var _ httpserver.Module = (*Module)(nil)

// New wires use cases and the HTTP adapter.
func New(deps Dependencies) *Module {
	return &Module{handler: examplehttp.NewHandler(
		application.NewCreateExample(deps.Repository, deps.Clock, deps.IDs),
		application.NewRenameExample(deps.Repository, deps.Transactor, deps.Clock),
		deps.Logger,
	)}
}

// Name implements [httpserver.Module].
func (*Module) Name() string { return "example" }

// RegisterRoutes implements [httpserver.Module].
func (m *Module) RegisterRoutes(mux *http.ServeMux) error {
	m.handler.RegisterRoutes(mux)
	return nil
}
