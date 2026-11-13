// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/runlevel-six/chrysalis/internal/service"
)

func TestBoardArchiveTrashRestore(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Garden")
	m.createBoard("alice", "Kitchen")
	m.column("alice", b.ID, "Col")

	list := func(path string) []service.Board {
		var out struct{ Boards []service.Board }
		_ = json.Unmarshal(m.mustJSON("alice", "GET", path, "", http.StatusOK), &out)
		return out.Boards
	}
	names := func(bs []service.Board) (n []string) {
		for _, b := range bs {
			n = append(n, b.Name)
		}
		return n
	}

	// Icon and archive.
	var got service.Board
	_ = json.Unmarshal(m.mustJSON("alice", "PATCH", "/api/v1/boards/"+b.ID, `{"icon":"🦋","archived":true}`, http.StatusOK), &got)
	if got.Icon != "🦋" || !got.Archived {
		t.Fatalf("patched = %+v", got)
	}
	if n := names(list("/api/v1/boards")); len(n) != 1 || n[0] != "Kitchen" {
		t.Errorf("live boards = %v", n)
	}
	if n := names(list("/api/v1/me/boards/archived")); len(n) != 1 || n[0] != "Garden" {
		t.Errorf("archived = %v", n)
	}
	// An archived board still opens.
	m.mustJSON("alice", "GET", "/api/v1/boards/"+b.ID+"/detail", "", http.StatusOK)
	m.mustJSON("alice", "PATCH", "/api/v1/boards/"+b.ID, `{"archived":false}`, http.StatusOK)

	// Bad icons are refused calmly.
	m.mustJSON("alice", "PATCH", "/api/v1/boards/"+b.ID, `{"icon":"not an emoji at all, a sentence"}`, http.StatusBadRequest)

	// Only the owner can delete; others can't even see it.
	m.mustJSON("bob", "DELETE", "/api/v1/boards/"+b.ID, "", http.StatusNotFound)
	m.mustJSON("alice", "DELETE", "/api/v1/boards/"+b.ID, "", http.StatusOK)
	m.mustJSON("alice", "GET", "/api/v1/boards/"+b.ID+"/detail", "", http.StatusNotFound)
	if n := names(list("/api/v1/me/boards/trash")); len(n) != 1 || n[0] != "Garden" {
		t.Errorf("trash = %v", n)
	}
	m.mustJSON("alice", "POST", "/api/v1/boards/"+b.ID+"/restore", "", http.StatusOK)
	var d struct {
		Columns []struct{ Name string } `json:"columns"`
	}
	_ = json.Unmarshal(m.mustJSON("alice", "GET", "/api/v1/boards/"+b.ID+"/detail", "", http.StatusOK), &d)
	if len(d.Columns) != 1 {
		t.Errorf("restored board lost its columns: %+v", d)
	}

	// Purging after the retention period removes it for good.
	m.mustJSON("alice", "DELETE", "/api/v1/boards/"+b.ID, "", http.StatusOK)
	if _, err := service.PurgeTrash(t.Context(), m.st, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n := list("/api/v1/me/boards/trash"); len(n) != 0 {
		t.Errorf("trash after purge = %v", names(n))
	}
	m.mustJSON("alice", "POST", "/api/v1/boards/"+b.ID+"/restore", "", http.StatusNotFound)
}

func TestBoardReorder(t *testing.T) {
	m := newMultiUser(t, testStatic)
	m.createBoard("alice", "A")
	b := m.createBoard("alice", "B")
	m.mustJSON("alice", "PATCH", "/api/v1/boards/"+b.ID, `{"position":"!"}`, http.StatusBadRequest)
	// "Zz" sorts before the first board's "a0".
	m.mustJSON("alice", "PATCH", "/api/v1/boards/"+b.ID, `{"position":"Zz"}`, http.StatusOK)
	var out struct{ Boards []service.Board }
	_ = json.Unmarshal(m.mustJSON("alice", "GET", "/api/v1/boards", "", http.StatusOK), &out)
	if len(out.Boards) != 2 || out.Boards[0].Name != "B" {
		t.Errorf("order = %+v", out.Boards)
	}
}
