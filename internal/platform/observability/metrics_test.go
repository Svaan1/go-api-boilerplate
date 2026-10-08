package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsUseServeMuxPatternInsteadOfRequestPath(t *testing.T) {
	metrics := New()
	mux := http.NewServeMux()
	mux.Handle("GET /v1/users/{userID}", metrics.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	})))

	request := httptest.NewRequest(http.MethodGet, "/v1/users/private-user-id?token=private-token", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusCreated)
	}
	if got := testutil.ToFloat64(metrics.requests.WithLabelValues("GET", "GET /v1/users/{userID}", "201")); got != 1 {
		t.Fatalf("request count = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.inFlight.WithLabelValues("GET", "GET /v1/users/{userID}")); got != 0 {
		t.Fatalf("in-flight gauge after request = %v, want 0", got)
	}

	scrape := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(scrape, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if scrape.Code != http.StatusOK {
		t.Fatalf("scrape status = %d, want %d", scrape.Code, http.StatusOK)
	}
	body := scrape.Body.String()
	for _, unsafe := range []string{"private-user-id", "private-token"} {
		if strings.Contains(body, unsafe) {
			t.Fatalf("scrape leaked high-cardinality value %q", unsafe)
		}
	}
	if !strings.Contains(body, `route="GET /v1/users/{userID}"`) {
		t.Fatalf("scrape missing normalized route label: %s", body)
	}
	if !strings.Contains(body, `status="201"`) {
		t.Fatalf("scrape missing status label: %s", body)
	}
	if !strings.Contains(body, "http_duration_seconds_bucket") || !strings.Contains(body, "http_response_bytes_bucket") {
		t.Fatalf("scrape missing histograms: %s", body)
	}
	if !strings.Contains(body, `http_response_bytes_sum{method="GET",route="GET /v1/users/{userID}",status="201"} 7`) {
		t.Fatalf("scrape missing actual response byte count: %s", body)
	}
}

func TestMetricsSanitizeUnknownMethodAndCountRateLimitRejection(t *testing.T) {
	metrics := New()
	metrics.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(
		httptest.NewRecorder(), httptest.NewRequest("SPOOFED-METHOD", "/arbitrary/path", nil),
	)
	metrics.RateLimitRejected(httptest.NewRequest("SPOOFED-METHOD", "/arbitrary/path", nil))

	if got := testutil.ToFloat64(metrics.requests.WithLabelValues("OTHER", "unmatched", "200")); got != 1 {
		t.Fatalf("sanitized request count = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.rateLimitRejections.WithLabelValues("OTHER", "unmatched", "429")); got != 1 {
		t.Fatalf("rate-limit rejection count = %v, want 1", got)
	}
}

func TestMetricsRecordFlushedResponseStatus(t *testing.T) {
	metrics := New()
	metrics.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.(http.Flusher).Flush()
		w.WriteHeader(http.StatusInternalServerError)
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/stream", nil))

	if got := testutil.ToFloat64(metrics.requests.WithLabelValues("GET", "unmatched", "200")); got != 1 {
		t.Fatalf("flushed response count = %v, want 1 for status 200", got)
	}
}
