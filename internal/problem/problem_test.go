package problem

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWrite(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("GET", "/v1/items/7?token=secret", nil)
	response := httptest.NewRecorder()
	Write(response, req, 400, "invalid request", []ValidationError{{Field: "name", Code: "required", Message: "is required"}})

	if got := response.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q", got)
	}
	var body Details
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Instance != "/v1/items/7" || strings.Contains(body.Instance, "secret") {
		t.Fatalf("Instance = %q", body.Instance)
	}
	if body.Status != 400 || len(body.Errors) != 1 || body.Errors[0].Field != "name" {
		t.Fatalf("unexpected details: %+v", body)
	}
}

func TestWriteErrorMapsKnownCause(t *testing.T) {
	t.Parallel()
	response := httptest.NewRecorder()
	WriteError(response, httptest.NewRequest("GET", "/private", nil), nil, errors.Join(errors.New("wrapped"), ErrUnauthorized))
	if response.Code != 401 || response.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("status=%d challenge=%q", response.Code, response.Header().Get("WWW-Authenticate"))
	}
	if strings.Contains(response.Body.String(), "wrapped") {
		t.Fatalf("cause leaked: %s", response.Body.String())
	}
}

func TestInternalRedactsAndLogsOnce(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	cause := errors.New("password=internal-secret")
	response := httptest.NewRecorder()
	Internal(response, httptest.NewRequest("POST", "/v1/items?access_token=secret", nil), logger, cause)

	if response.Code != 500 || strings.Contains(response.Body.String(), "internal-secret") || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("unexpected public response: %s", response.Body.String())
	}
	if got := strings.Count(logs.String(), "internal server error"); got != 1 {
		t.Fatalf("log count = %d; logs=%q", got, logs.String())
	}
	if !strings.Contains(logs.String(), "password=internal-secret") {
		t.Fatalf("internal cause missing from server log: %q", logs.String())
	}
}
