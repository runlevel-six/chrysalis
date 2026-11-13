// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/runlevel-six/chrysalis/internal/service"
	"github.com/runlevel-six/chrysalis/internal/trello"
)

func (m *multiUser) importTrello(user string, data []byte) *httptest.ResponseRecorder {
	m.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "board.json")
	_, _ = fw.Write(data)
	_ = mw.Close()
	r := httptest.NewRequest("POST", "/api/v1/import/trello", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Remote-User", user)
	w := httptest.NewRecorder()
	m.h.ServeHTTP(w, r)
	return w
}

type importResult struct {
	BoardID string        `json:"boardId"`
	Board   service.Board `json:"board"`
	Report  trello.Report `json:"report"`
}

func gardenExport(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "trello", "testdata", "garden-board.json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestImportTrello(t *testing.T) {
	m := newMultiUser(t, testStatic)
	m.createBoard("alice", "Existing board")

	w := m.importTrello("alice", gardenExport(t))
	if w.Code != http.StatusCreated {
		t.Fatalf("import: %d %s", w.Code, w.Body)
	}
	res := decode[importResult](t, w)
	want := service.ImportCounts{Columns: 5, Cards: 6, Descriptions: 3, Labels: 7, Checklists: 3, ChecklistItems: 5, Comments: 3, Links: 3}
	if res.Report.Imported != want {
		t.Errorf("counts = %+v, want %+v", res.Report.Imported, want)
	}
	if len(res.Report.Skipped) == 0 || len(res.Report.Notes) == 0 {
		t.Errorf("report = %+v", res.Report)
	}
	if res.Board.Name != "Garden Plans" || res.Board.Role != service.RoleOwner || res.Board.Background != (service.Background{Type: "color", Value: "#519839"}) {
		t.Errorf("board = %+v", res.Board)
	}

	// A new board, after the existing one; never merged into it.
	boards := decode[struct{ Boards []service.Board }](t, m.as("alice", "GET", "/api/v1/boards", ""))
	if len(boards.Boards) != 2 || boards.Boards[1].ID != res.BoardID {
		t.Fatalf("boards = %+v", boards.Boards)
	}

	d := m.detail("alice", res.BoardID)
	var colNames []string
	cols := map[string]string{}
	for _, c := range d.Columns {
		colNames = append(colNames, c.Name)
		cols[c.Name] = c.ID
	}
	if strings.Join(colNames, "|") != "Last year|Ideas|To plant|Growing|Harvested" {
		t.Errorf("columns = %v", colNames)
	}
	if !d.View.Columns[cols["Last year"]] || d.View.Columns[cols["Ideas"]] {
		t.Errorf("collapsed columns = %v", d.View.Columns)
	}
	for col, want := range map[string][]string{
		"Last year": nil, "Ideas": {"Peas along the fence"}, "To plant": {"Sow basil", "Start tomato seeds"},
		"Growing": nil, "Harvested": {"Build raised beds"},
	} {
		if got := titles(d.Cards, cols[col]); !eq(got, want) {
			t.Errorf("%s cards = %v, want %v", col, got, want)
		}
	}
	if len(d.Labels) != 7 {
		t.Errorf("labels = %+v", d.Labels)
	}

	archived := decode[struct{ Cards []service.Card }](t, m.as("alice", "GET", "/api/v1/boards/"+res.BoardID+"/archived", ""))
	var archivedTitles []string
	for _, c := range archived.Cards {
		archivedTitles = append(archivedTitles, c.Title)
	}
	if len(archivedTitles) != 2 || !strings.Contains(strings.Join(archivedTitles, "|"), "Plant garlic") || !strings.Contains(strings.Join(archivedTitles, "|"), "Turn the compost") {
		t.Errorf("archived = %v", archivedTitles)
	}

	var tomato, beds service.Card
	for _, c := range d.Cards {
		switch c.Title {
		case "Start tomato seeds":
			tomato = c
		case "Build raised beds":
			beds = c
		}
	}
	if tomato.DueAt == nil || *tomato.DueAt != "2026-03-15T17:00:00.000Z" || !tomato.DueHasTime ||
		tomato.StartAt == nil || *tomato.StartAt != "2026-03-01" || tomato.Cover != (service.Cover{Type: "color", Value: "red"}) ||
		len(tomato.LabelIDs) != 2 || !tomato.HasDescription || tomato.ChecklistDone != 2 || tomato.ChecklistTotal != 4 {
		t.Errorf("tomato card = %+v", tomato)
	}
	if beds.CompletedAt == nil {
		t.Errorf("beds should be completed: %+v", beds)
	}

	lists := m.checklists("alice", tomato.ID)
	if len(lists) != 2 || lists[0].Title != "Steps" || !eq(itemTexts(lists[1]), []string{"Grow light", "Seed trays", "Potting mix"}) ||
		lists[1].Items[0].DoneAt == nil || lists[1].Items[2].DoneAt != nil {
		t.Errorf("checklists = %+v", lists)
	}

	desc := decode[service.Description](t, m.as("alice", "GET", "/api/v1/cards/"+tomato.ID+"/description", ""))
	if !strings.Contains(string(desc.Doc), `"type":"heading"`) || desc.Version != 1 {
		t.Errorf("description = %s", desc.Doc)
	}

	comments := decode[struct{ Comments []service.Comment }](t, m.as("alice", "GET", "/api/v1/cards/"+tomato.ID+"/comments", ""))
	if len(comments.Comments) != 2 {
		t.Fatalf("comments = %+v", comments)
	}
	first := comments.Comments[0]
	if first.CreatedAt != "2026-03-01T08:00:00.000Z" || !strings.Contains(string(first.Body), "Originally by bob:") || first.AuthorName == "" {
		t.Errorf("first comment = %+v %s", first, first.Body)
	}

	atts := decode[struct{ Attachments []service.Attachment }](t, m.as("alice", "GET", "/api/v1/cards/"+tomato.ID+"/attachments", ""))
	if len(atts.Attachments) != 2 || atts.Attachments[0].Kind != "link" {
		t.Errorf("attachments = %+v", atts)
	}

	// Only the importer can see the new board.
	if w := m.as("bob", "GET", "/api/v1/boards/"+res.BoardID+"/detail", ""); w.Code != http.StatusNotFound {
		t.Errorf("bob detail = %d", w.Code)
	}
	if w := m.as("bob", "GET", "/api/v1/cards/"+tomato.ID+"/comments", ""); w.Code != http.StatusNotFound {
		t.Errorf("bob comments = %d", w.Code)
	}
	if bb := decode[struct{ Boards []service.Board }](t, m.as("bob", "GET", "/api/v1/boards", "")); len(bb.Boards) != 0 {
		t.Errorf("bob's boards = %+v", bb.Boards)
	}

	// Importing again makes a second, separate board.
	again := decode[importResult](t, m.importTrello("alice", gardenExport(t)))
	if again.BoardID == res.BoardID {
		t.Error("second import reused the board")
	}
}

func TestImportTrelloMalformed(t *testing.T) {
	m := newMultiUser(t, testStatic)
	for name, data := range map[string][]byte{
		"not json":     []byte("this is not a board"),
		"other json":   []byte(`{"title":"a different app's export","items":[]}`),
		"truncated":    gardenExport(t)[:500],
		"wrong shapes": []byte(`{"name":"x","lists":[{"id":1}],"cards":[]}`),
	} {
		w := m.importTrello("alice", data)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
			continue
		}
		var body struct{ Error string }
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if !strings.Contains(body.Error, "doesn't look like a Trello board export") {
			t.Errorf("%s: error = %q", name, body.Error)
		}
	}
	if w := m.as("alice", "POST", "/api/v1/import/trello", `{"name":"x"}`); w.Code != http.StatusBadRequest {
		t.Errorf("non-multipart = %d", w.Code)
	}
	if w := m.as("", "POST", "/api/v1/import/trello", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("signed out = %d", w.Code)
	}
	if bb := decode[struct{ Boards []service.Board }](t, m.as("alice", "GET", "/api/v1/boards", "")); len(bb.Boards) != 0 {
		t.Errorf("failed imports left boards: %+v", bb.Boards)
	}
}
