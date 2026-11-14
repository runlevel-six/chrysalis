// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"

	"github.com/runlevel-six/chrysalis/internal/service"
)

// copyCard copies a card: a plain copy, a new template, or a new card from a
// template (docs/DESIGN.md §4, Phase 2).
func (s *server) copyCard(w http.ResponseWriter, r *http.Request) {
	var opt service.CopyOptions
	if !decodeJSON(w, r, &opt) {
		return
	}
	c, err := s.attachments.CopyCard(r.Context(), userID(r), r.PathValue("id"), opt)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}
