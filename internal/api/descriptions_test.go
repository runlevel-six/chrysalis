// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/runlevel-six/chrysalis/internal/service"
)

const docA = `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Plant by the fence"}]}]}`
const docB = `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Plant by the porch"}]}]}`

func putDesc(doc string, base int64) string {
	b, _ := json.Marshal(map[string]any{"doc": json.RawMessage(doc), "baseVersion": base})
	return string(b)
}

func TestDescriptions(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Board")
	col := m.column("alice", b.ID, "Col")
	card := m.card("alice", col.ID, "Card")
	path := "/api/v1/cards/" + card.ID + "/description"

	// Empty to start.
	d := decode[service.Description](t, m.as("alice", "GET", path, ""))
	if string(d.Doc) != "null" || d.Version != 0 || findCard(t, m.detail("alice", b.ID), card.ID).HasDescription {
		t.Fatalf("initial = %+v", d)
	}

	w := m.as("alice", "PUT", path, putDesc(docA, 0))
	d = decode[service.Description](t, w)
	if w.Code != http.StatusOK || d.Version != 1 || !strings.Contains(string(d.Doc), "fence") {
		t.Fatalf("first save: %d %+v", w.Code, d)
	}
	if !findCard(t, m.detail("alice", b.ID), card.ID).HasDescription {
		t.Error("hasDescription not set")
	}

	// Another device saves from version 1 first...
	m.mustJSON("alice", "PUT", path, putDesc(docB, 1), http.StatusOK)
	// ...so a save still based on version 1 is a conflict, and nothing changes.
	w = m.as("alice", "PUT", path, putDesc(docA, 1))
	if w.Code != http.StatusConflict {
		t.Fatalf("stale save: %d %s", w.Code, w.Body)
	}
	conflict := decode[struct{ Current service.Description }](t, w)
	if conflict.Current.Version != 2 || !strings.Contains(string(conflict.Current.Doc), "porch") {
		t.Fatalf("conflict current = %+v", conflict.Current)
	}
	// "Keep yours": resend against the version the server reported.
	m.mustJSON("alice", "PUT", path, putDesc(docA, 2), http.StatusOK)
	if d := decode[service.Description](t, m.as("alice", "GET", path, "")); d.Version != 3 || !strings.Contains(string(d.Doc), "fence") {
		t.Fatalf("after keep-yours: %+v", d)
	}

	// null clears.
	m.mustJSON("alice", "PUT", path, putDesc("null", 3), http.StatusOK)
	if findCard(t, m.detail("alice", b.ID), card.ID).HasDescription {
		t.Error("hasDescription after clearing")
	}

	for _, body := range []string{
		putDesc(`{"type":"paragraph"}`, 4),
		putDesc(`[1,2,3]`, 4),
		putDesc(`{"type":"doc","content":[{"type":"text","text":"`+strings.Repeat("x", service.MaxDescriptionBytes)+`"}]}`, 4),
	} {
		if w := m.as("alice", "PUT", path, body); w.Code != http.StatusBadRequest {
			t.Errorf("%.60s…: %d, want 400", body, w.Code)
		}
	}
}

func TestDescriptionAuthorization(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Alice's")
	col := m.column("alice", b.ID, "Col")
	card := m.card("alice", col.ID, "Card")
	path := "/api/v1/cards/" + card.ID + "/description"
	m.mustJSON("alice", "PUT", path, putDesc(docA, 0), http.StatusOK)

	if w := m.as("mallory", "GET", path, ""); w.Code != http.StatusNotFound {
		t.Errorf("non-member read: %d", w.Code)
	}
	if w := m.as("mallory", "PUT", path, putDesc(docB, 1)); w.Code != http.StatusNotFound {
		t.Errorf("non-member write: %d", w.Code)
	}
	makeViewer(t, m, b.ID, "bob")
	if w := m.as("bob", "GET", path, ""); w.Code != http.StatusOK {
		t.Errorf("viewer read: %d", w.Code)
	}
	if w := m.as("bob", "PUT", path, putDesc(docB, 1)); w.Code != http.StatusForbidden {
		t.Errorf("viewer write: %d", w.Code)
	}
	if d := decode[service.Description](t, m.as("alice", "GET", path, "")); !strings.Contains(string(d.Doc), "fence") {
		t.Fatalf("description changed: %s", d.Doc)
	}
}
