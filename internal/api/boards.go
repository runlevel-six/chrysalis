// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/runlevel-six/chrysalis/internal/auth"
	"github.com/runlevel-six/chrysalis/internal/service"
)

func (s *server) listBoards(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	boards, err := s.boards.List(r.Context(), u.ID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"boards": boards})
}

func (s *server) createBoard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := auth.UserFrom(r.Context())
	b, err := s.boards.Create(r.Context(), u.ID, req.Name)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/boards/"+b.ID)
	writeJSON(w, http.StatusCreated, b)
}

func (s *server) getBoard(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	b, err := s.boards.Get(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

const maxJSONBody = 1 << 20 // 1 MiB; uploads have their own endpoint and limit

// decodeJSON reads a JSON request body into v. On failure it writes a 400
// and returns false.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		auth.WriteError(w, http.StatusBadRequest, "request body must be valid JSON")
		return false
	}
	return true
}

// serviceError maps service errors to HTTP responses.
func (s *server) serviceError(w http.ResponseWriter, r *http.Request, err error) {
	var verr *service.ValidationError
	switch {
	case errors.As(err, &verr):
		auth.WriteError(w, http.StatusBadRequest, verr.Msg)
	case errors.Is(err, service.ErrNotFound):
		auth.WriteError(w, http.StatusNotFound, "not found")
	default:
		s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		auth.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}
