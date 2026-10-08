package httpserver

import (
	"errors"
	"fmt"
	"net/http"
)

// Module is a feature slice served by the API. Modules receive their
// dependencies through their constructors and only register routes here.
type Module interface {
	// Name identifies the module in startup errors; it must be unique.
	Name() string
	// RegisterRoutes installs the module's routes on mux.
	RegisterRoutes(mux *http.ServeMux) error
}

// registerModules installs every module's routes in order. Nil modules,
// duplicate names, registration errors, and ServeMux pattern conflicts fail
// startup instead of serving a partial route table.
func registerModules(mux *http.ServeMux, modules []Module) error {
	seen := make(map[string]struct{}, len(modules))
	for _, module := range modules {
		if module == nil {
			return errors.New("registering modules: nil module")
		}
		name := module.Name()
		if _, exists := seen[name]; exists {
			return fmt.Errorf("registering modules: duplicate module %q", name)
		}
		seen[name] = struct{}{}
		if err := registerRoutes(mux, module); err != nil {
			return err
		}
	}
	return nil
}

func registerRoutes(mux *http.ServeMux, module Module) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("registering module %q: invalid route registration", module.Name())
		}
	}()
	if err := module.RegisterRoutes(mux); err != nil {
		return fmt.Errorf("registering module %q: %w", module.Name(), err)
	}
	return nil
}
