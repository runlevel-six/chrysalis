// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"
	"testing"

	"github.com/runlevel-six/chrysalis/internal/service"
)

func (m *multiUser) label(user, boardID, name, color string) service.Label {
	m.t.Helper()
	w := m.as(user, "POST", "/api/v1/boards/"+boardID+"/labels", `{"name":`+jsonString(name)+`,"color":"`+color+`"}`)
	if w.Code != http.StatusCreated {
		m.t.Fatalf("create label: %d %s", w.Code, w.Body)
	}
	return decode[service.Label](m.t, w)
}

func (m *multiUser) patchCard(user, cardID, body string) service.Card {
	m.t.Helper()
	w := m.as(user, "PATCH", "/api/v1/cards/"+cardID, body)
	if w.Code != http.StatusOK {
		m.t.Fatalf("patch card %s: %d %s", body, w.Code, w.Body)
	}
	return decode[service.Card](m.t, w)
}

func findCard(t *testing.T, d service.BoardDetail, id string) service.Card {
	t.Helper()
	for _, c := range d.Cards {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("card %s not on board", id)
	return service.Card{}
}

func TestLabels(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Board")
	col := m.column("alice", b.ID, "Col")
	card := m.card("alice", col.ID, "Card")
	urgent := m.label("alice", b.ID, " Urgent ", "orange")
	m.label("alice", b.ID, "", "sky") // color-only labels are fine

	c := m.patchCard("alice", card.ID, `{"labelIds":["`+urgent.ID+`"]}`)
	if len(c.LabelIDs) != 1 || c.LabelIDs[0] != urgent.ID {
		t.Fatalf("labelIds = %v", c.LabelIDs)
	}
	d := m.detail("alice", b.ID)
	if len(d.Labels) != 2 || d.Labels[0].Name != "Urgent" || len(findCard(t, d, card.ID).LabelIDs) != 1 {
		t.Fatalf("detail labels = %+v", d.Labels)
	}

	// Rename and recolor.
	w := m.as("alice", "PATCH", "/api/v1/labels/"+urgent.ID, `{"name":"Soon","color":"yellow"}`)
	if l := decode[service.Label](t, w); l.Name != "Soon" || l.Color != "yellow" {
		t.Fatalf("label = %+v", l)
	}

	// Trashing a label hides it from cards; restoring brings it back on them.
	m.mustJSON("alice", "DELETE", "/api/v1/labels/"+urgent.ID, "", http.StatusOK)
	if ids := findCard(t, m.detail("alice", b.ID), card.ID).LabelIDs; len(ids) != 0 {
		t.Fatalf("trashed label still on card: %v", ids)
	}
	m.mustJSON("alice", "POST", "/api/v1/labels/"+urgent.ID+"/restore", "", http.StatusOK)
	if ids := findCard(t, m.detail("alice", b.ID), card.ID).LabelIDs; len(ids) != 1 {
		t.Fatalf("restored label not back on card: %v", ids)
	}

	// Clearing with an empty list.
	if c := m.patchCard("alice", card.ID, `{"labelIds":[]}`); len(c.LabelIDs) != 0 {
		t.Fatalf("labelIds = %v", c.LabelIDs)
	}

	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/v1/boards/" + b.ID + "/labels", `{"name":"x","color":"chartreuse"}`},
		{"POST", "/api/v1/boards/" + b.ID + "/labels", `{"name":"this label name is far too long to be a calm label really","color":"red"}`},
		{"PATCH", "/api/v1/labels/" + urgent.ID, `{"color":"neon"}`},
		{"PATCH", "/api/v1/cards/" + card.ID, `{"labelIds":["nope"]}`},
	} {
		if w := m.as("alice", tc.method, tc.path, tc.body); w.Code != http.StatusBadRequest {
			t.Errorf("%s %s %s: %d, want 400", tc.method, tc.path, tc.body, w.Code)
		}
	}

	// A label from another board can't be put on this card.
	other := m.createBoard("alice", "Other")
	foreign := m.label("alice", other.ID, "Elsewhere", "red")
	if w := m.as("alice", "PATCH", "/api/v1/cards/"+card.ID, `{"labelIds":["`+foreign.ID+`"]}`); w.Code != http.StatusBadRequest {
		t.Errorf("foreign label: %d", w.Code)
	}
}

func TestCardDates(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Board")
	col := m.column("alice", b.ID, "Col")
	card := m.card("alice", col.ID, "Card")

	c := m.patchCard("alice", card.ID, `{"startAt":"2026-05-01","dueAt":"2026-05-31"}`)
	if c.StartAt == nil || *c.StartAt != "2026-05-01" || c.DueAt == nil || *c.DueAt != "2026-05-31" || c.DueHasTime {
		t.Fatalf("dates = %+v", c)
	}
	// With a time, normalized to UTC.
	c = m.patchCard("alice", card.ID, `{"dueAt":"2026-05-31T17:00:00-05:00","dueHasTime":true}`)
	if *c.DueAt != "2026-05-31T22:00:00.000Z" || !c.DueHasTime {
		t.Fatalf("due with time = %v %v", *c.DueAt, c.DueHasTime)
	}
	// null clears; clearing due also clears dueHasTime.
	c = m.patchCard("alice", card.ID, `{"dueAt":null}`)
	if c.DueAt != nil || c.DueHasTime || c.StartAt == nil {
		t.Fatalf("after clearing due: %+v", c)
	}
	c = m.patchCard("alice", card.ID, `{"startAt":null}`)
	if c.StartAt != nil {
		t.Fatalf("start not cleared")
	}

	for _, body := range []string{
		`{"startAt":"May 1"}`,
		`{"dueAt":"2026-13-01"}`,
		`{"dueAt":"2026-05-31","dueHasTime":true}`,
		`{"startAt":"2026-06-02","dueAt":"2026-06-01"}`,
	} {
		if w := m.as("alice", "PATCH", "/api/v1/cards/"+card.ID, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", body, w.Code)
		}
	}
}

// Regression: an update that doesn't mention a field must leave it alone.
// Before this was handled, a title edit or a move cleared dates, archive
// state and labels.
func TestCardUpdatesKeepOtherFields(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Board")
	a := m.column("alice", b.ID, "A")
	z := m.column("alice", b.ID, "Z")
	card := m.card("alice", a.ID, "Card")
	l := m.label("alice", b.ID, "Keep", "green")
	m.patchCard("alice", card.ID, `{"startAt":"2026-05-01","dueAt":"2026-05-31T17:00:00Z","dueHasTime":true,"labelIds":["`+l.ID+`"],"completed":true}`)

	for _, body := range []string{`{"title":"Renamed"}`, `{"columnId":"` + z.ID + `"}`, `{"position":"a5"}`} {
		c := m.patchCard("alice", card.ID, body)
		if c.StartAt == nil || c.DueAt == nil || !c.DueHasTime || c.CompletedAt == nil || len(c.LabelIDs) != 1 {
			t.Errorf("after %s: %+v", body, c)
		}
	}
}

func TestArchiveCards(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Board")
	col := m.column("alice", b.ID, "Col")
	card := m.card("alice", col.ID, "Done with this")
	m.card("alice", col.ID, "Still here")

	c := m.patchCard("alice", card.ID, `{"archived":true}`)
	if c.ArchivedAt == nil {
		t.Fatal("archivedAt not set")
	}
	if got := titles(m.detail("alice", b.ID).Cards, col.ID); !eq(got, []string{"Still here"}) {
		t.Fatalf("archived card still on board: %v", got)
	}
	w := m.as("alice", "GET", "/api/v1/boards/"+b.ID+"/archived", "")
	if got := decode[struct{ Cards []service.Card }](t, w).Cards; len(got) != 1 || got[0].ID != card.ID {
		t.Fatalf("archived list = %+v", got)
	}
	m.patchCard("alice", card.ID, `{"archived":false}`)
	if got := titles(m.detail("alice", b.ID).Cards, col.ID); !eq(got, []string{"Done with this", "Still here"}) {
		t.Fatalf("unarchived card not back in place: %v", got)
	}
}

func TestMoveAcrossBoardsDropsLabels(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b1 := m.createBoard("alice", "One")
	b2 := m.createBoard("alice", "Two")
	c1 := m.column("alice", b1.ID, "A")
	c2 := m.column("alice", b2.ID, "B")
	card := m.card("alice", c1.ID, "Card")
	l := m.label("alice", b1.ID, "Board one only", "red")
	m.patchCard("alice", card.ID, `{"labelIds":["`+l.ID+`"]}`)

	if c := m.patchCard("alice", card.ID, `{"columnId":"`+c2.ID+`"}`); len(c.LabelIDs) != 0 {
		t.Fatalf("labels travelled to another board: %v", c.LabelIDs)
	}
}

func TestBoardSettingsAndRename(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Board")
	if b.Settings.DefaultCardState != "collapsed" || b.Settings.ShowLabels || b.Settings.ShowDueDate || b.Settings.ShowChecklist {
		t.Fatalf("defaults = %+v (everything extra should be off)", b.Settings)
	}
	w := m.as("alice", "PATCH", "/api/v1/boards/"+b.ID, `{"name":"Garden","settings":{"defaultCardState":"expanded","showLabels":true,"showDueDate":true}}`)
	got := decode[service.Board](t, w)
	if w.Code != http.StatusOK || got.Name != "Garden" || !got.Settings.ShowLabels || got.Settings.ShowChecklist {
		t.Fatalf("patch board: %d %+v", w.Code, got)
	}
	if d := m.detail("alice", b.ID); d.View.DefaultCardState != "expanded" || d.Board.Name != "Garden" {
		t.Fatalf("detail = %+v", d.View)
	}
	for _, body := range []string{`{"name":" "}`, `{"settings":{"defaultCardState":"sideways"}}`} {
		if w := m.as("alice", "PATCH", "/api/v1/boards/"+b.ID, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", body, w.Code)
		}
	}
}

func TestDetailsAuthorizationMatrix(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Alice's")
	l := m.label("alice", b.ID, "Mine", "green")

	writes := []struct{ method, path, body string }{
		{"PATCH", "/api/v1/boards/" + b.ID, `{"name":"x"}`},
		{"POST", "/api/v1/boards/" + b.ID + "/labels", `{"name":"x","color":"red"}`},
		{"PATCH", "/api/v1/labels/" + l.ID, `{"name":"x"}`},
		{"DELETE", "/api/v1/labels/" + l.ID, ""},
		{"POST", "/api/v1/labels/" + l.ID + "/restore", ""},
	}
	read := struct{ method, path, body string }{"GET", "/api/v1/boards/" + b.ID + "/archived", ""}

	for _, e := range append(writes, read) {
		if w := m.as("mallory", e.method, e.path, e.body); w.Code != http.StatusNotFound {
			t.Errorf("non-member %s %s: %d, want 404", e.method, e.path, w.Code)
		}
	}
	makeViewer(t, m, b.ID, "bob")
	if w := m.as("bob", read.method, read.path, ""); w.Code != http.StatusOK {
		t.Errorf("viewer read archived: %d", w.Code)
	}
	for _, e := range writes {
		if w := m.as("bob", e.method, e.path, e.body); w.Code != http.StatusForbidden {
			t.Errorf("viewer %s %s: %d, want 403", e.method, e.path, w.Code)
		}
	}
}
