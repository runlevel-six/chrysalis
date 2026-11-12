// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"
	"testing"

	"github.com/runlevel-six/chrysalis/internal/order"
	"github.com/runlevel-six/chrysalis/internal/service"
)

func (m *multiUser) checklist(user, cardID, title string) service.Checklist {
	m.t.Helper()
	w := m.as(user, "POST", "/api/v1/cards/"+cardID+"/checklists", `{"title":`+jsonString(title)+`}`)
	if w.Code != http.StatusCreated {
		m.t.Fatalf("create checklist: %d %s", w.Code, w.Body)
	}
	return decode[service.Checklist](m.t, w)
}

func (m *multiUser) item(user, checklistID, text string) service.ChecklistItem {
	m.t.Helper()
	w := m.as(user, "POST", "/api/v1/checklists/"+checklistID+"/items", `{"text":`+jsonString(text)+`}`)
	if w.Code != http.StatusCreated {
		m.t.Fatalf("create item: %d %s", w.Code, w.Body)
	}
	return decode[service.ChecklistItem](m.t, w)
}

func (m *multiUser) checklists(user, cardID string) []service.Checklist {
	m.t.Helper()
	w := m.as(user, "GET", "/api/v1/cards/"+cardID+"/checklists", "")
	if w.Code != http.StatusOK {
		m.t.Fatalf("list checklists: %d %s", w.Code, w.Body)
	}
	return decode[struct{ Checklists []service.Checklist }](m.t, w).Checklists
}

func itemTexts(cl service.Checklist) []string {
	out := []string{}
	for _, it := range cl.Items {
		out = append(out, it.Text)
	}
	return out
}

func TestChecklists(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Board")
	col := m.column("alice", b.ID, "Col")
	card := m.card("alice", col.ID, "Trip")

	packing := m.checklist("alice", card.ID, "Packing")
	if untitled := m.checklist("alice", card.ID, "  "); untitled.Title != "Checklist" {
		t.Errorf("empty title should default, got %q", untitled.Title)
	}
	socks := m.item("alice", packing.ID, "Socks")
	m.item("alice", packing.ID, "Charger")
	water := m.item("alice", packing.ID, "Water bottle")

	lists := m.checklists("alice", card.ID)
	if len(lists) != 2 || lists[0].Title != "Packing" || !eq(itemTexts(lists[0]), []string{"Socks", "Charger", "Water bottle"}) {
		t.Fatalf("checklists = %+v", lists)
	}

	// Tick two off; the board shows 2/3.
	m.mustJSON("alice", "PATCH", "/api/v1/checklist-items/"+socks.ID, `{"done":true}`, http.StatusOK)
	w := m.as("alice", "PATCH", "/api/v1/checklist-items/"+water.ID, `{"done":true,"text":" Big water bottle "}`)
	if it := decode[service.ChecklistItem](t, w); it.DoneAt == nil || it.Text != "Big water bottle" {
		t.Fatalf("item = %+v", it)
	}
	if c := findCard(t, m.detail("alice", b.ID), card.ID); c.ChecklistDone != 2 || c.ChecklistTotal != 3 {
		t.Fatalf("progress = %d/%d", c.ChecklistDone, c.ChecklistTotal)
	}

	// Reorder: water bottle to the top.
	key, _ := order.KeyBetween("", lists[0].Items[0].Position)
	m.mustJSON("alice", "PATCH", "/api/v1/checklist-items/"+water.ID, `{"position":"`+key+`"}`, http.StatusOK)
	if got := itemTexts(m.checklists("alice", card.ID)[0]); !eq(got, []string{"Big water bottle", "Socks", "Charger"}) {
		t.Fatalf("after reorder: %v", got)
	}

	// Move an item to the other checklist on the same card.
	m.mustJSON("alice", "PATCH", "/api/v1/checklist-items/"+socks.ID, `{"checklistId":"`+lists[1].ID+`"}`, http.StatusOK)
	lists = m.checklists("alice", card.ID)
	if !eq(itemTexts(lists[1]), []string{"Socks"}) {
		t.Fatalf("moved item: %v", itemTexts(lists[1]))
	}
	// But not to a checklist on another card.
	other := m.card("alice", col.ID, "Other")
	foreign := m.checklist("alice", other.ID, "Elsewhere")
	if w := m.as("alice", "PATCH", "/api/v1/checklist-items/"+socks.ID, `{"checklistId":"`+foreign.ID+`"}`); w.Code != http.StatusBadRequest {
		t.Errorf("cross-card move: %d", w.Code)
	}

	// Delete and restore an item, then a whole checklist.
	m.mustJSON("alice", "DELETE", "/api/v1/checklist-items/"+water.ID, "", http.StatusOK)
	if got := itemTexts(m.checklists("alice", card.ID)[0]); !eq(got, []string{"Charger"}) {
		t.Fatalf("after item delete: %v", got)
	}
	m.mustJSON("alice", "POST", "/api/v1/checklist-items/"+water.ID+"/restore", "", http.StatusOK)
	m.mustJSON("alice", "DELETE", "/api/v1/checklists/"+packing.ID, "", http.StatusOK)
	if lists := m.checklists("alice", card.ID); len(lists) != 1 {
		t.Fatalf("after checklist delete: %+v", lists)
	}
	if c := findCard(t, m.detail("alice", b.ID), card.ID); c.ChecklistTotal != 1 {
		t.Errorf("progress after delete = %d/%d", c.ChecklistDone, c.ChecklistTotal)
	}
	m.mustJSON("alice", "POST", "/api/v1/checklists/"+packing.ID+"/restore", "", http.StatusOK)
	if got := itemTexts(m.checklists("alice", card.ID)[0]); !eq(got, []string{"Big water bottle", "Charger"}) {
		t.Fatalf("after restore: %v", got)
	}

	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/v1/checklists/" + packing.ID + "/items", `{"text":"  "}`},
		{"PATCH", "/api/v1/checklists/" + packing.ID, `{"title":""}`},
		{"PATCH", "/api/v1/checklist-items/" + water.ID, `{"position":"???"}`},
	} {
		if w := m.as("alice", tc.method, tc.path, tc.body); w.Code != http.StatusBadRequest {
			t.Errorf("%s %s %s: %d, want 400", tc.method, tc.path, tc.body, w.Code)
		}
	}
}

func TestChecklistAuthorizationMatrix(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Alice's")
	col := m.column("alice", b.ID, "Col")
	card := m.card("alice", col.ID, "Card")
	cl := m.checklist("alice", card.ID, "List")
	it := m.item("alice", cl.ID, "Item")

	read := struct{ method, path, body string }{"GET", "/api/v1/cards/" + card.ID + "/checklists", ""}
	writes := []struct{ method, path, body string }{
		{"POST", "/api/v1/cards/" + card.ID + "/checklists", `{"title":"x"}`},
		{"PATCH", "/api/v1/checklists/" + cl.ID, `{"title":"x"}`},
		{"DELETE", "/api/v1/checklists/" + cl.ID, ""},
		{"POST", "/api/v1/checklists/" + cl.ID + "/restore", ""},
		{"POST", "/api/v1/checklists/" + cl.ID + "/items", `{"text":"x"}`},
		{"PATCH", "/api/v1/checklist-items/" + it.ID, `{"done":true}`},
		{"DELETE", "/api/v1/checklist-items/" + it.ID, ""},
		{"POST", "/api/v1/checklist-items/" + it.ID + "/restore", ""},
	}
	for _, e := range append(writes, read) {
		if w := m.as("mallory", e.method, e.path, e.body); w.Code != http.StatusNotFound {
			t.Errorf("non-member %s %s: %d, want 404", e.method, e.path, w.Code)
		}
	}
	makeViewer(t, m, b.ID, "bob")
	if w := m.as("bob", read.method, read.path, ""); w.Code != http.StatusOK {
		t.Errorf("viewer read: %d", w.Code)
	}
	for _, e := range writes {
		if w := m.as("bob", e.method, e.path, e.body); w.Code != http.StatusForbidden {
			t.Errorf("viewer %s %s: %d, want 403", e.method, e.path, w.Code)
		}
	}
	if got := m.checklists("alice", card.ID); len(got) != 1 || got[0].Items[0].DoneAt != nil {
		t.Fatalf("viewer changed something: %+v", got)
	}
}
