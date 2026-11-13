// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"
	"testing"
)

func TestFocusAndHideCompleted(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Board")
	col := m.column("alice", b.ID, "Today")
	l := m.label("alice", b.ID, "Errand", "orange")
	path := "/api/v1/boards/" + b.ID + "/view"

	v := m.detail("alice", b.ID).View
	if v.Focus.Type != "none" || v.HideCompleted {
		t.Fatalf("defaults = %+v", v)
	}

	m.mustJSON("alice", "PUT", path, `{"focus":{"type":"column","columnId":"`+col.ID+`"},"hideCompleted":true}`, http.StatusNoContent)
	v = m.detail("alice", b.ID).View
	if v.Focus.Type != "column" || v.Focus.ColumnID != col.ID || !v.HideCompleted {
		t.Fatalf("after set = %+v", v)
	}

	// A filter focus; hideCompleted untouched when not sent.
	m.mustJSON("alice", "PUT", path, `{"focus":{"type":"filter","query":"  seeds ","labelIds":["`+l.ID+`"]}}`, http.StatusNoContent)
	v = m.detail("alice", b.ID).View
	if v.Focus.Type != "filter" || v.Focus.Query != "seeds" || len(v.Focus.LabelIDs) != 1 || !v.HideCompleted {
		t.Fatalf("filter = %+v", v)
	}

	other := m.createBoard("alice", "Other")
	otherCol := m.column("alice", other.ID, "Elsewhere")
	otherLabel := m.label("alice", other.ID, "x", "red")
	for _, body := range []string{
		`{"focus":{"type":"column","columnId":"` + otherCol.ID + `"}}`,
		`{"focus":{"type":"filter","labelIds":["` + otherLabel.ID + `"]}}`,
		`{"focus":{"type":"sideways"}}`,
	} {
		if w := m.as("alice", "PUT", path, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", body, w.Code)
		}
	}

	// Per user: Bob (a viewer) has his own focus, and can set it.
	makeViewer(t, m, b.ID, "bob")
	if bv := m.detail("bob", b.ID).View; bv.Focus.Type != "none" || bv.HideCompleted {
		t.Errorf("bob sees alice's focus: %+v", bv)
	}
	m.mustJSON("bob", "PUT", path, `{"hideCompleted":true}`, http.StatusNoContent)

	m.mustJSON("alice", "PUT", path, `{"focus":{"type":"none"},"hideCompleted":false}`, http.StatusNoContent)
	if v := m.detail("alice", b.ID).View; v.Focus.Type != "none" || v.HideCompleted {
		t.Fatalf("after clearing = %+v", v)
	}
	if !m.detail("bob", b.ID).View.HideCompleted {
		t.Error("alice's change reached bob's view")
	}
}

func TestLastOpenedBoard(t *testing.T) {
	m := newMultiUser(t, testStatic)
	me := func() string {
		return decode[struct {
			LastBoardID string `json:"lastBoardId"`
		}](t, m.as("alice", "GET", "/api/v1/me", "")).LastBoardID
	}
	if me() != "" {
		t.Fatal("no board opened yet")
	}
	a := m.createBoard("alice", "A")
	b := m.createBoard("alice", "B")
	m.mustJSON("alice", "PUT", "/api/v1/boards/"+a.ID+"/view", `{"opened":true}`, http.StatusNoContent)
	if me() != a.ID {
		t.Fatalf("last = %q, want A", me())
	}
	m.mustJSON("alice", "PUT", "/api/v1/boards/"+b.ID+"/view", `{"opened":true}`, http.StatusNoContent)
	if me() != b.ID {
		t.Fatalf("last = %q, want B", me())
	}
	// Opening a board you can't see changes nothing.
	bobs := m.createBoard("bob", "Bob's")
	if w := m.as("alice", "PUT", "/api/v1/boards/"+bobs.ID+"/view", `{"opened":true}`); w.Code != http.StatusNotFound {
		t.Errorf("open someone else's board: %d", w.Code)
	}
	if me() != b.ID {
		t.Errorf("last = %q after a refused open", me())
	}
}
