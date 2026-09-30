package httpapi

import (
	"net/http"
	"slices"
	"strings"

	"github.com/svaan1/go-api-boilerplate/internal/config"
	"github.com/svaan1/go-api-boilerplate/internal/problem"
)

// SecurityHeaders adds defensive browser response headers.
func SecurityHeaders(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := w.Header()
			header.Set("X-Content-Type-Options", "nosniff")
			header.Set("X-Frame-Options", "DENY")
			header.Set("Referrer-Policy", "no-referrer")
			header.Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
			if cfg.Env == "production" && cfg.HTTP.HSTS {
				header.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORS applies configured cross-origin access rules and terminates preflight requests.
func CORS(cfg config.Config) func(http.Handler) http.Handler {
	origins := slices.Clone(cfg.CORS.Origins)
	methods := slices.Clone(cfg.CORS.Methods)
	headers := slices.Clone(cfg.CORS.Headers)
	credentials := cfg.CORS.Credentials

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			preflightMethod := r.Header.Get("Access-Control-Request-Method")
			isPreflight := r.Method == http.MethodOptions && preflightMethod != ""

			vary(w.Header(), "Origin")
			if isPreflight {
				vary(w.Header(), "Access-Control-Request-Method")
				vary(w.Header(), "Access-Control-Request-Headers")
			}

			allowedOrigin, wildcard := corsOriginAllowed(origins, origin)
			if isPreflight {
				requestedHeaders := requestedHeaderNames(r.Header.Values("Access-Control-Request-Headers"))
				if wildcard && credentials {
					allowedOrigin = false
				}
				if !allowedOrigin || !corsMethodAllowed(methods, preflightMethod) || !corsHeadersAllowed(headers, requestedHeaders) {
					problem.Write(w, r, http.StatusForbidden, "cross-origin request not allowed", nil)
					return
				}
				setCORSOrigin(w.Header(), origin, wildcard, credentials)
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(methods, ", "))
				if len(requestedHeaders) != 0 {
					w.Header().Set("Access-Control-Allow-Headers", strings.Join(requestedHeaders, ", "))
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if allowedOrigin {
				setCORSOrigin(w.Header(), origin, wildcard, credentials)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func corsOriginAllowed(origins []string, origin string) (allowed, wildcard bool) {
	if origin == "" {
		return false, false
	}
	for _, configured := range origins {
		if configured == "*" {
			return true, true
		}
		if configured == origin {
			return true, false
		}
	}
	return false, false
}

func corsMethodAllowed(methods []string, method string) bool {
	return slices.Contains(methods, method)
}

func corsHeadersAllowed(allowed, requested []string) bool {
	for _, name := range requested {
		if !slices.ContainsFunc(allowed, func(configured string) bool {
			return strings.EqualFold(configured, name)
		}) {
			return false
		}
	}
	return true
}

func requestedHeaderNames(values []string) []string {
	var names []string
	for _, value := range values {
		for _, name := range strings.Split(value, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

func setCORSOrigin(header http.Header, origin string, wildcard, credentials bool) {
	if wildcard {
		if credentials {
			return
		}
		header.Set("Access-Control-Allow-Origin", "*")
	} else {
		header.Set("Access-Control-Allow-Origin", origin)
	}
	if credentials {
		header.Set("Access-Control-Allow-Credentials", "true")
	}
}

func vary(header http.Header, value string) {
	for _, existing := range header.Values("Vary") {
		for _, token := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(token), value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}
