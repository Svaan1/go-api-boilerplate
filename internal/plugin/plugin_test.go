package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/svaan1/go-api-boilerplate/internal/auth"
)

type testVerifier struct{ identity auth.Identity }

func (v testVerifier) Verify(context.Context, string) (auth.Identity, error) {
	return v.identity, nil
}

func TestRegisterSelection(t *testing.T) {
	tests := []struct {
		name      string
		enabled   []string
		wantError bool
	}{
		{name: "disabled plugins", enabled: []string{}},
		{name: "unknown plugin", enabled: []string{"missing"}, wantError: true},
		{name: "duplicate plugin", enabled: []string{"system", "system"}, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mux := http.NewServeMux()
			err := Register(mux, test.enabled, Dependencies{})
			if (err != nil) != test.wantError {
				t.Fatalf("Register() error = %v, want error %t", err, test.wantError)
			}
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/me", nil))
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("route status = %d, want %d", recorder.Code, http.StatusNotFound)
			}
		})
	}
}

func TestSystemPluginMeRequiresAuthenticationAndReturnsIdentity(t *testing.T) {
	mux := http.NewServeMux()
	if err := Register(mux, []string{"system"}, Dependencies{}); err != nil {
		t.Fatal(err)
	}
	identity := auth.Identity{
		Subject:     "subject-1",
		Roles:       []string{"admin"},
		Permissions: []string{"profile:read"},
	}
	handler := auth.Authenticate(testVerifier{identity: identity})(mux)

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/me", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	request.Header.Set("Authorization", "Bearer valid")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding identity: %v", err)
	}
	want := map[string]any{
		"subject":     "subject-1",
		"roles":       []any{"admin"},
		"permissions": []any{"profile:read"},
	}
	if !reflect.DeepEqual(body, want) {
		t.Fatalf("identity JSON = %#v, want %#v", body, want)
	}
}

func TestRegisterRejectsNilMux(t *testing.T) {
	if err := Register(nil, nil, Dependencies{}); err == nil {
		t.Fatal("Register(nil, ...) returned nil error")
	}
}
