package httpserver

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/svaan1/go-api-boilerplate/internal/platform/config"
)

func TestSecurityHeaders(t *testing.T) {
	for _, tc := range []struct {
		name     string
		env      string
		hsts     bool
		wantHSTS bool
	}{
		{name: "production TLS termination enabled", env: "production", hsts: true, wantHSTS: true},
		{name: "production TLS termination disabled", env: "production"},
		{name: "development configured", env: "development", hsts: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nextCalled := false
			handler := SecurityHeaders(config.Config{Env: tc.env, HTTP: config.HTTP{HSTS: tc.hsts}})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusNoContent)
			}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

			for name, want := range map[string]string{
				"X-Content-Type-Options": "nosniff",
				"X-Frame-Options":        "DENY",
				"Referrer-Policy":        "no-referrer",
			} {
				if got := response.Header().Get(name); got != want {
					t.Errorf("%s = %q, want %q", name, got, want)
				}
			}
			if got := response.Header().Get("Content-Security-Policy"); got != "default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'" {
				t.Errorf("Content-Security-Policy = %q", got)
			}
			gotHSTS := response.Header().Get("Strict-Transport-Security") != ""
			if gotHSTS != tc.wantHSTS {
				t.Errorf("HSTS present = %t, want %t", gotHSTS, tc.wantHSTS)
			}
			if !nextCalled {
				t.Fatal("handler was not called")
			}
		})
	}
}

func TestCORSPreflight(t *testing.T) {
	cfg := config.Config{CORS: config.CORS{
		Origins:     []string{"https://app.example"},
		Methods:     []string{http.MethodGet, http.MethodPost},
		Headers:     []string{"Content-Type", "Authorization"},
		Credentials: true,
	}}
	for _, tc := range []struct {
		name             string
		origin           string
		method           string
		headers          string
		wantStatus       int
		wantAllowOrigin  string
		wantAllowHeaders string
	}{
		{name: "allowed", origin: "https://app.example", method: http.MethodPost, headers: "content-type, Authorization", wantStatus: http.StatusNoContent, wantAllowOrigin: "https://app.example", wantAllowHeaders: "content-type, Authorization"},
		{name: "unapproved origin", origin: "https://attacker.example", method: http.MethodPost, headers: "Content-Type", wantStatus: http.StatusForbidden},
		{name: "unapproved method", origin: "https://app.example", method: http.MethodDelete, headers: "Content-Type", wantStatus: http.StatusForbidden},
		{name: "unapproved header", origin: "https://app.example", method: http.MethodPost, headers: "X-Evil", wantStatus: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nextCalled := false
			handler := CORS(cfg)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { nextCalled = true }))
			request := httptest.NewRequest(http.MethodOptions, "/resource", nil)
			request.Header.Set("Origin", tc.origin)
			request.Header.Set("Access-Control-Request-Method", tc.method)
			if tc.headers != "" {
				request.Header.Set("Access-Control-Request-Headers", tc.headers)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, tc.wantStatus)
			}
			if nextCalled {
				t.Error("preflight reached handler")
			}
			if got := response.Header().Get("Access-Control-Allow-Origin"); got != tc.wantAllowOrigin {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tc.wantAllowOrigin)
			}
			if got := response.Header().Get("Access-Control-Allow-Headers"); got != tc.wantAllowHeaders {
				t.Errorf("Access-Control-Allow-Headers = %q, want %q", got, tc.wantAllowHeaders)
			}
			if tc.wantStatus == http.StatusForbidden && (response.Header().Get("Access-Control-Allow-Origin") != "" || response.Header().Get("Access-Control-Allow-Headers") != "") {
				t.Error("rejected preflight exposed CORS permissions")
			}
			if tc.wantStatus == http.StatusNoContent && response.Header().Get("Access-Control-Allow-Methods") != "GET, POST" {
				t.Errorf("Access-Control-Allow-Methods = %q", response.Header().Get("Access-Control-Allow-Methods"))
			}
			if tc.wantStatus == http.StatusNoContent && response.Header().Get("Access-Control-Allow-Credentials") != "true" {
				t.Error("allowed credentialed request missing credentials permission")
			}
			if got := response.Header().Values("Vary"); !reflect.DeepEqual(got, []string{"Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers"}) {
				t.Errorf("Vary = %v", got)
			}
		})
	}
}

func TestCORSDoesNotReflectUnapprovedOrigin(t *testing.T) {
	nextCalled := false
	handler := CORS(config.Config{CORS: config.CORS{Origins: []string{"https://trusted.example"}, Methods: []string{http.MethodGet}}})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Origin", "https://attacker.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !nextCalled {
		t.Fatal("non-preflight request did not reach handler")
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("unapproved origin reflected as %q", got)
	}
	if got := response.Header().Values("Vary"); !reflect.DeepEqual(got, []string{"Origin"}) {
		t.Errorf("Vary = %v, want [Origin]", got)
	}
}

func TestCORSWildcardCredentialsFailsClosed(t *testing.T) {
	handler := CORS(config.Config{CORS: config.CORS{Origins: []string{"*"}, Methods: []string{http.MethodGet}, Credentials: true}})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("wildcard credentials preflight reached handler")
	}))
	request := httptest.NewRequest(http.MethodOptions, "/", nil)
	request.Header.Set("Origin", "https://app.example")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("wildcard credentials response: status=%d allow-origin=%q", response.Code, response.Header().Get("Access-Control-Allow-Origin"))
	}
}
