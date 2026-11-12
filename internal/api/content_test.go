// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/runlevel-six/chrysalis/internal/order"
	"github.com/runlevel-six/chrysalis/internal/service"
)

func (m *multiUser) mustJSON(user, method, path, body string, want int) []byte {
	m.t.Helper()
	w := m.as(user, method, path, body)
	if w.Code != want {
		m.t.Fatalf("%s %s as %s: %d %s (want %d)", method, path, user, w.Code, w.Body, want)
	}
	return w.Body.Bytes()
}

func (m *multiUser) column(user, boardID, name string) service.Column {
	m.t.Helper()
	w := m.as(user, "POST", "/api/v1/boards/"+boardID+"/columns", `{"name":`+jsonString(name)+`}`)
	if w.Code != http.StatusCreated {
		m.t.Fatalf("create column: %d %s", w.Code, w.Body)
	}
	return decode[service.Column](m.t, w)
}

func (m *multiUser) card(user, columnID, title string) service.Card {
	m.t.Helper()
	w := m.as(user, "POST", "/api/v1/columns/"+columnID+"/cards", `{"title":`+jsonString(title)+`}`)
	if w.Code != http.StatusCreated {
		m.t.Fatalf("create card: %d %s", w.Code, w.Body)
	}
	return decode[service.Card](m.t, w)
}

func (m *multiUser) detail(user, boardID string) service.BoardDetail {
	m.t.Helper()
	w := m.as(user, "GET", "/api/v1/boards/"+boardID+"/detail", "")
	if w.Code != http.StatusOK {
		m.t.Fatalf("detail: %d %s", w.Code, w.Body)
	}
	return decode[service.BoardDetail](m.t, w)
}

func titles(cards []service.Card, columnID string) []string {
	var out []string
	for _, c := range cards {
		if c.ColumnID == columnID {
			out = append(out, c.Title)
		}
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestColumnsAndCards(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Garden plans")
	todo := m.column("alice", b.ID, "To do")
	done := m.column("alice", b.ID, "Done")

	m.card("alice", todo.ID, "Order seeds")
	water := m.card("alice", todo.ID, "Water tomatoes")
	m.card("alice", todo.ID, "Mulch beds")

	d := m.detail("alice", b.ID)
	if len(d.Columns) != 2 || d.Columns[0].Name != "To do" || d.Columns[1].Name != "Done" {
		t.Fatalf("columns = %+v", d.Columns)
	}
	if got := titles(d.Cards, todo.ID); !eq(got, []string{"Order seeds", "Water tomatoes", "Mulch beds"}) {
		t.Fatalf("new cards go to the bottom: %v", got)
	}
	if d.View.DefaultCardState != "collapsed" {
		t.Errorf("default card state = %q", d.View.DefaultCardState)
	}

	// Move to another column without a position: goes to the bottom there.
	w := m.as("alice", "PATCH", "/api/v1/cards/"+water.ID, `{"columnId":"`+done.ID+`","completed":true}`)
	moved := decode[service.Card](t, w)
	if w.Code != http.StatusOK || moved.ColumnID != done.ID || moved.CompletedAt == nil || moved.Version != 2 {
		t.Fatalf("move: %d %+v", w.Code, moved)
	}

	// Reorder with a client-made key: put "Mulch beds" first.
	d = m.detail("alice", b.ID)
	var first, mulch service.Card
	for _, c := range d.Cards {
		if c.ColumnID == todo.ID && first.ID == "" {
			first = c
		}
		if c.Title == "Mulch beds" {
			mulch = c
		}
	}
	key, err := order.KeyBetween("", first.Position)
	if err != nil {
		t.Fatal(err)
	}
	m.mustJSON("alice", "PATCH", "/api/v1/cards/"+mulch.ID, `{"position":"`+key+`","title":"  Mulch the beds "}`, http.StatusOK)
	d = m.detail("alice", b.ID)
	if got := titles(d.Cards, todo.ID); !eq(got, []string{"Mulch the beds", "Order seeds"}) {
		t.Fatalf("after reorder: %v", got)
	}

	// Un-completing clears completedAt.
	w = m.as("alice", "PATCH", "/api/v1/cards/"+water.ID, `{"completed":false}`)
	if c := decode[service.Card](t, w); c.CompletedAt != nil {
		t.Errorf("completedAt = %v", *c.CompletedAt)
	}

	// Column updates, including clearing the soft limit with 0.
	m.mustJSON("alice", "PATCH", "/api/v1/columns/"+done.ID, `{"name":"Imago","isDone":true,"softLimit":5,"color":"#94a362","colorMode":"full"}`, http.StatusOK)
	w = m.as("alice", "PATCH", "/api/v1/columns/"+done.ID, `{"softLimit":0}`)
	c := decode[service.Column](t, w)
	if c.Name != "Imago" || !c.IsDone || c.SoftLimit != nil || c.ColorMode != "full" {
		t.Errorf("column = %+v", c)
	}
}

func TestContentValidation(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Board")
	col := m.column("alice", b.ID, "Col")
	card := m.card("alice", col.ID, "Card")
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/v1/boards/" + b.ID + "/columns", `{"name":"  "}`},
		{"POST", "/api/v1/columns/" + col.ID + "/cards", `{"title":""}`},
		{"POST", "/api/v1/columns/" + col.ID + "/cards", `{"title":"x","position":"not a key"}`},
		{"PATCH", "/api/v1/cards/" + card.ID, `{"title":" "}`},
		{"PATCH", "/api/v1/cards/" + card.ID, `{"position":"a00"}`},
		{"PATCH", "/api/v1/columns/" + col.ID, `{"colorMode":"sideways"}`},
		{"PATCH", "/api/v1/columns/" + col.ID, `{"softLimit":-1}`},
	} {
		if w := m.as("alice", tc.method, tc.path, tc.body); w.Code != http.StatusBadRequest {
			t.Errorf("%s %s %s: %d, want 400", tc.method, tc.path, tc.body, w.Code)
		}
	}
}

func TestTrashAndRestore(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Board")
	col := m.column("alice", b.ID, "Col")
	keep := m.card("alice", col.ID, "Keep")
	oops := m.card("alice", col.ID, "Oops")

	m.mustJSON("alice", "DELETE", "/api/v1/cards/"+oops.ID, "", http.StatusOK)
	if got := titles(m.detail("alice", b.ID).Cards, col.ID); !eq(got, []string{"Keep"}) {
		t.Fatalf("after trash: %v", got)
	}
	trash := decode[service.Trash](t, m.as("alice", "GET", "/api/v1/boards/"+b.ID+"/trash", ""))
	if len(trash.Cards) != 1 || trash.Cards[0].ID != oops.ID || trash.Cards[0].DeletedAt == nil {
		t.Fatalf("trash = %+v", trash)
	}
	// A trashed card can't be edited until restored.
	if w := m.as("alice", "PATCH", "/api/v1/cards/"+oops.ID, `{"title":"x"}`); w.Code != http.StatusNotFound {
		t.Errorf("edit trashed card: %d", w.Code)
	}
	m.mustJSON("alice", "POST", "/api/v1/cards/"+oops.ID+"/restore", "", http.StatusOK)
	if got := titles(m.detail("alice", b.ID).Cards, col.ID); !eq(got, []string{"Keep", "Oops"}) {
		t.Fatalf("restored card returns to its place: %v", got)
	}

	// Trashing a column hides its cards; restoring brings them all back.
	m.mustJSON("alice", "DELETE", "/api/v1/columns/"+col.ID, "", http.StatusOK)
	d := m.detail("alice", b.ID)
	if len(d.Columns) != 0 || len(d.Cards) != 0 {
		t.Fatalf("after column trash: %+v", d)
	}
	trash = decode[service.Trash](t, m.as("alice", "GET", "/api/v1/boards/"+b.ID+"/trash", ""))
	if len(trash.Columns) != 1 || len(trash.Cards) != 0 {
		t.Fatalf("trash = %+v", trash)
	}
	if w := m.as("alice", "POST", "/api/v1/columns/"+col.ID+"/cards", `{"title":"x"}`); w.Code != http.StatusNotFound {
		t.Errorf("add card to trashed column: %d", w.Code)
	}
	m.mustJSON("alice", "POST", "/api/v1/columns/"+col.ID+"/restore", "", http.StatusOK)
	if got := titles(m.detail("alice", b.ID).Cards, col.ID); !eq(got, []string{"Keep", "Oops"}) {
		t.Fatalf("after column restore: %v", got)
	}

	// Purge removes only what's past the retention cutoff.
	m.mustJSON("alice", "DELETE", "/api/v1/cards/"+keep.ID, "", http.StatusOK)
	if n, err := service.PurgeTrash(context.Background(), m.st, time.Now().Add(-time.Hour)); err != nil || n != 0 {
		t.Fatalf("purge with old cutoff: %d, %v", n, err)
	}
	if n, err := service.PurgeTrash(context.Background(), m.st, time.Now().Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("purge: %d, %v", n, err)
	}
	if w := m.as("alice", "POST", "/api/v1/cards/"+keep.ID+"/restore", ""); w.Code != http.StatusNotFound {
		t.Errorf("restore purged card: %d", w.Code)
	}
}

func TestViewStateIsPerUser(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Board")
	col := m.column("alice", b.ID, "Col")
	c1 := m.card("alice", col.ID, "One")
	c2 := m.card("alice", col.ID, "Two")
	other := m.createBoard("alice", "Other")
	otherCol := m.column("alice", other.ID, "Elsewhere")

	m.mustJSON("alice", "PUT", "/api/v1/boards/"+b.ID+"/view",
		`{"cards":{"`+c1.ID+`":true,"`+c2.ID+`":false,"nope":true},"columns":{"`+col.ID+`":true,"`+otherCol.ID+`":true}}`,
		http.StatusNoContent)
	v := m.detail("alice", b.ID).View
	if !v.Cards[c1.ID] || v.Cards[c2.ID] || !v.Columns[col.ID] || len(v.Cards) != 2 || len(v.Columns) != 1 {
		t.Fatalf("view = %+v", v)
	}
	// An ID from another board was ignored, not saved against it.
	if ov := m.detail("alice", other.ID).View; len(ov.Columns) != 0 {
		t.Errorf("other board view = %+v", ov)
	}

	// Bob, as a viewer, has his own view state; his toggles don't change Alice's.
	makeViewer(t, m, b.ID, "bob")
	m.mustJSON("bob", "PUT", "/api/v1/boards/"+b.ID+"/view", `{"cards":{"`+c1.ID+`":false}}`, http.StatusNoContent)
	if bv := m.detail("bob", b.ID).View; bv.Cards[c1.ID] || len(bv.Columns) != 0 {
		t.Errorf("bob's view = %+v", bv)
	}
	if !m.detail("alice", b.ID).View.Cards[c1.ID] {
		t.Error("bob's toggle changed alice's view")
	}
}

// makeViewer adds user (signing them in first) as a viewer of boardID. The
// sharing UI is Phase 2, so tests write the membership row directly.
func makeViewer(t *testing.T, m *multiUser, boardID, user string) {
	t.Helper()
	m.mustJSON(user, "GET", "/api/v1/me", "", http.StatusOK)
	if _, err := m.st.DB.Exec(`INSERT INTO board_members (board_id, user_id, role)
		SELECT ?, id, 'viewer' FROM users WHERE oidc_sub = ?`, boardID, user); err != nil {
		t.Fatal(err)
	}
}

// docs/DESIGN.md §9: for every endpoint, a non-member is denied and a viewer
// can't mutate.
func TestContentAuthorizationMatrix(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Alice's")
	col := m.column("alice", b.ID, "Col")
	card := m.card("alice", col.ID, "Card")

	reads := []struct{ method, path, body string }{
		{"GET", "/api/v1/boards/" + b.ID + "/detail", ""},
		{"GET", "/api/v1/boards/" + b.ID + "/trash", ""},
		{"PUT", "/api/v1/boards/" + b.ID + "/view", `{"cards":{"` + card.ID + `":true}}`},
	}
	writes := []struct{ method, path, body string }{
		{"POST", "/api/v1/boards/" + b.ID + "/columns", `{"name":"x"}`},
		{"PATCH", "/api/v1/columns/" + col.ID, `{"name":"x"}`},
		{"DELETE", "/api/v1/columns/" + col.ID, ""},
		{"POST", "/api/v1/columns/" + col.ID + "/restore", ""},
		{"POST", "/api/v1/columns/" + col.ID + "/cards", `{"title":"x"}`},
		{"PATCH", "/api/v1/cards/" + card.ID, `{"title":"x"}`},
		{"DELETE", "/api/v1/cards/" + card.ID, ""},
		{"POST", "/api/v1/cards/" + card.ID + "/restore", ""},
	}

	// Mallory isn't a member: everything is "not found".
	for _, e := range append(reads, writes...) {
		if w := m.as("mallory", e.method, e.path, e.body); w.Code != http.StatusNotFound {
			t.Errorf("non-member %s %s: %d, want 404", e.method, e.path, w.Code)
		}
	}

	// Bob is a viewer: reads (and his own view state) work, writes are refused.
	makeViewer(t, m, b.ID, "bob")
	for _, e := range reads {
		if w := m.as("bob", e.method, e.path, e.body); w.Code >= 300 {
			t.Errorf("viewer %s %s: %d, want success", e.method, e.path, w.Code)
		}
	}
	for _, e := range writes {
		if w := m.as("bob", e.method, e.path, e.body); w.Code != http.StatusForbidden {
			t.Errorf("viewer %s %s: %d, want 403", e.method, e.path, w.Code)
		}
	}
	// Nothing changed.
	d := m.detail("alice", b.ID)
	if len(d.Columns) != 1 || d.Columns[0].Name != "Col" || len(d.Cards) != 1 || d.Cards[0].Title != "Card" {
		t.Fatalf("board changed: %+v", d)
	}
}

func TestMoveCardAcrossBoards(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b1 := m.createBoard("alice", "One")
	b2 := m.createBoard("alice", "Two")
	c1 := m.column("alice", b1.ID, "A")
	c2 := m.column("alice", b2.ID, "B")
	card := m.card("alice", c1.ID, "Travelling card")

	w := m.as("alice", "PATCH", "/api/v1/cards/"+card.ID, `{"columnId":"`+c2.ID+`"}`)
	if moved := decode[service.Card](t, w); w.Code != http.StatusOK || moved.BoardID != b2.ID {
		t.Fatalf("move across boards: %d %+v", w.Code, moved)
	}
	if got := titles(m.detail("alice", b2.ID).Cards, c2.ID); !eq(got, []string{"Travelling card"}) {
		t.Fatalf("board two: %v", got)
	}

	// Can't move a card into someone else's board, even knowing a column ID.
	bobs := m.createBoard("bob", "Bob's")
	bobCol := m.column("bob", bobs.ID, "Bob's column")
	if w := m.as("alice", "PATCH", "/api/v1/cards/"+card.ID, `{"columnId":"`+bobCol.ID+`"}`); w.Code != http.StatusNotFound {
		t.Errorf("move into another user's board: %d", w.Code)
	}
	// Or into a board where you're only a viewer.
	makeViewer(t, m, bobs.ID, "alice")
	if w := m.as("alice", "PATCH", "/api/v1/cards/"+card.ID, `{"columnId":"`+bobCol.ID+`"}`); w.Code != http.StatusForbidden {
		t.Errorf("move into a viewed board: %d", w.Code)
	}
}
