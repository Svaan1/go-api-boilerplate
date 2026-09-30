package plugin

import (
	"encoding/json"
	"net/http"

	"github.com/svaan1/go-api-boilerplate/internal/auth"
	"github.com/svaan1/go-api-boilerplate/internal/problem"
)

type systemPlugin struct{}

func (systemPlugin) Name() string { return "system" }

func (systemPlugin) RegisterRoutes(mux *http.ServeMux, deps Dependencies) error {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := auth.FromContext(r.Context())
		if !ok {
			problem.Write(w, r, http.StatusUnauthorized, "authentication required", nil)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(identity)
	})
	mux.Handle("GET /v1/me", deps.RequireAuthenticated(handler))
	return nil
}
