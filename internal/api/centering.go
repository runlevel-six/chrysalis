// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/runlevel-six/chrysalis/internal/auth"
	"github.com/runlevel-six/chrysalis/internal/service"
)

// The centering line (docs/DESIGN.md §3.13). The person's own settings and
// line live under /me/centering; collections and entries under /centering,
// where every request checks collection membership.
func (s *server) registerCentering(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/me/centering", s.centeringSettings)
	mux.HandleFunc("PATCH /api/v1/me/centering", s.updateCenteringSettings)
	mux.HandleFunc("GET /api/v1/me/centering/line", s.centeringLine)
	mux.HandleFunc("POST /api/v1/me/centering/line/next", s.centeringStep(1))
	mux.HandleFunc("POST /api/v1/me/centering/line/previous", s.centeringStep(-1))
	mux.HandleFunc("POST /api/v1/me/centering/line/pin", s.centeringPin)
	mux.HandleFunc("POST /api/v1/me/centering/line/unpin", s.centeringAction(s.centering.Unpin))
	mux.HandleFunc("POST /api/v1/me/centering/line/hide", s.centeringHide)
	mux.HandleFunc("POST /api/v1/me/centering/line/show", s.centeringAction(s.centering.Unhide))

	mux.HandleFunc("GET /api/v1/centering/collections", s.listQuoteCollections)
	mux.HandleFunc("POST /api/v1/centering/collections", s.createQuoteCollection)
	mux.HandleFunc("PATCH /api/v1/centering/collections/{id}", s.updateQuoteCollection)
	mux.HandleFunc("DELETE /api/v1/centering/collections/{id}", s.trashQuoteCollection(true))
	mux.HandleFunc("POST /api/v1/centering/collections/{id}/restore", s.trashQuoteCollection(false))
	mux.HandleFunc("GET /api/v1/centering/collections/{id}/entries", s.listQuotes)
	mux.HandleFunc("POST /api/v1/centering/collections/{id}/entries", s.createQuote)
	mux.HandleFunc("GET /api/v1/centering/collections/{id}/export", s.exportQuotes)
	mux.HandleFunc("PATCH /api/v1/centering/entries/{id}", s.updateQuote)
	mux.HandleFunc("DELETE /api/v1/centering/entries/{id}", s.trashQuote(true))
	mux.HandleFunc("POST /api/v1/centering/entries/{id}/restore", s.trashQuote(false))
	mux.HandleFunc("GET /api/v1/centering/trash", s.centeringTrash)
	mux.HandleFunc("POST /api/v1/centering/import", s.importQuotes)
}

func (s *server) centeringSettings(w http.ResponseWriter, r *http.Request) {
	out, err := s.centering.Settings(r.Context(), userID(r))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) updateCenteringSettings(w http.ResponseWriter, r *http.Request) {
	var p service.CenteringSettingsPatch
	if !decodeJSON(w, r, &p) {
		return
	}
	out, err := s.centering.UpdateSettings(r.Context(), userID(r), p)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// centeringLine answers with what the line shows now. ?tz= is the client's
// IANA time zone (where "midnight" is); ?load= (or else the
// X-Chrysalis-Client tab id) identifies the page load.
func (s *server) centeringLine(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	load := q.Get("load")
	if load == "" {
		load = r.Header.Get("X-Chrysalis-Client") // the tab, which is one page load
	}
	if len(load) > 100 {
		load = load[:100]
	}
	out, err := s.centering.Current(r.Context(), userID(r), q.Get("tz"), load)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	// The line can change between requests; don't let a cache answer.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

func (s *server) writeLine(w http.ResponseWriter, r *http.Request, l service.CenteringLine, err error) {
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *server) centeringStep(dir int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l, err := s.centering.Step(r.Context(), userID(r), dir)
		s.writeLine(w, r, l, err)
	}
}

func (s *server) centeringAction(fn func(ctx context.Context, userID string) (service.CenteringLine, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l, err := fn(r.Context(), userID(r))
		s.writeLine(w, r, l, err)
	}
}

func (s *server) centeringPin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EntryID string `json:"entryId"`
	}
	if r.ContentLength != 0 && !decodeJSON(w, r, &req) {
		return
	}
	l, err := s.centering.Pin(r.Context(), userID(r), req.EntryID)
	s.writeLine(w, r, l, err)
}

func (s *server) centeringHide(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TZ string `json:"tz"`
	}
	if r.ContentLength != 0 && !decodeJSON(w, r, &req) {
		return
	}
	l, err := s.centering.HideForToday(r.Context(), userID(r), req.TZ)
	s.writeLine(w, r, l, err)
}

func (s *server) listQuoteCollections(w http.ResponseWriter, r *http.Request) {
	list, err := s.centering.Collections(r.Context(), userID(r))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"collections": list})
}

func (s *server) createQuoteCollection(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if !decodeJSON(w, r, &req) {
		return
	}
	c, err := s.centering.CreateCollection(r.Context(), userID(r), req.Name)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *server) updateQuoteCollection(w http.ResponseWriter, r *http.Request) {
	var p service.CollectionPatch
	if !decodeJSON(w, r, &p) {
		return
	}
	c, err := s.centering.UpdateCollection(r.Context(), userID(r), r.PathValue("id"), p)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *server) trashQuoteCollection(trashed bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := s.centering.SetCollectionTrashed(r.Context(), userID(r), r.PathValue("id"), trashed)
		if err != nil {
			s.serviceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
	}
}

func (s *server) listQuotes(w http.ResponseWriter, r *http.Request) {
	list, err := s.centering.Entries(r.Context(), userID(r), r.PathValue("id"))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": list})
}

func (s *server) createQuote(w http.ResponseWriter, r *http.Request) {
	var in service.EntryInput
	if !decodeJSON(w, r, &in) {
		return
	}
	e, err := s.centering.CreateEntry(r.Context(), userID(r), r.PathValue("id"), in)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *server) updateQuote(w http.ResponseWriter, r *http.Request) {
	var p service.EntryPatch
	if !decodeJSON(w, r, &p) {
		return
	}
	e, err := s.centering.UpdateEntry(r.Context(), userID(r), r.PathValue("id"), p)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *server) trashQuote(trashed bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := s.centering.SetEntryTrashed(r.Context(), userID(r), r.PathValue("id"), trashed)
		if err != nil {
			s.serviceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, e)
	}
}

func (s *server) centeringTrash(w http.ResponseWriter, r *http.Request) {
	t, err := s.centering.Trash(r.Context(), userID(r))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// exportQuotes downloads a collection as YAML (the default) or JSON
// (?format=json).
func (s *server) exportQuotes(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	data, name, err := s.centering.Export(r.Context(), userID(r), r.PathValue("id"), format)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	h := w.Header()
	if strings.HasSuffix(name, ".json") {
		h.Set("Content-Type", "application/json; charset=utf-8")
	} else {
		h.Set("Content-Type", "application/yaml; charset=utf-8")
	}
	h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	_, _ = w.Write(data) // #nosec G705 -- a download (attachment, nosniff), never rendered as a page
}

// importQuotes adds entries from a YAML or JSON file: uploaded as the
// multipart field "file", or sent as the request body. The query says where
// they go: ?collectionId= (an existing collection), or ?collection= (one of
// the person's own by name, created if needed; defaults to the name in the
// file). ?replace=1 moves the collection's current entries to the trash
// first; ?format=json|yaml overrides guessing from the content.
func (s *server) importQuotes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	target := service.ImportTarget{CollectionID: q.Get("collectionId"), Name: q.Get("collection")}
	replace := q.Get("replace") == "1" || q.Get("replace") == "true"
	format := q.Get("format")
	run := func(name string, data []byte) (any, error) {
		f := format
		if f == "" {
			switch {
			case strings.HasSuffix(strings.ToLower(name), ".json"):
				f = "json"
			case strings.HasSuffix(strings.ToLower(name), ".yaml"), strings.HasSuffix(strings.ToLower(name), ".yml"):
				f = "yaml"
			}
		}
		file, err := service.ParseQuoteFile(data, f)
		if err != nil {
			return nil, err
		}
		return s.centering.Import(r.Context(), userID(r), target, file, replace)
	}
	tooLarge := func() {
		auth.WriteError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("That file is too big to import (the limit is %d MB).", service.MaxQuoteFileBytes>>20))
	}
	ctype, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if ctype == "multipart/form-data" {
		s.receiveFileWithin(w, r, service.MaxQuoteFileBytes, tooLarge, run)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, service.MaxQuoteFileBytes)
	data, err := io.ReadAll(r.Body)
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		tooLarge()
		return
	}
	if err != nil {
		auth.WriteError(w, http.StatusBadRequest, "couldn't read the upload")
		return
	}
	name := "" // tell YAML from JSON by the content
	if ctype == "application/json" {
		name = "body.json"
	}
	out, err := run(name, data)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}
