// Package auth verifies bearer identities and applies route-specific guards.
package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/svaan1/go-api-boilerplate/internal/config"
	"github.com/svaan1/go-api-boilerplate/internal/problem"
)

// Identity is request-scoped, with initialized role and permission collections.
type Identity struct {
	Subject     string   `json:"subject"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

// Verifier supports replacement of local HMAC verification without handler changes.
type Verifier interface {
	Verify(context.Context, string) (Identity, error)
}

type contextKey struct{}

// FromContext returns identity only after successful bearer verification.
func FromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(contextKey{}).(Identity)
	return identity, ok
}

// HMACVerifier verifies HS256 signatures and required registered claims.
type HMACVerifier struct {
	secret   []byte
	issuer   string
	audience string
}

// NewHMACVerifier constructs a verifier from validated configuration.
func NewHMACVerifier(cfg config.Auth) (*HMACVerifier, error) {
	if cfg.Algorithm != "HS256" || len(cfg.HMACSecret) < 32 || cfg.Issuer == "" || cfg.Audience == "" {
		return nil, errors.New("invalid jwt verifier configuration")
	}
	return &HMACVerifier{secret: []byte(cfg.HMACSecret), issuer: cfg.Issuer, audience: cfg.Audience}, nil
}

// Verify rejects invalid signature, time, issuer, audience, subject, and claim types.
func (v *HMACVerifier) Verify(_ context.Context, token string) (Identity, error) {
	invalid := Identity{}
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) { return v.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(v.issuer), jwt.WithAudience(v.audience), jwt.WithExpirationRequired())
	if err != nil || !parsed.Valid {
		return invalid, problem.ErrUnauthorized
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return invalid, problem.ErrUnauthorized
	}
	subject, ok := claims["sub"].(string)
	if !ok || subject == "" {
		return invalid, problem.ErrUnauthorized
	}
	roles, ok := stringArray(claims["roles"])
	if !ok {
		return invalid, problem.ErrUnauthorized
	}
	permissions, ok := stringArray(claims["permissions"])
	if !ok {
		return invalid, problem.ErrUnauthorized
	}
	return Identity{Subject: subject, Roles: roles, Permissions: permissions}, nil
}

func stringArray(value any) ([]string, bool) {
	result := []string{}
	if value == nil {
		return result, true
	}
	entries, ok := value.([]any)
	if !ok {
		return nil, false
	}
	result = make([]string, 0, len(entries))
	for _, entry := range entries {
		item, ok := entry.(string)
		if !ok || item == "" {
			return nil, false
		}
		result = append(result, item)
	}
	return result, true
}

// Authenticate allows anonymous public requests but rejects malformed or invalid bearer credentials.
func Authenticate(verifier Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			headers := r.Header.Values("Authorization")
			if len(headers) == 0 {
				next.ServeHTTP(w, r)
				return
			}
			parts := strings.Fields(headers[0])
			if len(headers) != 1 || len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				problem.WriteError(w, r, nil, problem.ErrUnauthorized)
				return
			}
			identity, err := verifier.Verify(r.Context(), parts[1])
			if err != nil {
				problem.WriteError(w, r, nil, problem.ErrUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, identity)))
		})
	}
}

// RequireAuthenticated gates a route without altering public routes.
func RequireAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := FromContext(r.Context()); !ok {
			problem.WriteError(w, r, nil, problem.ErrUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRole gates a route on a verified role.
func RequireRole(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := FromContext(r.Context())
		if !ok {
			problem.WriteError(w, r, nil, problem.ErrUnauthorized)
			return
		}
		for _, candidate := range identity.Roles {
			if candidate == role {
				next.ServeHTTP(w, r)
				return
			}
		}
		problem.WriteError(w, r, nil, problem.ErrForbidden)
	})
}

// RequirePermission gates a route on a verified permission.
func RequirePermission(permission string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := FromContext(r.Context())
		if !ok {
			problem.WriteError(w, r, nil, problem.ErrUnauthorized)
			return
		}
		for _, candidate := range identity.Permissions {
			if candidate == permission {
				next.ServeHTTP(w, r)
				return
			}
		}
		problem.WriteError(w, r, nil, problem.ErrForbidden)
	})
}
