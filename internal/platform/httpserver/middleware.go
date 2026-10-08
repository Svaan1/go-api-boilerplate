package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/svaan1/go-api-boilerplate/internal/platform/httpx"
	"github.com/svaan1/go-api-boilerplate/internal/platform/observability"
)

type requestIDKey struct{}

// RequestID returns generated or validated identifier assigned to request.
func RequestID(r *http.Request) string {
	id, _ := r.Context().Value(requestIDKey{}).(string)
	return id
}

// WithRequestID accepts printable bounded identifiers and otherwise creates random 128-bit ID.
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids := r.Header.Values("X-Request-ID")
		id := ""
		if len(ids) == 1 {
			id = ids[0]
		}
		if len(id) == 0 || len(id) > 128 || strings.IndexFunc(id, func(char rune) bool { return char < 32 || char > 126 }) >= 0 {
			var bits [16]byte
			if _, err := rand.Read(bits[:]); err != nil {
				httpx.WriteProblem(w, r, http.StatusServiceUnavailable, "request id unavailable", nil)
				return
			}
			id = hex.EncodeToString(bits[:])
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

// RequestLogging records bounded route labels, response size and request duration.
func RequestLogging(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := observability.NewRecorder(w)
		next.ServeHTTP(recorder, r)
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		level := slog.LevelInfo
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			level = slog.LevelDebug
		}
		logger.Log(r.Context(), level, "http request", "request_id", RequestID(r), "method", r.Method, "route", route, "status", recorder.Status(), "bytes", recorder.Bytes(), "duration", time.Since(start), "client_ip", ClientIP(r))
	})
}

// RecoverPanic isolates handler panic, logs stack on server, and redacts response.
func RecoverPanic(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				logger.Error("http panic", "request_id", RequestID(r), "stack", string(debug.Stack()))
				httpx.WriteProblem(w, r, http.StatusInternalServerError, "an unexpected error occurred", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// LimitBody bounds request bytes before handlers decode input.
func LimitBody(next http.Handler, maxBytes int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		next.ServeHTTP(w, r)
	})
}
