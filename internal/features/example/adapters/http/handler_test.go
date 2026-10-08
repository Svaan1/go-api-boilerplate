package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	examplehttp "github.com/svaan1/go-api-boilerplate/internal/features/example/adapters/http"
	"github.com/svaan1/go-api-boilerplate/internal/features/example/adapters/memory"
	"github.com/svaan1/go-api-boilerplate/internal/features/example/application"
	"github.com/svaan1/go-api-boilerplate/internal/features/example/domain"
	"github.com/svaan1/go-api-boilerplate/internal/platform/auth"
)

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }

type sequenceIDs struct{ next int }

func (s *sequenceIDs) NewID() string {
	s.next++
	return "id-" + strconv.Itoa(s.next)
}

type directTransactor struct{}

func (directTransactor) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type failingRepository struct{ application.Repository }

func (failingRepository) Create(context.Context, domain.Example) error {
	return errors.New("private driver detail")
}

type allowAll struct{}

func (allowAll) Verify(context.Context, string) (auth.Identity, error) {
	return auth.Identity{Subject: "tester", Roles: []string{}, Permissions: []string{}}, nil
}

func newServer(repository application.Repository, logs *bytes.Buffer) http.Handler {
	logger := slog.New(slog.NewTextHandler(logs, nil))
	handler := examplehttp.NewHandler(
		application.NewCreateExample(repository, fixedClock{}, &sequenceIDs{}),
		application.NewRenameExample(repository, directTransactor{}, fixedClock{}),
		logger,
	)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return auth.Authenticate(allowAll{})(mux)
}

type call struct {
	method, path, contentType, body string
	anonymous                       bool
}

func do(server http.Handler, c call) *httptest.ResponseRecorder {
	request := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
	if c.contentType != "" {
		request.Header.Set("Content-Type", c.contentType)
	}
	if !c.anonymous {
		request.Header.Set("Authorization", "Bearer token")
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}

func TestCreateAndRenameSuccess(t *testing.T) {
	server := newServer(memory.NewRepository(), &bytes.Buffer{})

	created := do(server, call{method: http.MethodPost, path: "/v1/examples", contentType: "application/json", body: `{"name":" first "}`})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", created.Code, created.Body.String())
	}
	want := `{"id":"id-1","name":"first","created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-02T03:04:05Z"}`
	if got := strings.TrimSpace(created.Body.String()); got != want {
		t.Fatalf("create body = %s, want %s", got, want)
	}

	renamed := do(server, call{method: http.MethodPatch, path: "/v1/examples/id-1", contentType: "application/json", body: `{"name":"second"}`})
	if renamed.Code != http.StatusOK {
		t.Fatalf("rename status = %d body=%s", renamed.Code, renamed.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(renamed.Body.Bytes(), &body); err != nil || body["name"] != "second" || body["id"] != "id-1" {
		t.Fatalf("rename body = %s (%v)", renamed.Body.String(), err)
	}
}

func TestErrorMapping(t *testing.T) {
	repository := memory.NewRepository()
	server := newServer(repository, &bytes.Buffer{})
	if code := do(server, call{method: http.MethodPost, path: "/v1/examples", contentType: "application/json", body: `{"name":"taken"}`}).Code; code != http.StatusCreated {
		t.Fatalf("seed status = %d", code)
	}

	for _, tc := range []struct {
		name       string
		call       call
		wantStatus int
		wantField  string
	}{
		{name: "anonymous", call: call{method: http.MethodPost, path: "/v1/examples", contentType: "application/json", body: `{"name":"x"}`, anonymous: true}, wantStatus: http.StatusUnauthorized},
		{name: "wrong content type", call: call{method: http.MethodPost, path: "/v1/examples", contentType: "text/plain", body: `{"name":"x"}`}, wantStatus: http.StatusUnsupportedMediaType},
		{name: "malformed json", call: call{method: http.MethodPost, path: "/v1/examples", contentType: "application/json", body: `{"name":`}, wantStatus: http.StatusBadRequest},
		{name: "unknown field", call: call{method: http.MethodPost, path: "/v1/examples", contentType: "application/json", body: `{"name":"x","extra":1}`}, wantStatus: http.StatusBadRequest},
		{name: "invalid name", call: call{method: http.MethodPost, path: "/v1/examples", contentType: "application/json", body: `{"name":"  "}`}, wantStatus: http.StatusUnprocessableEntity, wantField: "name"},
		{name: "duplicate name", call: call{method: http.MethodPost, path: "/v1/examples", contentType: "application/json", body: `{"name":"taken"}`}, wantStatus: http.StatusConflict},
		{name: "rename unknown id", call: call{method: http.MethodPatch, path: "/v1/examples/missing", contentType: "application/json", body: `{"name":"x"}`}, wantStatus: http.StatusNotFound},
		{name: "rename invalid name", call: call{method: http.MethodPatch, path: "/v1/examples/id-1", contentType: "application/json", body: `{"name":""}`}, wantStatus: http.StatusUnprocessableEntity, wantField: "name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := do(server, tc.call)
			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tc.wantStatus, response.Body.String())
			}
			if got := response.Header().Get("Content-Type"); got != "application/problem+json" {
				t.Fatalf("Content-Type = %q, want application/problem+json", got)
			}
			var problem struct {
				Status int `json:"status"`
				Errors []struct {
					Field string `json:"field"`
				} `json:"errors"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil || problem.Status != tc.wantStatus {
				t.Fatalf("problem body = %s (%v)", response.Body.String(), err)
			}
			if tc.wantField != "" && (len(problem.Errors) != 1 || problem.Errors[0].Field != tc.wantField) {
				t.Fatalf("validation errors = %+v, want field %q", problem.Errors, tc.wantField)
			}
		})
	}
}

func TestUnexpectedErrorIsRedactedAndLogged(t *testing.T) {
	var logs bytes.Buffer
	server := newServer(failingRepository{memory.NewRepository()}, &logs)
	response := do(server, call{method: http.MethodPost, path: "/v1/examples", contentType: "application/json", body: `{"name":"x"}`})
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "private driver detail") {
		t.Fatalf("status = %d body=%s; want redacted 500", response.Code, response.Body.String())
	}
	if !strings.Contains(logs.String(), "private driver detail") {
		t.Fatalf("cause not logged server-side: %s", logs.String())
	}
}
