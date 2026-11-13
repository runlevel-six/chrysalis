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

func TestReminderRoutes(t *testing.T) {
	m := newMultiUser(t, testStatic)
	b := m.createBoard("alice", "Home")
	col := m.column("alice", b.ID, "To do")
	var c service.Card
	_ = json.Unmarshal(m.mustJSON("alice", "POST", "/api/v1/columns/"+col.ID+"/cards", `{"title":"Call the clinic"}`, http.StatusCreated), &c)
	m.mustJSON("alice", "PATCH", "/api/v1/cards/"+c.ID, `{"dueAt":"2030-01-02"}`, http.StatusOK)

	var got struct{ Reminder *service.Reminder }
	_ = json.Unmarshal(m.mustJSON("alice", "GET", "/api/v1/cards/"+c.ID+"/reminder", "", http.StatusOK), &got)
	if got.Reminder != nil {
		t.Fatalf("no reminder yet, got %+v", got.Reminder)
	}
	_ = json.Unmarshal(m.mustJSON("alice", "PUT", "/api/v1/cards/"+c.ID+"/reminder",
		`{"kind":"before","offsetMinutes":60,"timeZone":"Europe/London"}`, http.StatusOK), &got)
	if got.Reminder == nil || got.Reminder.RemindAt == nil || *got.Reminder.RemindAt != "2030-01-02T08:00:00.000Z" {
		t.Fatalf("reminder = %+v", got.Reminder)
	}
	m.mustJSON("alice", "PUT", "/api/v1/cards/"+c.ID+"/reminder", `{"kind":"before","offsetMinutes":-5}`, http.StatusBadRequest)

	// Someone who can't see the card can't set or read a reminder on it.
	m.mustJSON("bob", "PUT", "/api/v1/cards/"+c.ID+"/reminder", `{"kind":"due"}`, http.StatusNotFound)
	m.mustJSON("bob", "GET", "/api/v1/cards/"+c.ID+"/reminder", "", http.StatusNotFound)

	m.mustJSON("alice", "DELETE", "/api/v1/cards/"+c.ID+"/reminder", "", http.StatusNoContent)

	var list struct {
		Notifications []service.Notification
		Unread        int
	}
	_ = json.Unmarshal(m.mustJSON("alice", "GET", "/api/v1/me/notifications", "", http.StatusOK), &list)
	if len(list.Notifications) != 0 || list.Unread != 0 {
		t.Errorf("list = %+v", list)
	}
	m.mustJSON("alice", "POST", "/api/v1/me/notifications/read", "", http.StatusNoContent)
	m.mustJSON("alice", "POST", "/api/v1/notifications/nope/dismiss", "", http.StatusNotFound)
	soon := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	m.mustJSON("bob", "POST", "/api/v1/notifications/nope/snooze", `{"until":"`+soon+`"}`, http.StatusNotFound)

	// Preferences carry the new settings without losing the others.
	m.mustJSON("alice", "PATCH", "/api/v1/me/prefs", `{"theme":"butterfly-garden"}`, http.StatusOK)
	var prefs service.Prefs
	_ = json.Unmarshal(m.mustJSON("alice", "PATCH", "/api/v1/me/prefs", `{"remindersEnabled":true,"timeZone":"America/Chicago"}`, http.StatusOK), &prefs)
	if !prefs.RemindersEnabled || prefs.TimeZone != "America/Chicago" || prefs.Theme != "butterfly-garden" {
		t.Errorf("prefs = %+v", prefs)
	}
	m.mustJSON("alice", "PATCH", "/api/v1/me/prefs", `{"timeZone":"Mars/Olympus"}`, http.StatusBadRequest)
}
