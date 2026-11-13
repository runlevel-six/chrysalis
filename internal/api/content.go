// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/runlevel-six/chrysalis/internal/auth"
	"github.com/runlevel-six/chrysalis/internal/service"
)

func (s *server) registerContent(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/boards/{id}/detail", s.boardDetail)
	mux.HandleFunc("DELETE /api/v1/boards/{id}", s.trashBoard(true))
	mux.HandleFunc("POST /api/v1/boards/{id}/restore", s.trashBoard(false))
	mux.HandleFunc("GET /api/v1/me/boards/archived", s.listBoardsWith(s.boards.ListArchived))
	mux.HandleFunc("GET /api/v1/me/boards/trash", s.listBoardsWith(s.boards.ListTrashed))
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
	mux.HandleFunc("PATCH /api/v1/boards/{id}", s.updateBoard)
	mux.HandleFunc("GET /api/v1/boards/{id}/archived", s.archivedCards)
	mux.HandleFunc("POST /api/v1/boards/{id}/labels", s.createLabel)
	mux.HandleFunc("PATCH /api/v1/labels/{id}", s.updateLabel)
	mux.HandleFunc("DELETE /api/v1/labels/{id}", s.trashLabel(true))
	mux.HandleFunc("POST /api/v1/labels/{id}/restore", s.trashLabel(false))
	mux.HandleFunc("GET /api/v1/cards/{id}/description", s.getDescription)
	mux.HandleFunc("PUT /api/v1/cards/{id}/description", s.setDescription)
	mux.HandleFunc("GET /api/v1/cards/{id}/checklists", s.listChecklists)
	mux.HandleFunc("POST /api/v1/cards/{id}/checklists", s.createChecklist)
	mux.HandleFunc("PATCH /api/v1/checklists/{id}", s.updateChecklist)
	mux.HandleFunc("DELETE /api/v1/checklists/{id}", s.trashChecklist(true))
	mux.HandleFunc("POST /api/v1/checklists/{id}/restore", s.trashChecklist(false))
	mux.HandleFunc("POST /api/v1/checklists/{id}/items", s.createChecklistItem)
	mux.HandleFunc("PATCH /api/v1/checklist-items/{id}", s.updateChecklistItem)
	mux.HandleFunc("DELETE /api/v1/checklist-items/{id}", s.trashChecklistItem(true))
	mux.HandleFunc("POST /api/v1/checklist-items/{id}/restore", s.trashChecklistItem(false))
}

func (s *server) listChecklists(w http.ResponseWriter, r *http.Request) {
	lists, err := s.boards.Checklists(r.Context(), userID(r), r.PathValue("id"))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"checklists": lists})
}

func (s *server) createChecklist(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if !decodeJSON(w, r, &req) {
		return
	}
	cl, err := s.boards.CreateChecklist(r.Context(), userID(r), r.PathValue("id"), req.Title)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, cl)
}

func (s *server) updateChecklist(w http.ResponseWriter, r *http.Request) {
	var p service.ChecklistPatch
	if !decodeJSON(w, r, &p) {
		return
	}
	cl, err := s.boards.UpdateChecklist(r.Context(), userID(r), r.PathValue("id"), p)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cl)
}

func (s *server) trashChecklist(trashed bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, err := s.boards.SetChecklistTrashed(r.Context(), userID(r), r.PathValue("id"), trashed)
		if err != nil {
			s.serviceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, cl)
	}
}

func (s *server) createChecklistItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text     string  `json:"text"`
		Position *string `json:"position"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	it, err := s.boards.CreateChecklistItem(r.Context(), userID(r), r.PathValue("id"), req.Text, req.Position)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, it)
}

func (s *server) updateChecklistItem(w http.ResponseWriter, r *http.Request) {
	var p service.ChecklistItemPatch
	if !decodeJSON(w, r, &p) {
		return
	}
	it, err := s.boards.UpdateChecklistItem(r.Context(), userID(r), r.PathValue("id"), p)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (s *server) trashChecklistItem(trashed bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		it, err := s.boards.SetChecklistItemTrashed(r.Context(), userID(r), r.PathValue("id"), trashed)
		if err != nil {
			s.serviceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, it)
	}
}

func (s *server) updateBoard(w http.ResponseWriter, r *http.Request) {
	var p service.BoardPatch
	if !decodeJSON(w, r, &p) {
		return
	}
	b, err := s.boards.UpdateBoard(r.Context(), userID(r), r.PathValue("id"), p)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *server) archivedCards(w http.ResponseWriter, r *http.Request) {
	cards, err := s.boards.Archived(r.Context(), userID(r), r.PathValue("id"))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cards": cards})
}

func (s *server) createLabel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	l, err := s.boards.CreateLabel(r.Context(), userID(r), r.PathValue("id"), req.Name, req.Color)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (s *server) updateLabel(w http.ResponseWriter, r *http.Request) {
	var p service.LabelPatch
	if !decodeJSON(w, r, &p) {
		return
	}
	l, err := s.boards.UpdateLabel(r.Context(), userID(r), r.PathValue("id"), p)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *server) trashLabel(trashed bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l, err := s.boards.SetLabelTrashed(r.Context(), userID(r), r.PathValue("id"), trashed)
		if err != nil {
			s.serviceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, l)
	}
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
	var req service.ViewChange
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.boards.SetView(r.Context(), userID(r), r.PathValue("id"), req); err != nil {
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

func (s *server) getDescription(w http.ResponseWriter, r *http.Request) {
	d, err := s.boards.Description(r.Context(), userID(r), r.PathValue("id"))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// setDescription saves a card description with an optimistic version check:
// 409 with the current description if someone else saved first (§5.5).
func (s *server) setDescription(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Doc         json.RawMessage `json:"doc"`
		BaseVersion int64           `json:"baseVersion"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	d, err := s.boards.SetDescription(r.Context(), userID(r), r.PathValue("id"), req.Doc, req.BaseVersion)
	var conflict *service.ConflictError
	if errors.As(err, &conflict) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": conflict.Error(), "current": conflict.Current})
		return
	}
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *server) trashBoard(trashed bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := s.boards.SetBoardTrashed(r.Context(), userID(r), r.PathValue("id"), trashed)
		if err != nil {
			s.serviceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, b)
	}
}

func (s *server) listBoardsWith(list func(context.Context, string) ([]service.Board, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := list(r.Context(), userID(r))
		if err != nil {
			s.serviceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"boards": b})
	}
}
