// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/runlevel-six/chrysalis/internal/service"
)

func (m *multiUser) quoteCollection(user, name string) service.QuoteCollection {
	m.t.Helper()
	var c service.QuoteCollection
	_ = json.Unmarshal(m.mustJSON(user, "POST", "/api/v1/centering/collections", `{"name":`+jsonString(name)+`}`, http.StatusCreated), &c)
	return c
}

func (m *multiUser) quote(user, collectionID, text string) service.Quote {
	m.t.Helper()
	var q service.Quote
	_ = json.Unmarshal(m.mustJSON(user, "POST", "/api/v1/centering/collections/"+collectionID+"/entries",
		`{"text":`+jsonString(text)+`,"attribution":"A gardener"}`, http.StatusCreated), &q)
	return q
}

func (m *multiUser) line(user, query string) service.CenteringLine {
	m.t.Helper()
	var l service.CenteringLine
	_ = json.Unmarshal(m.mustJSON(user, "GET", "/api/v1/me/centering/line"+query, "", http.StatusOK), &l)
	return l
}

func (m *multiUser) uploadQuotes(user, query, filename, data string) *httptest.ResponseRecorder {
	m.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", filename)
	_, _ = fw.Write([]byte(data))
	_ = mw.Close()
	r := httptest.NewRequest("POST", "/api/v1/centering/import"+query, &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Remote-User", user)
	w := httptest.NewRecorder()
	m.h.ServeHTTP(w, r)
	return w
}

func TestCenteringLineAPI(t *testing.T) {
	m := newMultiUser(t, testStatic)
	col := m.quoteCollection("alice", "Morning")
	one := m.quote("alice", col.ID, "Be still, and breathe.")
	m.quote("alice", col.ID, "Water the roots, not the leaves.")

	// Off by default: nothing to show.
	if l := m.line("alice", "?tz=America/Chicago"); l.Enabled || l.Entry != nil {
		t.Fatalf("line while off = %+v", l)
	}
	var s service.CenteringSettings
	_ = json.Unmarshal(m.mustJSON("alice", "PATCH", "/api/v1/me/centering", `{"enabled":true}`, http.StatusOK), &s)
	if !s.Enabled || len(s.CollectionIDs) != 1 || s.Interval != "daily" {
		t.Fatalf("settings = %+v", s)
	}
	l := m.line("alice", "?tz=America/Chicago&load=tab-1")
	if !l.Enabled || l.Entry == nil || l.Entry.ID != one.ID || l.PoolSize != 2 {
		t.Fatalf("line = %+v", l)
	}

	var got service.CenteringLine
	_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/me/centering/line/next", "", http.StatusOK), &got)
	if got.Entry == nil || got.Entry.ID == one.ID {
		t.Fatalf("next = %+v", got)
	}
	_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/me/centering/line/previous", "", http.StatusOK), &got)
	if got.Entry == nil || got.Entry.ID != one.ID {
		t.Fatalf("previous = %+v", got)
	}
	_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/me/centering/line/pin", "", http.StatusOK), &got)
	if !got.Pinned {
		t.Fatalf("pin = %+v", got)
	}
	_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/me/centering/line/unpin", "", http.StatusOK), &got)
	if got.Pinned {
		t.Fatalf("unpin = %+v", got)
	}
	_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/me/centering/line/hide", `{"tz":"America/Chicago"}`, http.StatusOK), &got)
	if !got.Hidden || got.Entry != nil {
		t.Fatalf("hide = %+v", got)
	}
	if l := m.line("alice", "?tz=Europe/London"); !l.Hidden {
		t.Fatalf("hidden on another device = %+v", l)
	}
	_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/me/centering/line/show", "", http.StatusOK), &got)
	if got.Hidden || got.Entry == nil {
		t.Fatalf("show = %+v", got)
	}
	if w := m.as("alice", "PATCH", "/api/v1/me/centering", `{"interval":"fortnightly"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad interval: %d", w.Code)
	}

	// Bob has his own line, off, and none of Alice's collections.
	if l := m.line("bob", ""); l.Enabled || l.Entry != nil {
		t.Fatalf("bob's line = %+v", l)
	}
}

func TestCenteringCollectionsAPI(t *testing.T) {
	m := newMultiUser(t, testStatic)
	col := m.quoteCollection("alice", "Morning")
	q := m.quote("alice", col.ID, "Be still, and breathe.")
	base := "/api/v1/centering/collections/" + col.ID

	var e service.Quote
	_ = json.Unmarshal(m.mustJSON("alice", "PATCH", "/api/v1/centering/entries/"+q.ID, `{"reference":"Notebook 1","tags":["calm"]}`, http.StatusOK), &e)
	if e.Reference != "Notebook 1" || len(e.Tags) != 1 || e.Text != q.Text {
		t.Fatalf("patch = %+v", e)
	}
	m.mustJSON("alice", "DELETE", "/api/v1/centering/entries/"+q.ID, "", http.StatusOK)
	var trash service.CenteringTrash
	_ = json.Unmarshal(m.mustJSON("alice", "GET", "/api/v1/centering/trash", "", http.StatusOK), &trash)
	if len(trash.Entries) != 1 {
		t.Fatalf("trash = %+v", trash)
	}
	m.mustJSON("alice", "POST", "/api/v1/centering/entries/"+q.ID+"/restore", "", http.StatusOK)
	var list struct{ Entries []service.Quote }
	_ = json.Unmarshal(m.mustJSON("alice", "GET", base+"/entries", "", http.StatusOK), &list)
	if len(list.Entries) != 1 {
		t.Fatalf("entries = %+v", list)
	}
	if w := m.as("alice", "POST", base+"/entries", `{"text":"   "}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "needs some text") {
		t.Fatalf("empty entry: %d %s", w.Code, w.Body)
	}

	var c service.QuoteCollection
	_ = json.Unmarshal(m.mustJSON("alice", "PATCH", base, `{"name":"Evening"}`, http.StatusOK), &c)
	if c.Name != "Evening" || c.EntryCount != 1 {
		t.Fatalf("rename = %+v", c)
	}
	m.mustJSON("alice", "DELETE", base, "", http.StatusOK)
	var cols struct{ Collections []service.QuoteCollection }
	_ = json.Unmarshal(m.mustJSON("alice", "GET", "/api/v1/centering/collections", "", http.StatusOK), &cols)
	if len(cols.Collections) != 0 {
		t.Fatalf("trashed collection listed: %+v", cols)
	}
	m.mustJSON("alice", "POST", base+"/restore", "", http.StatusOK)
	_ = json.Unmarshal(m.mustJSON("alice", "GET", "/api/v1/centering/collections", "", http.StatusOK), &cols)
	if len(cols.Collections) != 1 {
		t.Fatalf("restored collection missing: %+v", cols)
	}
}

func TestCenteringAuthorizationMatrix(t *testing.T) {
	m := newMultiUser(t, testStatic)
	col := m.quoteCollection("alice", "Morning")
	q := m.quote("alice", col.ID, "Be still, and breathe.")
	m.as("bob", "GET", "/api/v1/me", "") // bob exists

	var cols struct{ Collections []service.QuoteCollection }
	_ = json.Unmarshal(m.mustJSON("bob", "GET", "/api/v1/centering/collections", "", http.StatusOK), &cols)
	if len(cols.Collections) != 0 {
		t.Fatalf("bob sees %+v", cols)
	}
	base := "/api/v1/centering/collections/" + col.ID
	for _, tc := range []struct{ method, path, body string }{
		{"GET", base + "/entries", ""},
		{"POST", base + "/entries", `{"text":"mine now"}`},
		{"PATCH", base, `{"name":"x"}`},
		{"DELETE", base, ""},
		{"POST", base + "/restore", ""},
		{"GET", base + "/export", ""},
		{"PATCH", "/api/v1/centering/entries/" + q.ID, `{"text":"x"}`},
		{"DELETE", "/api/v1/centering/entries/" + q.ID, ""},
		{"POST", "/api/v1/centering/entries/" + q.ID + "/restore", ""},
		{"POST", "/api/v1/centering/import?collectionId=" + col.ID, `{"entries":[{"text":"x"}]}`},
	} {
		if w := m.as("bob", tc.method, tc.path, tc.body); w.Code != http.StatusNotFound {
			t.Errorf("bob %s %s: %d %s", tc.method, tc.path, w.Code, w.Body)
		}
	}
	for _, tc := range []struct{ method, path, body string }{
		{"PATCH", "/api/v1/me/centering", `{"collectionIds":["` + col.ID + `"]}`},
		{"POST", "/api/v1/me/centering/line/pin", `{"entryId":"` + q.ID + `"}`},
	} {
		if w := m.as("bob", tc.method, tc.path, tc.body); w.Code != http.StatusBadRequest {
			t.Errorf("bob %s %s: %d %s", tc.method, tc.path, w.Code, w.Body)
		}
	}
	// Nothing of Alice's changed, and nothing reached Bob's trash.
	var list struct{ Entries []service.Quote }
	_ = json.Unmarshal(m.mustJSON("alice", "GET", base+"/entries", "", http.StatusOK), &list)
	if len(list.Entries) != 1 || list.Entries[0].Text != q.Text {
		t.Fatalf("alice's entries = %+v", list)
	}
	var trash service.CenteringTrash
	_ = json.Unmarshal(m.mustJSON("bob", "GET", "/api/v1/centering/trash", "", http.StatusOK), &trash)
	if len(trash.Entries)+len(trash.Collections) != 0 {
		t.Fatalf("bob's trash = %+v", trash)
	}

	// A viewer (sharing arrives in Phase 2) reads but can't change.
	var bobID string
	if err := m.st.DB.QueryRow(`SELECT id FROM users WHERE oidc_sub = 'bob'`).Scan(&bobID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.st.DB.Exec(`INSERT INTO quote_collection_members (collection_id, user_id, role) VALUES (?, ?, 'viewer')`, col.ID, bobID); err != nil {
		t.Fatal(err)
	}
	m.mustJSON("bob", "GET", base+"/entries", "", http.StatusOK)
	m.mustJSON("bob", "GET", base+"/export", "", http.StatusOK)
	if w := m.as("bob", "POST", base+"/entries", `{"text":"x"}`); w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "collection") {
		t.Fatalf("viewer adding: %d %s", w.Code, w.Body)
	}
	if w := m.as("bob", "DELETE", base, ""); w.Code != http.StatusForbidden {
		t.Fatalf("viewer deleting the collection: %d", w.Code)
	}
}

func TestCenteringImportExportAPI(t *testing.T) {
	m := newMultiUser(t, testStatic)
	yamlFile := "collection: Morning\nentries:\n  - text: Be still, and breathe.\n    attribution: A gardener\n  - text: Rest is part of the work.\n"
	w := m.uploadQuotes("alice", "", "morning.yaml", yamlFile)
	if w.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	res := decode[service.QuoteImportResult](t, w)
	if !res.Created || res.Added != 2 || res.Collection.Name != "Morning" {
		t.Fatalf("import = %+v", res)
	}
	// Additive by default; ?replace=1 replaces.
	w = m.as("alice", "POST", "/api/v1/centering/import?collection=Morning", `{"entries":[{"text":"One more."}]}`)
	if w.Code != http.StatusCreated || decode[service.QuoteImportResult](t, w).Collection.EntryCount != 3 {
		t.Fatalf("json body import: %d %s", w.Code, w.Body)
	}
	w = m.uploadQuotes("alice", "?replace=1&collectionId="+res.Collection.ID, "x.json", `{"entries":[{"text":"Only this."}]}`)
	if r := decode[service.QuoteImportResult](t, w); w.Code != http.StatusCreated || r.Replaced != 3 || r.Collection.EntryCount != 1 {
		t.Fatalf("replace: %d %s", w.Code, w.Body)
	}
	// Calm errors.
	w = m.uploadQuotes("alice", "", "bad.yaml", "entries:\n  - text: hi\n    author: me\n")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "author") {
		t.Fatalf("unknown field: %d %s", w.Code, w.Body)
	}
	w = m.uploadQuotes("alice", "", "big.yaml", strings.Repeat("#", service.MaxQuoteFileBytes+10))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized: %d", w.Code)
	}

	// Export downloads as a file, YAML by default.
	w = m.as("alice", "GET", "/api/v1/centering/collections/"+res.Collection.ID+"/export", "")
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") ||
		!strings.Contains(w.Header().Get("Content-Disposition"), "morning.yaml") || !strings.Contains(w.Body.String(), "text: Only this.") {
		t.Fatalf("export: %d %v %s", w.Code, w.Header(), w.Body)
	}
	w = m.as("alice", "GET", "/api/v1/centering/collections/"+res.Collection.ID+"/export?format=json", "")
	var f service.QuoteFile
	if err := json.Unmarshal(w.Body.Bytes(), &f); err != nil || f.Collection != "Morning" || len(f.Entries) != 1 {
		t.Fatalf("json export: %v %s", err, w.Body)
	}
}

func TestBoardCenteringOverride(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Board")
	if v := m.detail("alice", b.ID).View; v.CenteringLine != nil {
		t.Fatalf("default override = %v", *v.CenteringLine)
	}
	m.mustJSON("alice", "PUT", "/api/v1/boards/"+b.ID+"/view", `{"centeringLine":false}`, http.StatusNoContent)
	v := m.detail("alice", b.ID).View
	if v.CenteringLine == nil || *v.CenteringLine {
		t.Fatalf("after hiding = %v", v.CenteringLine)
	}
	// Other view changes leave it alone.
	m.mustJSON("alice", "PUT", "/api/v1/boards/"+b.ID+"/view", `{"hideCompleted":true}`, http.StatusNoContent)
	if v := m.detail("alice", b.ID).View; v.CenteringLine == nil || *v.CenteringLine {
		t.Fatalf("after another change = %v", v.CenteringLine)
	}
}
