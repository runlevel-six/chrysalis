// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/runlevel-six/chrysalis/internal/auth"
	"github.com/runlevel-six/chrysalis/internal/service"
)

// clientID matches the per-tab id browsers send in X-Chrysalis-Client.
var clientID = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

// withOrigin puts the request's tab id in the context, so the change
// notices it causes can be marked and that tab can skip them.
func withOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := r.Header.Get("X-Chrysalis-Client"); clientID.MatchString(id) {
			r = r.WithContext(service.WithOrigin(r.Context(), id))
		}
		next.ServeHTTP(w, r)
	})
}

const heartbeat = 25 * time.Second

// maxStreamBoards bounds how many boards one stream follows (the calendar
// across every board is the widest case).
const maxStreamBoards = 100

// events streams change notices (server-sent events, docs/DESIGN.md §5.5):
// for each ?board= given that the user can see, and always the user's own
// channel (their view state and preferences on other devices).
func (s *server) events(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	boardIDs := r.URL.Query()["board"]
	if len(boardIDs) > maxStreamBoards {
		auth.WriteError(w, http.StatusBadRequest, "too many boards for one stream")
		return
	}
	for _, id := range boardIDs {
		if _, err := s.boards.Get(r.Context(), u.ID, id); err != nil {
			s.serviceError(w, r, err)
			return
		}
	}
	rc := http.NewResponseController(w)

	// Browsers resend the last id they saw when they reconnect.
	lastID, _ := strconv.ParseUint(r.Header.Get("Last-Event-ID"), 10, 64)
	sub := s.hub.Subscribe(boardIDs, u.ID, lastID)
	defer s.hub.Unsubscribe(sub)

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no") // tell proxies not to buffer the stream
	w.WriteHeader(http.StatusOK)
	// Retry after 3 s if the connection drops, then say hello.
	if _, err := fmt.Fprint(w, "retry: 3000\n: connected\n\n"); err != nil {
		return
	}
	_ = rc.Flush()

	tick := time.NewTicker(heartbeat)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			// A comment line keeps idle proxies from closing the stream.
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			_ = rc.Flush()
		case e, ok := <-sub.C:
			if !ok {
				return // server shutting down, or this client fell behind (it got a resync first)
			}
			data, _ := json.Marshal(e)
			if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.ID, e.Type, data); err != nil {
				return
			}
			_ = rc.Flush()
		}
	}
}
