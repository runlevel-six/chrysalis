// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/runlevel-six/chrysalis/internal/service"
)

// gpsPhoto is a real JPEG carrying an EXIF block with a GPS marker string
// and orientation 6 (needs a 90° turn to display upright).
func gpsPhoto(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for i := range img.Pix {
		img.Pix[i] = 180
	}
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	var tiff bytes.Buffer
	tiff.WriteString("MM\x00\x2a\x00\x00\x00\x08")
	_ = binary.Write(&tiff, binary.BigEndian, []uint16{1, 0x0112, 3, 0, 1, 6, 0})
	_ = binary.Write(&tiff, binary.BigEndian, uint32(0))
	tiff.WriteString("GPSLatitude 40.7128")
	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	seg = append(seg, payload...)
	raw := b.Bytes()
	return append(append(append([]byte{}, raw[:2]...), seg...), raw[2:]...)
}

func (m *multiUser) upload(user, cardID, filename string, data []byte) *httptest.ResponseRecorder {
	m.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", filename)
	_, _ = fw.Write(data)
	_ = mw.Close()
	r := httptest.NewRequest("POST", "/api/v1/cards/"+cardID+"/attachments", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Remote-User", user)
	w := httptest.NewRecorder()
	m.h.ServeHTTP(w, r)
	return w
}

func TestUploadImageStripsMetadata(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Board")
	col := m.column("alice", b.ID, "Col")
	card := m.card("alice", col.ID, "Card")

	w := m.upload("alice", card.ID, "../../IMG_0001.jpg", gpsPhoto(t))
	if w.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	att := decode[service.Attachment](t, w)
	if att.Kind != "image" || att.MIME != "image/jpeg" || !att.HasThumb || att.Filename != "IMG_0001.jpg" {
		t.Fatalf("attachment = %+v", att)
	}
	if att.Width != 20 || att.Height != 40 {
		t.Errorf("size = %dx%d, want 20x40 (rotated upright)", att.Width, att.Height)
	}

	for _, q := range []string{"", "?thumb=1"} {
		w = m.as("alice", "GET", "/api/v1/attachments/"+att.ID+q, "")
		if w.Code != http.StatusOK {
			t.Fatalf("download%s: %d", q, w.Code)
		}
		if bytes.Contains(w.Body.Bytes(), []byte("GPS")) || bytes.Contains(w.Body.Bytes(), []byte("Exif")) {
			t.Errorf("download%s still carries EXIF/GPS", q)
		}
		if ct := w.Header().Get("Content-Type"); ct != "image/jpeg" {
			t.Errorf("download%s content type = %q", q, ct)
		}
		if !strings.HasPrefix(w.Header().Get("Content-Disposition"), "inline") || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("download%s headers = %v", q, w.Header())
		}
	}

	// And nothing with GPS was written to disk at all.
	_ = filepath.Walk(filepath.Join(filepath.Dir(m.dbPath), "attachments"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if b, _ := os.ReadFile(p); bytes.Contains(b, []byte("GPS")) {
				t.Errorf("%s on disk still has GPS data", p)
			}
		}
		return nil
	})
}

func TestUploadFilesAreDownloadsNotPages(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Board")
	card := m.card("alice", m.column("alice", b.ID, "Col").ID, "Card")

	for name, data := range map[string]string{
		"evil.html": "<!doctype html><script>alert(document.cookie)</script>",
		"logo.svg":  `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		"notes.txt": "just some notes",
	} {
		w := m.upload("alice", card.ID, name, []byte(data))
		if w.Code != http.StatusCreated {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body)
		}
		att := decode[service.Attachment](t, w)
		if att.Kind != "file" || att.HasThumb {
			t.Errorf("%s: %+v", name, att)
		}
		w = m.as("alice", "GET", "/api/v1/attachments/"+att.ID, "")
		h := w.Header()
		if h.Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(h.Get("Content-Disposition"), "attachment") ||
			!strings.Contains(h.Get("Content-Security-Policy"), "sandbox") {
			t.Errorf("%s served as %v", name, h)
		}
	}
}

func TestUploadLimit(t *testing.T) {
	m := newMultiUserWith(t, testStatic, map[string]string{"CHRYSALIS_MAX_UPLOAD_MB": "1"})
	b := m.createBoard("alice", "Board")
	card := m.card("alice", m.column("alice", b.ID, "Col").ID, "Card")
	if w := m.upload("alice", card.ID, "ok.bin", bytes.Repeat([]byte("x"), 1<<20)); w.Code != http.StatusCreated {
		t.Errorf("exactly at the limit: %d %s", w.Code, w.Body)
	}
	w := m.upload("alice", card.ID, "big.bin", bytes.Repeat([]byte("x"), 1<<20+1))
	if w.Code != http.StatusRequestEntityTooLarge || !strings.Contains(w.Body.String(), "1 MB") {
		t.Errorf("over the limit: %d %s", w.Code, w.Body)
	}
	if w := m.upload("alice", card.ID, "empty.txt", nil); w.Code != http.StatusBadRequest {
		t.Errorf("empty file: %d", w.Code)
	}
}

func TestLinksAndListAndTrash(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Board")
	card := m.card("alice", m.column("alice", b.ID, "Col").ID, "Card")

	w := m.as("alice", "POST", "/api/v1/cards/"+card.ID+"/links", `{"url":"https://example.com/seeds","title":"Seed list"}`)
	link := decode[service.Attachment](t, w)
	if w.Code != http.StatusCreated || link.Kind != "link" || link.Filename != "Seed list" {
		t.Fatalf("link: %d %+v", w.Code, link)
	}
	for _, bad := range []string{`{"url":"javascript:alert(1)"}`, `{"url":"example.com"}`, `{"url":"ftp://example.com/x"}`} {
		if w := m.as("alice", "POST", "/api/v1/cards/"+card.ID+"/links", bad); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
	file := decode[service.Attachment](t, m.upload("alice", card.ID, "a.txt", []byte("hello")))

	list := decode[struct{ Attachments []service.Attachment }](t, m.as("alice", "GET", "/api/v1/cards/"+card.ID+"/attachments", "")).Attachments
	if len(list) != 2 {
		t.Fatalf("list = %+v", list)
	}
	m.mustJSON("alice", "DELETE", "/api/v1/attachments/"+file.ID, "", http.StatusOK)
	list = decode[struct{ Attachments []service.Attachment }](t, m.as("alice", "GET", "/api/v1/cards/"+card.ID+"/attachments", "")).Attachments
	if len(list) != 1 {
		t.Fatalf("after trash: %+v", list)
	}
	m.mustJSON("alice", "POST", "/api/v1/attachments/"+file.ID+"/restore", "", http.StatusOK)

	// Purge removes the file from storage, not just the row.
	m.mustJSON("alice", "DELETE", "/api/v1/attachments/"+file.ID, "", http.StatusOK)
	n, err := service.PurgeFiles(context.Background(), m.st, m.files, time.Now().Add(time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("purge: %d %v", n, err)
	}
	if _, err := m.files.Open(context.Background(), file.ID); err == nil {
		t.Error("purged file still in storage")
	}
	if w := m.as("alice", "GET", "/api/v1/attachments/"+file.ID, ""); w.Code != http.StatusNotFound {
		t.Errorf("download after purge: %d", w.Code)
	}
}

func TestCardCovers(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Board")
	col := m.column("alice", b.ID, "Col")
	card := m.card("alice", col.ID, "Card")
	other := m.card("alice", col.ID, "Other")
	img := decode[service.Attachment](t, m.upload("alice", card.ID, "p.jpg", gpsPhoto(t)))
	otherImg := decode[service.Attachment](t, m.upload("alice", other.ID, "o.jpg", gpsPhoto(t)))
	txt := decode[service.Attachment](t, m.upload("alice", card.ID, "a.txt", []byte("hello")))

	if c := m.patchCard("alice", card.ID, `{"cover":{"type":"color","value":"sky"}}`); c.Cover.Type != "color" || c.Cover.Value != "sky" {
		t.Fatalf("color cover = %+v", c.Cover)
	}
	if c := m.patchCard("alice", card.ID, `{"cover":{"type":"image","value":"`+img.ID+`"}}`); c.Cover.Type != "image" {
		t.Fatalf("image cover = %+v", c.Cover)
	}
	// A title edit keeps the cover (regression guard, like dates and labels).
	if c := m.patchCard("alice", card.ID, `{"title":"Renamed"}`); c.Cover.Type != "image" || c.Cover.Value != img.ID {
		t.Fatalf("cover lost on rename: %+v", c.Cover)
	}
	for _, bad := range []string{
		`{"cover":{"type":"color","value":"neon"}}`,
		`{"cover":{"type":"image","value":"` + otherImg.ID + `"}}`,
		`{"cover":{"type":"image","value":"` + txt.ID + `"}}`,
		`{"cover":{"type":"poster"}}`,
	} {
		if w := m.as("alice", "PATCH", "/api/v1/cards/"+card.ID, bad); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
	if c := m.patchCard("alice", card.ID, `{"cover":{"type":"none"}}`); c.Cover.Type != "none" || c.Cover.Value != "" {
		t.Fatalf("cleared cover = %+v", c.Cover)
	}
}

func TestAttachmentAuthorization(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Alice's")
	card := m.card("alice", m.column("alice", b.ID, "Col").ID, "Card")
	att := decode[service.Attachment](t, m.upload("alice", card.ID, "a.txt", []byte("private")))

	reads := [][2]string{{"GET", "/api/v1/cards/" + card.ID + "/attachments"}, {"GET", "/api/v1/attachments/" + att.ID}}
	writes := [][3]string{
		{"POST", "/api/v1/cards/" + card.ID + "/links", `{"url":"https://example.com"}`},
		{"DELETE", "/api/v1/attachments/" + att.ID, ""},
		{"POST", "/api/v1/attachments/" + att.ID + "/restore", ""},
	}
	for _, e := range reads {
		if w := m.as("mallory", e[0], e[1], ""); w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "private") {
			t.Errorf("non-member %s %s: %d", e[0], e[1], w.Code)
		}
	}
	for _, e := range writes {
		if w := m.as("mallory", e[0], e[1], e[2]); w.Code != http.StatusNotFound {
			t.Errorf("non-member %s %s: %d", e[0], e[1], w.Code)
		}
	}
	if w := m.upload("mallory", card.ID, "x.txt", []byte("x")); w.Code != http.StatusNotFound {
		t.Errorf("non-member upload: %d", w.Code)
	}

	makeViewer(t, m, b.ID, "bob")
	for _, e := range reads {
		if w := m.as("bob", e[0], e[1], ""); w.Code != http.StatusOK {
			t.Errorf("viewer %s %s: %d", e[0], e[1], w.Code)
		}
	}
	for _, e := range writes {
		if w := m.as("bob", e[0], e[1], e[2]); w.Code != http.StatusForbidden {
			t.Errorf("viewer %s %s: %d", e[0], e[1], w.Code)
		}
	}
	if w := m.upload("bob", card.ID, "x.txt", []byte("x")); w.Code != http.StatusForbidden {
		t.Errorf("viewer upload: %d", w.Code)
	}
	// Unauthenticated: no identity, no file.
	w := m.as("", "GET", "/api/v1/attachments/"+att.ID, "")
	if body, _ := io.ReadAll(w.Body); w.Code != http.StatusUnauthorized || bytes.Contains(body, []byte("private")) {
		t.Errorf("anonymous download: %d", w.Code)
	}
}
