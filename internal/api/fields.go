// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"

	"github.com/runlevel-six/chrysalis/internal/service"
)

func (s *server) registerFields(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/boards/{id}/fields", s.createField)
	mux.HandleFunc("PATCH /api/v1/fields/{id}", s.updateField)
	mux.HandleFunc("DELETE /api/v1/fields/{id}", s.trashField(true))
	mux.HandleFunc("POST /api/v1/fields/{id}/restore", s.trashField(false))
	mux.HandleFunc("PUT /api/v1/cards/{id}/fields/{fieldId}", s.setCardField)
	mux.HandleFunc("POST /api/v1/cards/{id}/copy", s.copyCard)
}

func (s *server) createField(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string                `json:"name"`
		Type    string                `json:"type"`
		Options []service.FieldOption `json:"options"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	f, err := s.boards.CreateField(r.Context(), userID(r), r.PathValue("id"), body.Name, body.Type, body.Options)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (s *server) updateField(w http.ResponseWriter, r *http.Request) {
	var p service.FieldPatch
	if !decodeJSON(w, r, &p) {
		return
	}
	f, err := s.boards.UpdateField(r.Context(), userID(r), r.PathValue("id"), p)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *server) trashField(trashed bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, err := s.boards.SetFieldTrashed(r.Context(), userID(r), r.PathValue("id"), trashed)
		if err != nil {
			s.serviceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, f)
	}
}

func (s *server) setCardField(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Value json.RawMessage `json:"value"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	c, err := s.boards.SetCardField(r.Context(), userID(r), r.PathValue("id"), r.PathValue("fieldId"), body.Value)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}
