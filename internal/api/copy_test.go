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

func TestCopyCardAndTemplates(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Garden")
	col := m.column("alice", b.ID, "To do")
	done := m.column("alice", b.ID, "Done")
	var c service.Card
	_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/columns/"+col.ID+"/cards", `{"title":"Plant bulbs"}`, http.StatusCreated), &c)
	var after service.Card
	_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/columns/"+col.ID+"/cards", `{"title":"Next one"}`, http.StatusCreated), &after)

	// Fill the card: label, dates, checklist (one item ticked), field,
	// a photo as cover and inline in the description.
	var l service.Label
	_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/boards/"+b.ID+"/labels", `{"name":"outside","color":"green"}`, http.StatusCreated), &l)
	m.mustJSON("alice", "PATCH", "/api/v1/cards/"+c.ID, `{"labelIds":["`+l.ID+`"],"dueAt":"2026-11-20"}`, http.StatusOK)
	cl := m.checklist("alice", c.ID, "Steps")
	it := m.item("alice", cl.ID, "Dig")
	m.item("alice", cl.ID, "Plant")
	m.mustJSON("alice", "PATCH", "/api/v1/checklist-items/"+it.ID, `{"done":true}`, http.StatusOK)
	var f service.CustomField
	_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/boards/"+b.ID+"/fields", `{"name":"Cost","type":"number"}`, http.StatusCreated), &f)
	m.mustJSON("alice", "PUT", "/api/v1/cards/"+c.ID+"/fields/"+f.ID, `{"value":4}`, http.StatusOK)
	up := m.upload("alice", c.ID, "bulbs.png", widePNG(t, 40, 20))
	var att service.Attachment
	_ = json.Unmarshal(up.Body.Bytes(), &att)
	m.mustJSON("alice", "PATCH", "/api/v1/cards/"+c.ID, `{"cover":{"type":"image","value":"`+att.ID+`"}}`, http.StatusOK)
	doc := `{"type":"doc","content":[{"type":"image","attrs":{"src":"attachment:` + att.ID + `"}}]}`
	m.mustJSON("alice", "PUT", "/api/v1/cards/"+c.ID+"/description", `{"doc":`+doc+`,"baseVersion":0}`, http.StatusOK)

	// A full copy lands just after the original, with everything.
	var cp service.Card
	_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/cards/"+c.ID+"/copy",
		`{"include":{"checklists":true,"labels":true,"description":true,"dates":true,"fields":true,"attachments":true}}`, http.StatusCreated), &cp)
	if cp.ID == c.ID || cp.Title != "Plant bulbs" || len(cp.LabelIDs) != 1 || cp.DueAt == nil || string(cp.Fields[f.ID]) != "4" {
		t.Fatalf("copy = %+v", cp)
	}
	if cp.Position <= c.Position || cp.Position >= after.Position {
		t.Errorf("copy position %q not between %q and %q", cp.Position, c.Position, after.Position)
	}
	lists := m.checklists("alice", cp.ID)
	if len(lists) != 1 || len(lists[0].Items) != 2 || lists[0].Items[0].DoneAt == nil {
		t.Errorf("copied checklist = %+v", lists)
	}
	var atts struct{ Attachments []service.Attachment }
	_ = json.Unmarshal(m.mustJSON("alice", "GET", "/api/v1/cards/"+cp.ID+"/attachments", "", http.StatusOK), &atts)
	if len(atts.Attachments) != 1 || atts.Attachments[0].ID == att.ID {
		t.Fatalf("copied attachments = %+v", atts.Attachments)
	}
	newAtt := atts.Attachments[0].ID
	if cp.Cover.Type != "image" || cp.Cover.Value != newAtt {
		t.Errorf("cover = %+v, want the copy's own photo %s", cp.Cover, newAtt)
	}
	desc := string(m.mustJSON("alice", "GET", "/api/v1/cards/"+cp.ID+"/description", "", http.StatusOK))
	if !strings.Contains(desc, "attachment:"+newAtt) || strings.Contains(desc, "attachment:"+att.ID) {
		t.Errorf("description images not remapped: %s", desc)
	}
	// Deleting the original's photo leaves the copy's intact.
	m.mustJSON("alice", "DELETE", "/api/v1/attachments/"+att.ID, "", http.StatusOK)
	if w := m.as("alice", "GET", "/api/v1/attachments/"+newAtt, ""); w.Code != http.StatusOK || w.Body.Len() == 0 {
		t.Errorf("copy's photo: %d", w.Code)
	}

	// A bare copy, renamed, to another column.
	var bare service.Card
	_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/cards/"+c.ID+"/copy", `{"title":"Plant tulips","columnId":"`+done.ID+`"}`, http.StatusCreated), &bare)
	if bare.Title != "Plant tulips" || bare.ColumnID != done.ID || len(bare.LabelIDs) != 0 || bare.DueAt != nil || len(bare.Fields) != 0 {
		t.Errorf("bare copy = %+v", bare)
	}

	// Save as a template: out of the columns, items unticked.
	var tpl service.Card
	_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/cards/"+c.ID+"/copy",
		`{"title":"Planting","template":true,"include":{"checklists":true,"labels":true,"fields":true}}`, http.StatusCreated), &tpl)
	var d struct {
		Cards     []service.Card
		Templates []service.Card
	}
	_ = json.Unmarshal(m.mustJSON("alice", "GET", "/api/v1/boards/"+b.ID+"/detail", "", http.StatusOK), &d)
	if len(d.Templates) != 1 || d.Templates[0].ID != tpl.ID || !d.Templates[0].IsTemplate {
		t.Fatalf("templates = %+v", d.Templates)
	}
	for _, x := range d.Cards {
		if x.ID == tpl.ID {
			t.Fatal("a template shows in the columns")
		}
	}
	if items := m.checklists("alice", tpl.ID)[0].Items; items[0].DoneAt != nil {
		t.Error("template items should start unticked")
	}

	// A new card from the template goes to the bottom of the chosen column.
	var fromTpl service.Card
	_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/cards/"+tpl.ID+"/copy",
		`{"title":"Plant crocuses","columnId":"`+col.ID+`","freshChecklists":true,"include":{"checklists":true,"labels":true,"fields":true}}`, http.StatusCreated), &fromTpl)
	if fromTpl.IsTemplate || fromTpl.Title != "Plant crocuses" || fromTpl.Position <= after.Position || len(fromTpl.LabelIDs) != 1 {
		t.Errorf("card from template = %+v", fromTpl)
	}

	// Others can't copy what they can't see.
	m.mustJSON("bob", "POST", "/api/v1/cards/"+c.ID+"/copy", `{}`, http.StatusNotFound)
	// Or copy into another board's column.
	other := m.createBoard("alice", "Kitchen")
	oc := m.column("alice", other.ID, "Shelf")
	m.mustJSON("alice", "POST", "/api/v1/cards/"+c.ID+"/copy", `{"columnId":"`+oc.ID+`"}`, http.StatusNotFound)
}
