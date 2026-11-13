// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Package sse fans change notifications out to connected browsers
// (docs/DESIGN.md §5.5). Events are small notices ("card X on board B
// changed"), not the data itself: clients refetch what they need, which keeps
// authorization in one place (the normal API) and events cheap.
//
// Each event gets an increasing ID. A client that reconnects with
// Last-Event-ID gets everything it missed from a short in-memory history, or
// a "resync" telling it to refetch, if the gap is too large or the server
// restarted.
package sse

import (
	"sync"
)

// Event is one change notice.
type Event struct {
	ID       uint64 `json:"id"`
	Type     string `json:"type"`              // "change", or "resync" (refetch everything)
	BoardID  string `json:"boardId,omitempty"` // set for board content changes
	UserID   string `json:"-"`                 // set for a user's own channel (view state, prefs)
	Kind     string `json:"kind,omitempty"`    // board, column, card, label, checklist, description, attachment, view, prefs
	EntityID string `json:"entityId,omitempty"`
	CardID   string `json:"cardId,omitempty"` // for things that belong to a card
	// Origin is the browser tab that made the change (from X-Chrysalis-Client),
	// so it can skip its own echoes; it already shows the change.
	Origin string `json:"origin,omitempty"`
}

// Subscription is one connected client: the boards it shows (none, one,
// or several for the calendar) and the user's own channel.
type Subscription struct {
	Boards map[string]bool
	UserID string
	C      chan Event
}

const (
	historySize = 2048
	bufferSize  = 64
)

// Hub is safe for concurrent use.
type Hub struct {
	mu      sync.Mutex
	nextID  uint64
	history []Event // ring of recent events, oldest first
	subs    map[*Subscription]struct{}
	closed  bool
}

func NewHub() *Hub {
	return &Hub{nextID: 1, subs: map[*Subscription]struct{}{}}
}

func (s *Subscription) wants(e Event) bool {
	return (e.BoardID != "" && s.Boards[e.BoardID]) || (e.UserID != "" && e.UserID == s.UserID)
}

// Publish records an event and delivers it to matching subscribers. A
// subscriber too slow to keep up gets a resync instead of blocking anyone.
func (h *Hub) Publish(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	e.ID = h.nextID
	h.nextID++
	if e.Type == "" {
		e.Type = "change"
	}
	h.history = append(h.history, e)
	if len(h.history) > historySize {
		h.history = h.history[len(h.history)-historySize:]
	}
	for s := range h.subs {
		if !s.wants(e) {
			continue
		}
		select {
		case s.C <- e:
		default:
			h.dropLocked(s, true)
		}
	}
}

// Subscribe registers a client. lastID is the Last-Event-ID it reconnected
// with (0 for a fresh connection); missed events are queued first, or a
// resync if they're no longer all in the history.
func (h *Hub) Subscribe(boardIDs []string, userID string, lastID uint64) *Subscription {
	boards := make(map[string]bool, len(boardIDs))
	for _, id := range boardIDs {
		if id != "" {
			boards[id] = true
		}
	}
	s := &Subscription{Boards: boards, UserID: userID, C: make(chan Event, bufferSize)}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		close(s.C)
		return s
	}
	if lastID > 0 {
		var missed []Event
		complete := lastID+1 >= h.oldestLocked() && lastID < h.nextID
		for _, e := range h.history {
			if e.ID > lastID && s.wants(e) {
				missed = append(missed, e)
			}
		}
		if !complete || len(missed) > bufferSize-1 {
			s.C <- Event{ID: h.nextID - 1, Type: "resync"}
		} else {
			for _, e := range missed {
				s.C <- e
			}
		}
	}
	h.subs[s] = struct{}{}
	return s
}

func (h *Hub) oldestLocked() uint64 {
	if len(h.history) == 0 {
		return h.nextID
	}
	return h.history[0].ID
}

// Unsubscribe removes a client and closes its channel.
func (h *Hub) Unsubscribe(s *Subscription) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dropLocked(s, false)
}

func (h *Hub) dropLocked(s *Subscription, resync bool) {
	if _, ok := h.subs[s]; !ok {
		return
	}
	delete(h.subs, s)
	if resync {
		// Make room for one resync so the client knows to refetch.
		select {
		case <-s.C:
		default:
		}
		s.C <- Event{ID: h.nextID - 1, Type: "resync"}
	}
	close(s.C)
}

// Close ends every subscription (server shutdown).
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for s := range h.subs {
		h.dropLocked(s, false)
	}
}
