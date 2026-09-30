// Package problem writes stable RFC 7807 responses for HTTP APIs.
package problem

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

const mediaType = "application/problem+json"

// Details is an RFC 7807 problem response.
type Details struct {
	Type     string            `json:"type"`
	Title    string            `json:"title"`
	Status   int               `json:"status"`
	Detail   string            `json:"detail"`
	Instance string            `json:"instance"`
	Errors   []ValidationError `json:"errors,omitempty"`
}

// ValidationError describes one invalid request field.
type ValidationError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

var (
	// ErrBadRequest marks a malformed or otherwise invalid request.
	ErrBadRequest = errors.New("bad request")
	// ErrUnauthorized marks missing or invalid authentication.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrForbidden marks an authenticated request without required access.
	ErrForbidden = errors.New("forbidden")
	// ErrNotFound marks a missing resource.
	ErrNotFound = errors.New("not found")
	// ErrConflict marks a request conflicting with current resource state.
	ErrConflict = errors.New("conflict")
)

// Write writes a problem response. Instance includes URL path only, never query values.
func Write(w http.ResponseWriter, r *http.Request, status int, detail string, entries []ValidationError) {
	if status < 400 || status > 599 {
		status = http.StatusInternalServerError
		detail = "an unexpected error occurred"
		entries = nil
	}
	if detail == "" {
		detail = http.StatusText(status)
	}
	if entries != nil && len(entries) == 0 {
		entries = nil
	}
	instance := ""
	if r != nil && r.URL != nil {
		instance = r.URL.Path
	}
	body := Details{
		Type:     "about:blank",
		Title:    http.StatusText(status),
		Status:   status,
		Detail:   detail,
		Instance: instance,
		Errors:   entries,
	}
	w.Header().Set("Content-Type", mediaType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// WriteError maps known application errors to stable public responses and
// logs unexpected errors exactly once through Internal.
func WriteError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, ErrBadRequest):
		Write(w, r, http.StatusBadRequest, "the request is invalid", nil)
	case errors.Is(err, ErrUnauthorized):
		w.Header().Set("WWW-Authenticate", "Bearer")
		Write(w, r, http.StatusUnauthorized, "authentication is required", nil)
	case errors.Is(err, ErrForbidden):
		Write(w, r, http.StatusForbidden, "you are not allowed to perform this action", nil)
	case errors.Is(err, ErrNotFound):
		Write(w, r, http.StatusNotFound, "the requested resource was not found", nil)
	case errors.Is(err, ErrConflict):
		Write(w, r, http.StatusConflict, "the request conflicts with the current resource state", nil)
	default:
		Internal(w, r, logger, err)
	}
}

// Internal logs the unexpected cause once and returns no internal details.
func Internal(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	if logger != nil {
		logger.Error("internal server error", "error", err)
	}
	Write(w, r, http.StatusInternalServerError, "an unexpected error occurred", nil)
}
