// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/runlevel-six/chrysalis/internal/service"
)

func TestCalendar(t *testing.T) {
	m := newMultiUser(t, testStatic)
	garden := m.createBoard("alice", "Garden")
	kitchen := m.createBoard("alice", "Kitchen")
	gc := m.column("alice", garden.ID, "To do")
	kc := m.column("alice", kitchen.ID, "To do")
	card := func(col, title, patch string) string {
		var c service.Card
		_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/columns/"+col+"/cards", `{"title":"`+title+`"}`, http.StatusCreated), &c)
		if patch != "" {
			m.mustJSON("alice", "PATCH", "/api/v1/cards/"+c.ID, patch, http.StatusOK)
		}
		return c.ID
	}
	card(gc.ID, "Plant bulbs", `{"dueAt":"2026-11-20"}`)
	card(gc.ID, "Order seeds", "")
	card(kc.ID, "Call the plumber", `{"dueAt":"2026-11-21T16:30:00Z","dueHasTime":true}`)
	card(kc.ID, "Next month", `{"dueAt":"2026-12-28"}`)
	m.createBoard("bob", "Bob's board")

	get := func(q string) service.Calendar {
		var c service.Calendar
		_ = json.Unmarshal(m.mustJSON("alice", "GET", "/api/v1/calendar?"+q, "", http.StatusOK), &c)
		return c
	}
	titles := func(cs []service.Card) (out []string) {
		for _, c := range cs {
			out = append(out, c.Title)
		}
		return out
	}

	all := get("from=2026-11-01&to=2026-11-30&undated=1")
	if got := titles(all.Cards); len(got) != 2 || got[0] != "Plant bulbs" || got[1] != "Call the plumber" {
		t.Errorf("all boards = %v", got)
	}
	if got := titles(all.Undated); len(got) != 1 || got[0] != "Order seeds" {
		t.Errorf("undated = %v", got)
	}
	if len(all.Boards) != 2 || len(all.Columns) != 2 {
		t.Errorf("boards %d columns %d (bob's board must not appear)", len(all.Boards), len(all.Columns))
	}

	one := get("board=" + garden.ID + "&from=2026-11-01&to=2026-11-30")
	if got := titles(one.Cards); len(got) != 1 || got[0] != "Plant bulbs" || len(one.Undated) != 0 {
		t.Errorf("one board = %v, undated %v", got, titles(one.Undated))
	}

	m.mustJSON("bob", "GET", "/api/v1/calendar?board="+garden.ID+"&from=2026-11-01&to=2026-11-30", "", http.StatusNotFound)
	m.mustJSON("alice", "GET", "/api/v1/calendar?from=2026-11-30&to=2026-11-01", "", http.StatusBadRequest)
	m.mustJSON("alice", "GET", "/api/v1/calendar?from=2026-01-01&to=2026-12-31", "", http.StatusBadRequest)
}
