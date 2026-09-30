package httpjson

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type requestBody struct {
	Name string `json:"name"`
}

func TestDecodeRejectsInvalidRequests(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		contentType string
		body        string
		limit       int64
		wantStatus  int
	}{
		{name: "malformed", contentType: "application/json", body: `{"name":`, wantStatus: http.StatusBadRequest},
		{name: "wrong content type", contentType: "text/plain", body: `{"name":"x"}`, wantStatus: http.StatusUnsupportedMediaType},
		{name: "extra value", contentType: "application/json", body: `{"name":"x"} {"name":"y"}`, wantStatus: http.StatusBadRequest},
		{name: "unknown field", contentType: "application/json", body: `{"name":"x","admin":true}`, wantStatus: http.StatusBadRequest},
		{name: "oversized", contentType: "application/json", body: `{"name":"too big"}`, limit: 4, wantStatus: http.StatusRequestEntityTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/", strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			if test.limit > 0 {
				request.Body = http.MaxBytesReader(httptest.NewRecorder(), request.Body, test.limit)
			}
			var decoded requestBody
			err := Decode(httptest.NewRecorder(), request, &decoded)
			var decodeError *DecodeError
			if !errors.As(err, &decodeError) || decodeError.Status != test.wantStatus {
				t.Fatalf("Decode error = %#v, want status %d", err, test.wantStatus)
			}
		})
	}
}

func TestTimestampUsesUTCAndRFC3339(t *testing.T) {
	t.Parallel()
	zone := time.FixedZone("offset", 2*60*60)
	got := Timestamp(time.Date(2025, 1, 2, 3, 4, 5, 0, zone))
	if got != "2025-01-02T01:04:05Z" {
		t.Fatalf("Timestamp() = %q", got)
	}
}
func TestEncodeNormalizesTimeToUTC(t *testing.T) {
	t.Parallel()
	response := httptest.NewRecorder()
	localTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.FixedZone("offset", 2*60*60))
	if err := Encode(response, http.StatusOK, struct {
		At time.Time `json:"at"`
	}{At: localTime}); err != nil {
		t.Fatal(err)
	}
	if got := response.Body.String(); !strings.Contains(got, `"at":"2025-01-02T01:04:05Z"`) {
		t.Fatalf("encoded response = %s", got)
	}
}
