package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/svaan1/go-api-boilerplate/internal/problem"
	"github.com/svaan1/go-api-boilerplate/internal/ratelimit"
)

// RateLimit admits requests by resolved client identity and fails closed on limiter errors.
func RateLimit(next http.Handler, limiter ratelimit.Limiter, onRejected func(*http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decision, err := limiter.Allow(r.Context(), ClientIP(r))
		if err != nil {
			problem.Write(w, r, http.StatusServiceUnavailable, "request admission unavailable", nil)
			return
		}
		w.Header().Set("RateLimit-Limit", strconv.Itoa(decision.Limit))
		w.Header().Set("RateLimit-Remaining", strconv.Itoa(decision.Remaining))
		reset := int((decision.RetryAfter + time.Second - 1) / time.Second)
		if !decision.Allowed && reset < 1 {
			reset = 1
		}
		w.Header().Set("RateLimit-Reset", strconv.Itoa(reset))
		if !decision.Allowed {
			if onRejected != nil {
				onRejected(r)
			}
			w.Header().Set("Retry-After", strconv.Itoa(reset))
			problem.Write(w, r, http.StatusTooManyRequests, "rate limit exceeded", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
