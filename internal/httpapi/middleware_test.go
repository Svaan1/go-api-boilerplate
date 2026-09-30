package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestIDPanicRecovery(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	handler := WithRequestID(RequestLogging(RecoverPanic(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestID(r) == "" {
			t.Error("missing request id in handler")
		}
		panic("internal authorization value")
	}), logger), logger))
	request := httptest.NewRequest(http.MethodGet, "/v1/crash?token=private", nil)
	request.Header.Set("Authorization", "Bearer ultra-private")
	request.Header.Set("X-Request-ID", "bad\nvalue")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	if w.Code != 500 || len(w.Header().Get("X-Request-ID")) != 32 {
		t.Fatalf("panic response: %d id=%q", w.Code, w.Header().Get("X-Request-ID"))
	}
	body := w.Body.String()
	if strings.Contains(body, "internal authorization") || strings.Contains(body, "ultra-private") || strings.Contains(logs.String(), "ultra-private") || !strings.Contains(logs.String(), "stack=") {
		t.Fatalf("panic disclosure or missing stack: body=%s logs=%s", body, logs.String())
	}
}
