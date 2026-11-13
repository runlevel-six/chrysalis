// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/runlevel-six/chrysalis/internal/service"
)

func (m *multiUser) uploadBackground(user, boardID, name string, data []byte) *httptest.ResponseRecorder {
	m.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	_ = mw.Close()
	r := httptest.NewRequest("POST", "/api/v1/boards/"+boardID+"/background", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Remote-User", user)
	w := httptest.NewRecorder()
	m.h.ServeHTTP(w, r)
	return w
}

func widePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestBoardBackgrounds(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Board")
	patch := func(body string) *httptest.ResponseRecorder {
		return m.as("alice", "PATCH", "/api/v1/boards/"+b.ID, body)
	}

	w := patch(`{"background":{"type":"color","value":"#a9cf62"}}`)
	if got := decode[service.Board](t, w).Background; w.Code != http.StatusOK || got.Type != "color" || got.Value != "#a9cf62" {
		t.Fatalf("color: %d %+v", w.Code, got)
	}
	if got := decode[service.Board](t, patch(`{"background":{"type":"gradient","value":"meadow"}}`)).Background; got.Value != "meadow" {
		t.Fatalf("gradient: %+v", got)
	}
	for _, bad := range []string{
		`{"background":{"type":"color","value":"red"}}`,
		`{"background":{"type":"gradient","value":"plaid"}}`,
		`{"background":{"type":"video"}}`,
	} {
		if w := patch(bad); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}

	// A big photo is scaled to 2560 on its longest edge.
	w = m.uploadBackground("alice", b.ID, "meadow.png", widePNG(t, 3000, 300))
	img := decode[service.Attachment](t, w)
	if w.Code != http.StatusCreated || img.Width != 2560 || img.Height != 256 {
		t.Fatalf("upload: %d %+v", w.Code, img)
	}
	if got := decode[service.Board](t, patch(`{"background":{"type":"image","value":"`+img.ID+`"}}`)).Background; got.Type != "image" {
		t.Fatalf("image: %+v", got)
	}
	// Replacing it sends the old image to the trash (purged later).
	patch(`{"background":{"type":"color","value":""}}`)
	if row, err := m.st.GetAttachment(t.Context(), img.ID); err != nil || !row.DeletedAt.Valid {
		t.Errorf("old background not trashed: %+v %v", row.DeletedAt, err)
	}

	if w := m.uploadBackground("alice", b.ID, "notes.txt", []byte("not an image")); w.Code != http.StatusBadRequest {
		t.Errorf("non-image background: %d", w.Code)
	}
	other := m.createBoard("alice", "Other")
	otherImg := decode[service.Attachment](t, m.uploadBackground("alice", other.ID, "x.png", widePNG(t, 10, 10)))
	if w := patch(`{"background":{"type":"image","value":"` + otherImg.ID + `"}}`); w.Code != http.StatusBadRequest {
		t.Errorf("another board's image: %d", w.Code)
	}

	makeViewer(t, m, b.ID, "bob")
	if w := m.uploadBackground("bob", b.ID, "x.png", widePNG(t, 10, 10)); w.Code != http.StatusForbidden {
		t.Errorf("viewer upload: %d", w.Code)
	}
	if w := m.uploadBackground("mallory", b.ID, "x.png", widePNG(t, 10, 10)); w.Code != http.StatusNotFound {
		t.Errorf("non-member upload: %d", w.Code)
	}
}
