// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"

	"github.com/runlevel-six/chrysalis/internal/webpush"
)

func (s *server) registerPush(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/push", s.pushInfo)
	mux.HandleFunc("POST /api/v1/me/push-subscriptions", s.pushSubscribe)
	mux.HandleFunc("POST /api/v1/me/push-subscriptions/remove", s.pushUnsubscribe)
	mux.HandleFunc("POST /api/v1/me/push-subscriptions/status", s.pushStatus)
	mux.HandleFunc("POST /api/v1/me/push-test", s.pushTest)
}

// pushInfo tells the app whether push is available and the key to subscribe
// with.
func (s *server) pushInfo(w http.ResponseWriter, r *http.Request) {
	if s.push == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "publicKey": s.push.PublicKey()})
}

func (s *server) pushOff(w http.ResponseWriter) bool {
	if s.push == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Push notifications are turned off on this server."})
		return true
	}
	return false
}

func (s *server) pushSubscribe(w http.ResponseWriter, r *http.Request) {
	if s.pushOff(w) {
		return
	}
	var body struct {
		Subscription webpush.Subscription `json:"subscription"`
		Label        string               `json:"label"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.push.Subscribe(r.Context(), userID(r), body.Subscription, body.Label); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type endpointBody struct {
	Endpoint string `json:"endpoint"`
}

func (s *server) pushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if s.pushOff(w) {
		return
	}
	var body endpointBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.push.Unsubscribe(r.Context(), userID(r), body.Endpoint); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) pushStatus(w http.ResponseWriter, r *http.Request) {
	if s.pushOff(w) {
		return
	}
	var body endpointBody
	if !decodeJSON(w, r, &body) {
		return
	}
	ok, n, err := s.push.Subscribed(r.Context(), userID(r), body.Endpoint)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscribed": ok, "devices": n})
}

func (s *server) pushTest(w http.ResponseWriter, r *http.Request) {
	if s.pushOff(w) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": s.push.Test(r.Context(), userID(r))})
}
