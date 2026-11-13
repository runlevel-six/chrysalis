// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"net/http"

	"github.com/runlevel-six/chrysalis/internal/auth"
	"github.com/runlevel-six/chrysalis/internal/trello"
)

func (s *server) registerImports(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/import/trello", s.importTrello)
	mux.HandleFunc("GET /api/v1/cards/{id}/comments", s.listComments)
}

// importTrello takes a Trello board export (multipart field "file") and
// creates a new board from it, owned by the signed-in user (docs/DESIGN.md
// §8). It answers with the new board's ID and a report of what came across.
func (s *server) importTrello(w http.ResponseWriter, r *http.Request) {
	tooLarge := func() {
		auth.WriteError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("That file is too big to import (the limit is %d MB).", trello.MaxExportBytes>>20))
	}
	s.receiveFileWithin(w, r, trello.MaxExportBytes, tooLarge, func(_ string, data []byte) (any, error) {
		plan, report, err := trello.Parse(data)
		if err != nil {
			return nil, err
		}
		board, counts, err := s.boards.Import(r.Context(), userID(r), plan)
		if err != nil {
			return nil, err
		}
		report.Imported = counts
		return map[string]any{"boardId": board.ID, "board": board, "report": report}, nil
	})
}

func (s *server) listComments(w http.ResponseWriter, r *http.Request) {
	list, err := s.boards.Comments(r.Context(), userID(r), r.PathValue("id"))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": list})
}
