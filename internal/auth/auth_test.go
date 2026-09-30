package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/svaan1/go-api-boilerplate/internal/config"
)

const secret = "12345678901234567890123456789012"

func signed(t *testing.T, changes jwt.MapClaims, key string) string {
	t.Helper()
	claims := jwt.MapClaims{"sub": "smoke-user", "iss": "issuer", "aud": "api", "exp": time.Now().Add(time.Minute).Unix(), "nbf": time.Now().Add(-time.Minute).Unix(), "roles": []string{"admin"}, "permissions": []string{"profile:read"}}
	for name, value := range changes {
		claims[name] = value
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(key))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestHMACVerifierRejectsInvalidClaims(t *testing.T) {
	verifier, err := NewHMACVerifier(config.Auth{HMACSecret: secret, Issuer: "issuer", Audience: "api", Algorithm: "HS256"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		changes jwt.MapClaims
		key     string
	}{
		{name: "invalid signature", key: strings.Repeat("z", 32)},
		{name: "expired", changes: jwt.MapClaims{"exp": time.Now().Add(-time.Second).Unix()}},
		{name: "not before", changes: jwt.MapClaims{"nbf": time.Now().Add(time.Hour).Unix()}},
		{name: "wrong issuer", changes: jwt.MapClaims{"iss": "other"}},
		{name: "wrong audience", changes: jwt.MapClaims{"aud": "other"}},
		{name: "missing subject", changes: jwt.MapClaims{"sub": ""}},
		{name: "invalid role type", changes: jwt.MapClaims{"roles": []any{"admin", 1}}},
		{name: "invalid permission type", changes: jwt.MapClaims{"permissions": "profile:read"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := tc.key
			if key == "" {
				key = secret
			}
			if _, err := verifier.Verify(context.Background(), signed(t, tc.changes, key)); err == nil {
				t.Fatal("accepted invalid token")
			}
		})
	}
	identity, err := verifier.Verify(context.Background(), signed(t, nil, secret))
	if err != nil || identity.Subject != "smoke-user" || len(identity.Permissions) != 1 {
		t.Fatalf("valid identity: %+v error=%v", identity, err)
	}
}

func TestPermissionGuard(t *testing.T) {
	verifier, _ := NewHMACVerifier(config.Auth{HMACSecret: secret, Issuer: "issuer", Audience: "api", Algorithm: "HS256"})
	handler := Authenticate(verifier)(RequirePermission("profile:read", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })))
	for _, tc := range []struct {
		name, token string
		want        int
	}{
		{name: "anonymous", want: 401},
		{name: "missing permission", token: signed(t, jwt.MapClaims{"permissions": []string{}}, secret), want: 403},
		{name: "allowed", token: signed(t, nil, secret), want: 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/fixture", nil)
			if tc.token != "" {
				request.Header.Set("Authorization", "Bearer "+tc.token)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, request)
			if w.Code != tc.want {
				t.Fatalf("status %d want %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
