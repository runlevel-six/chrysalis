// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"
	"time"

	"github.com/runlevel-six/chrysalis/internal/service"
)

func (s *server) registerReminders(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/cards/{id}/reminder", s.getReminder)
	mux.HandleFunc("PUT /api/v1/cards/{id}/reminder", s.setReminder)
	mux.HandleFunc("DELETE /api/v1/cards/{id}/reminder", s.clearReminder)
	mux.HandleFunc("GET /api/v1/me/notifications", s.listNotifications)
	mux.HandleFunc("POST /api/v1/me/notifications/read", s.readNotifications)
	mux.HandleFunc("POST /api/v1/me/notifications/dismiss", s.dismissNotification(true))
	mux.HandleFunc("POST /api/v1/notifications/{id}/dismiss", s.dismissNotification(false))
	mux.HandleFunc("POST /api/v1/notifications/{id}/snooze", s.snoozeNotification)
}

func (s *server) getReminder(w http.ResponseWriter, r *http.Request) {
	rem, err := s.boards.Reminder(r.Context(), userID(r), r.PathValue("id"))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reminder": rem})
}

func (s *server) setReminder(w http.ResponseWriter, r *http.Request) {
	var spec service.ReminderSpec
	if !decodeJSON(w, r, &spec) {
		return
	}
	rem, err := s.boards.SetReminder(r.Context(), userID(r), r.PathValue("id"), spec)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reminder": rem})
}

func (s *server) clearReminder(w http.ResponseWriter, r *http.Request) {
	if err := s.boards.ClearReminder(r.Context(), userID(r), r.PathValue("id")); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listNotifications(w http.ResponseWriter, r *http.Request) {
	ns, err := s.boards.Notifications(r.Context(), userID(r))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	unread := 0
	for _, n := range ns {
		if n.ReadAt == nil {
			unread++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": ns, "unread": unread})
}

func (s *server) readNotifications(w http.ResponseWriter, r *http.Request) {
	if err := s.boards.MarkNotificationsRead(r.Context(), userID(r)); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) dismissNotification(all bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if all {
			id = ""
		}
		if err := s.boards.DismissNotification(r.Context(), userID(r), id); err != nil {
			s.serviceError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *server) snoozeNotification(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Until string `json:"until"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	until, err := time.Parse(time.RFC3339Nano, body.Until)
	if err != nil {
		s.serviceError(w, r, &service.ValidationError{Msg: "Choose when to be reminded again."})
		return
	}
	if err := s.boards.SnoozeNotification(r.Context(), userID(r), r.PathValue("id"), until); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
