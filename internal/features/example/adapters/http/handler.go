// Package http is the inbound HTTP adapter for examples: request decoding,
// response DTOs, route registration, and the only place where example errors
// become problem responses.
package http

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/svaan1/go-api-boilerplate/internal/features/example/application"
	"github.com/svaan1/go-api-boilerplate/internal/features/example/domain"
	"github.com/svaan1/go-api-boilerplate/internal/platform/auth"
	"github.com/svaan1/go-api-boilerplate/internal/platform/httpx"
)

// Handler serves example endpoints.
type Handler struct {
	create *application.CreateExample
	rename *application.RenameExample
	logger *slog.Logger
}

// NewHandler wires the HTTP adapter to its use cases.
func NewHandler(create *application.CreateExample, rename *application.RenameExample, logger *slog.Logger) *Handler {
	return &Handler{create: create, rename: rename, logger: logger}
}

// RegisterRoutes installs authenticated example routes on mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("POST /v1/examples", auth.RequireAuthenticated(http.HandlerFunc(h.handleCreate)))
	mux.Handle("PATCH /v1/examples/{id}", auth.RequireAuthenticated(http.HandlerFunc(h.handleRename)))
}

type createRequest struct {
	Name string `json:"name"`
}

type renameRequest struct {
	Name string `json:"name"`
}

type exampleResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toResponse(example domain.Example) exampleResponse {
	return exampleResponse{
		ID:        string(example.ID()),
		Name:      example.Name().String(),
		CreatedAt: example.CreatedAt(),
		UpdatedAt: example.UpdatedAt(),
	}
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var request createRequest
	if err := httpx.DecodeJSON(w, r, &request); err != nil {
		h.writeError(w, r, err)
		return
	}
	example, err := h.create.Execute(r.Context(), application.CreateExampleInput{Name: request.Name})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	_ = httpx.EncodeJSON(w, http.StatusCreated, toResponse(example))
}

func (h *Handler) handleRename(w http.ResponseWriter, r *http.Request) {
	var request renameRequest
	if err := httpx.DecodeJSON(w, r, &request); err != nil {
		h.writeError(w, r, err)
		return
	}
	example, err := h.rename.Execute(r.Context(), application.RenameExampleInput{
		ID: r.PathValue("id"), Name: request.Name,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	_ = httpx.EncodeJSON(w, http.StatusOK, toResponse(example))
}

// writeError maps decode and domain errors to problem responses. Anything
// unrecognized is logged once and returned as a generic 500.
func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var decodeErr *httpx.DecodeError
	switch {
	case errors.As(err, &decodeErr):
		httpx.WriteProblem(w, r, decodeErr.Status, decodeErr.Message, nil)
	case errors.Is(err, domain.ErrInvalidName):
		httpx.WriteProblem(w, r, http.StatusUnprocessableEntity, "the request contains invalid fields",
			[]httpx.ValidationError{{Field: "name", Code: "invalid_value", Message: domain.ErrInvalidName.Error()}})
	case errors.Is(err, domain.ErrNotFound):
		httpx.WriteError(w, r, h.logger, httpx.ErrNotFound)
	case errors.Is(err, domain.ErrDuplicateName):
		httpx.WriteProblem(w, r, http.StatusConflict, domain.ErrDuplicateName.Error(), nil)
	default:
		httpx.WriteInternal(w, r, h.logger, err)
	}
}
