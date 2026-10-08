package httpserver

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/svaan1/go-api-boilerplate/internal/platform/config"
	"github.com/svaan1/go-api-boilerplate/internal/platform/ratelimit"
)

type routeModule struct {
	name    string
	pattern string
	err     error
}

func (m routeModule) Name() string { return m.name }

func (m routeModule) RegisterRoutes(mux *http.ServeMux) error {
	if m.err != nil {
		return m.err
	}
	mux.HandleFunc(m.pattern, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-ID", r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})
	return nil
}

func newTestRouter(modules ...Module) (http.Handler, error) {
	return NewRouter(config.Config{}, Dependencies{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Limiter: ratelimit.New(config.RateLimit{
			RequestsPerSecond: 100, Burst: 100, MaxClients: 100, IdleTTL: time.Minute, CleanupInterval: time.Minute,
		}, nil),
	}, modules)
}

func TestNewRouterRejectsInvalidModules(t *testing.T) {
	for _, tc := range []struct {
		name    string
		modules []Module
	}{
		{name: "nil module", modules: []Module{nil}},
		{name: "duplicate name", modules: []Module{routeModule{name: "a", pattern: "GET /a"}, routeModule{name: "a", pattern: "GET /b"}}},
		{name: "registration error", modules: []Module{routeModule{name: "a", err: errors.New("boom")}}},
		{name: "conflicting pattern", modules: []Module{routeModule{name: "a", pattern: "GET /a"}, routeModule{name: "b", pattern: "GET /a"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := newTestRouter(tc.modules...); err == nil {
				t.Fatal("NewRouter returned nil error")
			}
		})
	}
}

func TestNewRouterServesModuleRoutesAndAPINotFound(t *testing.T) {
	router, err := newTestRouter(routeModule{name: "a", pattern: "GET /v1/items/{id}"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path   string
		want   int
		wantID string
	}{
		{path: "/v1/items/42", want: http.StatusNoContent, wantID: "42"},
		{path: "/v1/missing", want: http.StatusNotFound},
	} {
		w := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, tc.path, nil)
		request.RemoteAddr = "192.0.2.1:1234"
		router.ServeHTTP(w, request)
		if w.Code != tc.want || w.Header().Get("X-ID") != tc.wantID {
			t.Fatalf("%s status = %d id = %q, want %d %q", tc.path, w.Code, w.Header().Get("X-ID"), tc.want, tc.wantID)
		}
	}
}
