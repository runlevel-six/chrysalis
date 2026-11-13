// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/runlevel-six/chrysalis/internal/service"
)

func TestCustomFields(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Garden")
	col := m.column("alice", b.ID, "To do")
	var c service.Card
	_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/columns/"+col.ID+"/cards", `{"title":"Plant bulbs"}`, http.StatusCreated), &c)

	field := func(body string) service.CustomField {
		var f service.CustomField
		_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/boards/"+b.ID+"/fields", body, http.StatusCreated), &f)
		return f
	}
	cost := field(`{"name":"Cost","type":"number"}`)
	bed := field(`{"name":"Bed","type":"select","options":[{"name":"North","color":"green"},{"name":"South"}]}`)
	when := field(`{"name":"Planted","type":"date"}`)
	note := field(`{"name":"Variety","type":"text"}`)
	ok := field(`{"name":"Ordered","type":"checkbox"}`)
	if len(bed.Options) != 2 || bed.Options[0].ID == "" || bed.Position <= cost.Position {
		t.Fatalf("select field = %+v", bed)
	}
	m.mustJSON("alice", "POST", "/api/v1/boards/"+b.ID+"/fields", `{"name":"x","type":"colour"}`, http.StatusBadRequest)
	m.mustJSON("alice", "POST", "/api/v1/boards/"+b.ID+"/fields", `{"name":"","type":"text"}`, http.StatusBadRequest)

	set := func(fieldID, value string, want int) {
		m.mustJSON("alice", "PUT", "/api/v1/cards/"+c.ID+"/fields/"+fieldID, `{"value":`+value+`}`, want)
	}
	set(cost.ID, `12.5`, http.StatusOK)
	set(cost.ID, `"twelve"`, http.StatusBadRequest)
	set(bed.ID, `"`+bed.Options[1].ID+`"`, http.StatusOK)
	set(bed.ID, `"no-such-choice"`, http.StatusBadRequest)
	set(when.ID, `"2026-11-20"`, http.StatusOK)
	set(when.ID, `"next week"`, http.StatusBadRequest)
	set(note.ID, `"Purple Sensation"`, http.StatusOK)
	set(ok.ID, `true`, http.StatusOK)

	type boardDetail struct {
		Fields []service.CustomField
		Cards  []service.Card
	}
	// A fresh value each time: decoding into an old one merges maps.
	detail := func() (d boardDetail) {
		_ = json.Unmarshal(m.mustJSON("alice", "GET", "/api/v1/boards/"+b.ID+"/detail", "", http.StatusOK), &d)
		return d
	}
	d := detail()
	if len(d.Fields) != 5 || len(d.Cards) != 1 || len(d.Cards[0].Fields) != 5 || string(d.Cards[0].Fields[cost.ID]) != "12.5" {
		t.Fatalf("detail fields=%d card fields=%v", len(d.Fields), d.Cards[0].Fields)
	}

	// Clearing a value; removing a choice clears values that used it.
	set(note.ID, `null`, http.StatusOK)
	m.mustJSON("alice", "PATCH", "/api/v1/fields/"+bed.ID, `{"options":[{"id":"`+bed.Options[0].ID+`","name":"North"}],"showOnCollapsed":true}`, http.StatusOK)
	d = detail()
	if _, has := d.Cards[0].Fields[note.ID]; has {
		t.Error("cleared text still set")
	}
	if _, has := d.Cards[0].Fields[bed.ID]; has {
		t.Error("value for a removed choice still set")
	}

	// Trash a field: it and its values disappear, and come back on restore.
	m.mustJSON("alice", "DELETE", "/api/v1/fields/"+cost.ID, "", http.StatusOK)
	d = detail()
	if len(d.Fields) != 4 {
		t.Errorf("after trashing, %d fields", len(d.Fields))
	}
	if _, has := d.Cards[0].Fields[cost.ID]; has {
		t.Error("a trashed field's value still shows")
	}
	m.mustJSON("alice", "POST", "/api/v1/fields/"+cost.ID+"/restore", "", http.StatusOK)
	d = detail()
	if string(d.Cards[0].Fields[cost.ID]) != "12.5" {
		t.Errorf("restored value = %s", d.Cards[0].Fields[cost.ID])
	}

	// Others can't see or change any of it.
	m.mustJSON("bob", "PATCH", "/api/v1/fields/"+cost.ID, `{"name":"Mine"}`, http.StatusNotFound)
	m.mustJSON("bob", "PUT", "/api/v1/cards/"+c.ID+"/fields/"+cost.ID, `{"value":1}`, http.StatusNotFound)

	// A field from another board can't be set on this card.
	other := m.createBoard("alice", "Kitchen")
	var of service.CustomField
	_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/boards/"+other.ID+"/fields", `{"name":"Aisle","type":"text"}`, http.StatusCreated), &of)
	set(of.ID, `"3"`, http.StatusNotFound)
}
