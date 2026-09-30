package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/svaan1/go-api-boilerplate/internal/config"
	"github.com/svaan1/go-api-boilerplate/internal/ratelimit"
)

func TestRateLimitDeniedResponse(t *testing.T) {
	now := time.Unix(0, 0)
	limiter := ratelimit.New(config.RateLimit{RequestsPerSecond: 1, Burst: 1, MaxClients: 5, IdleTTL: time.Minute, CleanupInterval: time.Minute}, func() time.Time { return now })
	handler := RateLimit(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }), limiter, nil)
	for attempt := range 2 {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/resource", nil))
		if attempt == 0 && (response.Code != 200 || response.Header().Get("RateLimit-Reset") != "1") {
			t.Fatalf("first response status=%d reset=%q", response.Code, response.Header().Get("RateLimit-Reset"))
		}
		if attempt == 1 && (response.Code != 429 || response.Header().Get("Retry-After") != "1" || response.Header().Get("RateLimit-Remaining") != "0" || !strings.Contains(response.Body.String(), `"status":429`)) {
			t.Fatalf("denied status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
		}
	}
}

type failedLimiter struct{}

func (failedLimiter) Allow(context.Context, string) (ratelimit.Decision, error) {
	return ratelimit.Decision{}, context.DeadlineExceeded
}
func TestRateLimitFailsClosed(t *testing.T) {
	w := httptest.NewRecorder()
	RateLimit(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("admitted on error") }), failedLimiter{}, nil).ServeHTTP(w, httptest.NewRequest("GET", "/resource", nil))
	if w.Code != 503 {
		t.Fatalf("status=%d", w.Code)
	}
}
