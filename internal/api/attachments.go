// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/runlevel-six/chrysalis/internal/auth"
)

func (s *server) registerAttachments(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/cards/{id}/attachments", s.listAttachments)
	mux.HandleFunc("POST /api/v1/cards/{id}/attachments", s.uploadAttachment)
	mux.HandleFunc("POST /api/v1/cards/{id}/links", s.addLink)
	mux.HandleFunc("POST /api/v1/boards/{id}/background", s.uploadBackground)
	mux.HandleFunc("GET /api/v1/attachments/{id}", s.downloadAttachment)
	mux.HandleFunc("DELETE /api/v1/attachments/{id}", s.trashAttachment(true))
	mux.HandleFunc("POST /api/v1/attachments/{id}/restore", s.trashAttachment(false))
}

func (s *server) listAttachments(w http.ResponseWriter, r *http.Request) {
	list, err := s.attachments.List(r.Context(), userID(r), r.PathValue("id"))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"attachments": list})
}

// uploadAttachment takes one file in a multipart form field named "file".
func (s *server) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	s.receiveFile(w, r, func(name string, data []byte) (any, error) {
		return s.attachments.Upload(r.Context(), userID(r), r.PathValue("id"), name, data)
	})
}

// uploadBackground takes an image for the board's background.
func (s *server) uploadBackground(w http.ResponseWriter, r *http.Request) {
	s.receiveFile(w, r, func(name string, data []byte) (any, error) {
		return s.attachments.UploadBackground(r.Context(), userID(r), r.PathValue("id"), name, data)
	})
}

// receiveFile reads the multipart field "file" (within the upload limit) and
// hands it to store, answering 201 with what store returns.
func (s *server) receiveFile(w http.ResponseWriter, r *http.Request, store func(name string, data []byte) (any, error)) {
	limit := s.cfg.MaxUploadBytes
	// Room for the multipart framing around the file itself.
	r.Body = http.MaxBytesReader(w, r.Body, limit+64<<10)
	mr, err := r.MultipartReader()
	if err != nil {
		auth.WriteError(w, http.StatusBadRequest, "send the file as multipart form data")
		return
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			auth.WriteError(w, http.StatusBadRequest, "no file in the upload")
			return
		}
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			tooLarge(w, limit)
			return
		}
		if err != nil {
			auth.WriteError(w, http.StatusBadRequest, "couldn't read the upload")
			return
		}
		if part.FormName() != "file" {
			continue
		}
		// Read one byte past the limit to tell "exactly at" from "over".
		data, err := io.ReadAll(io.LimitReader(part, limit+1))
		if errors.As(err, &tooBig) || int64(len(data)) > limit {
			tooLarge(w, limit)
			return
		}
		if err != nil {
			auth.WriteError(w, http.StatusBadRequest, "couldn't read the upload")
			return
		}
		out, err := store(part.FileName(), data)
		if err != nil {
			s.serviceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
		return
	}
}

func tooLarge(w http.ResponseWriter, limit int64) {
	auth.WriteError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("That file is too big to attach (the limit is %d MB).", limit>>20))
}

func (s *server) addLink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	att, err := s.attachments.AddLink(r.Context(), userID(r), r.PathValue("id"), req.URL, req.Title)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, att)
}

// inlineTypes are the only types a browser may render in place. Everything
// else is sent as a download, so an uploaded HTML or SVG file can never run
// in the app's origin.
var inlineTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true}

// downloadAttachment serves an attachment (?thumb=1 for its thumbnail), only
// to members of its board, never from a public URL (§7.3).
func (s *server) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	att, f, err := s.attachments.Open(r.Context(), userID(r), r.PathValue("id"), r.URL.Query().Get("thumb") == "1")
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	defer func() { _ = f.Close() }()

	h := w.Header()
	ctype := att.MIME
	disposition := "attachment"
	if inlineTypes[ctype] {
		disposition = "inline"
	} else {
		ctype = "application/octet-stream"
	}
	h.Set("Content-Type", ctype)
	h.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": att.Filename}))
	h.Set("X-Content-Type-Options", "nosniff")
	// Belt and braces: even if a browser rendered this response, it gets no
	// scripts, no plugins, and a unique origin.
	h.Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; sandbox")
	// An attachment's bytes never change for its ID; private, because it's
	// only for signed-in board members.
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeContent(w, r, "", time.Time{}, f)
}

func (s *server) trashAttachment(trashed bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		att, err := s.attachments.SetTrashed(r.Context(), userID(r), r.PathValue("id"), trashed)
		if err != nil {
			s.serviceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, att)
	}
}
