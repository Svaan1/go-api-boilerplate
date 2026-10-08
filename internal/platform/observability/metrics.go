// Package observability provides bounded-cardinality Prometheus instrumentation.
package observability

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics owns an isolated Prometheus registry and HTTP instrumentation.
type Metrics struct {
	registry            *prometheus.Registry
	requests            *prometheus.CounterVec
	duration            *prometheus.HistogramVec
	inFlight            *prometheus.GaugeVec
	responseBytes       *prometheus.HistogramVec
	rateLimitRejections *prometheus.CounterVec
}

// New creates a dedicated registry and registers API metrics.
func New() *Metrics {
	m := &Metrics{registry: prometheus.NewRegistry()}
	m.requests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests served.",
	}, []string{"method", "route", "status"})
	m.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route", "status"})
	m.inFlight = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "http_in_flight",
		Help: "Current number of HTTP requests being served.",
	}, []string{"method", "route"})
	m.responseBytes = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_response_bytes",
		Help:    "HTTP response size in bytes.",
		Buckets: prometheus.ExponentialBuckets(100, 2, 10),
	}, []string{"method", "route", "status"})
	m.rateLimitRejections = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "rate_limit_rejections_total",
		Help: "Total requests rejected by rate limiting.",
	}, []string{"method", "route", "status"})
	m.registry.MustRegister(m.requests, m.duration, m.inFlight, m.responseBytes, m.rateLimitRejections)
	return m
}

// Handler returns Prometheus exposition handler for the dedicated registry.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Wrap records request count, duration, in-flight requests, and response size.
func (m *Metrics) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := safeMethod(r.Method)
		route := routePattern(r)
		m.inFlight.WithLabelValues(method, route).Inc()
		defer m.inFlight.WithLabelValues(method, route).Dec()

		started := time.Now()
		recorder, ok := w.(*Recorder)
		if !ok {
			recorder = NewRecorder(w)
		}
		next.ServeHTTP(recorder, r)
		statusLabel := strconv.Itoa(recorder.Status())
		m.requests.WithLabelValues(method, route, statusLabel).Inc()
		m.duration.WithLabelValues(method, route, statusLabel).Observe(time.Since(started).Seconds())
		m.responseBytes.WithLabelValues(method, route, statusLabel).Observe(float64(recorder.Bytes()))
	})
}

// RateLimitRejected counts throttled requests using only normalized route labels.
func (m *Metrics) RateLimitRejected(r *http.Request) {
	m.rateLimitRejections.WithLabelValues(
		safeMethod(r.Method), routePattern(r), strconv.Itoa(http.StatusTooManyRequests),
	).Inc()
}

func safeMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodConnect,
		http.MethodOptions, http.MethodTrace:
		return method
	default:
		return "OTHER"
	}
}

// ServeMux patterns are static templates. Never promote request URLs to labels.
func routePattern(r *http.Request) string {
	if r.Pattern == "" {
		return "unmatched"
	}
	return r.Pattern
}

// Recorder captures final HTTP status and response bytes while forwarding optional writer operations.
type Recorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

// NewRecorder wraps one request response for both logging and metrics.
func NewRecorder(w http.ResponseWriter) *Recorder { return &Recorder{ResponseWriter: w} }

// Status reports final response status, including implicit 200 responses.
func (w *Recorder) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

// Bytes reports bytes successfully written to underlying response writer.
func (w *Recorder) Bytes() int { return w.bytes }

func (w *Recorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *Recorder) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	// Informational responses do not commit final response status. Preserve 101
	// because protocol switching is a completed response.
	if status >= 100 && status < 200 && status != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *Recorder) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}

func (w *Recorder) Flush() {
	if err := http.NewResponseController(w.ResponseWriter).Flush(); err == nil && w.status == 0 {
		w.status = http.StatusOK
	}
}

func (w *Recorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func (w *Recorder) Push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

func (w *Recorder) ReadFrom(reader io.Reader) (int64, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if fast, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err := fast.ReadFrom(reader)
		w.bytes += int(n)
		return n, err
	}
	return io.Copy(struct{ io.Writer }{w}, reader)
}
