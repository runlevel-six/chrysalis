// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"

	"github.com/runlevel-six/chrysalis/internal/auth"
	"github.com/runlevel-six/chrysalis/internal/service"
)

func (s *server) registerContent(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/boards/{id}/detail", s.boardDetail)
	mux.HandleFunc("GET /api/v1/boards/{id}/trash", s.boardTrash)
	mux.HandleFunc("PUT /api/v1/boards/{id}/view", s.setBoardView)
	mux.HandleFunc("POST /api/v1/boards/{id}/columns", s.createColumn)
	mux.HandleFunc("PATCH /api/v1/columns/{id}", s.updateColumn)
	mux.HandleFunc("DELETE /api/v1/columns/{id}", s.trashColumn(true))
	mux.HandleFunc("POST /api/v1/columns/{id}/restore", s.trashColumn(false))
	mux.HandleFunc("POST /api/v1/columns/{id}/cards", s.createCard)
	mux.HandleFunc("PATCH /api/v1/cards/{id}", s.updateCard)
	mux.HandleFunc("DELETE /api/v1/cards/{id}", s.trashCard(true))
	mux.HandleFunc("POST /api/v1/cards/{id}/restore", s.trashCard(false))
}

func userID(r *http.Request) string {
	u, _ := auth.UserFrom(r.Context())
	return u.ID
}

func (s *server) boardDetail(w http.ResponseWriter, r *http.Request) {
	d, err := s.boards.Detail(r.Context(), userID(r), r.PathValue("id"))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *server) boardTrash(w http.ResponseWriter, r *http.Request) {
	t, err := s.boards.Trash(r.Context(), userID(r), r.PathValue("id"))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *server) setBoardView(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cards   map[string]bool `json:"cards"`
		Columns map[string]bool `json:"columns"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.boards.SetView(r.Context(), userID(r), r.PathValue("id"), req.Cards, req.Columns); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type createReq struct {
	Name     string  `json:"name"`
	Title    string  `json:"title"`
	Position *string `json:"position"`
}

func (s *server) createColumn(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if !decodeJSON(w, r, &req) {
		return
	}
	c, err := s.boards.CreateColumn(r.Context(), userID(r), r.PathValue("id"), req.Name, req.Position)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *server) updateColumn(w http.ResponseWriter, r *http.Request) {
	var p service.ColumnPatch
	if !decodeJSON(w, r, &p) {
		return
	}
	c, err := s.boards.UpdateColumn(r.Context(), userID(r), r.PathValue("id"), p)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *server) trashColumn(trashed bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := s.boards.SetColumnTrashed(r.Context(), userID(r), r.PathValue("id"), trashed)
		if err != nil {
			s.serviceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
	}
}

func (s *server) createCard(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if !decodeJSON(w, r, &req) {
		return
	}
	c, err := s.boards.CreateCard(r.Context(), userID(r), r.PathValue("id"), req.Title, req.Position)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *server) updateCard(w http.ResponseWriter, r *http.Request) {
	var p service.CardPatch
	if !decodeJSON(w, r, &p) {
		return
	}
	c, err := s.boards.UpdateCard(r.Context(), userID(r), r.PathValue("id"), p)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *server) trashCard(trashed bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := s.boards.SetCardTrashed(r.Context(), userID(r), r.PathValue("id"), trashed)
		if err != nil {
			s.serviceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
	}
}
