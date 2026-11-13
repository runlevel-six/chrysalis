// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package sse

import "testing"

func drain(s *Subscription) []Event {
	var out []Event
	for {
		select {
		case e, ok := <-s.C:
			if !ok {
				return out
			}
			out = append(out, e)
		default:
			return out
		}
	}
}

func TestDeliversOnlyMatchingEvents(t *testing.T) {
	h := NewHub()
	alice := h.Subscribe([]string{"board-a"}, "alice", 0)
	bob := h.Subscribe([]string{"board-b"}, "bob", 0)
	h.Publish(Event{BoardID: "board-a", Kind: "card", EntityID: "c1"})
	h.Publish(Event{BoardID: "board-b", Kind: "card", EntityID: "c2"})
	h.Publish(Event{UserID: "alice", Kind: "prefs"})
	a, b := drain(alice), drain(bob)
	if len(a) != 2 || a[0].EntityID != "c1" || a[1].Kind != "prefs" {
		t.Fatalf("alice got %+v", a)
	}
	if len(b) != 1 || b[0].EntityID != "c2" {
		t.Fatalf("bob got %+v", b)
	}
	if a[0].ID >= a[1].ID || a[0].Type != "change" {
		t.Errorf("ids not increasing or type unset: %+v", a)
	}
}

func TestReplayOnReconnect(t *testing.T) {
	h := NewHub()
	s := h.Subscribe([]string{"b"}, "u", 0)
	h.Publish(Event{BoardID: "b", EntityID: "1"})
	first := drain(s)[0]
	h.Unsubscribe(s)
	h.Publish(Event{BoardID: "b", EntityID: "2"})
	h.Publish(Event{BoardID: "other", EntityID: "x"})
	h.Publish(Event{BoardID: "b", EntityID: "3"})

	s = h.Subscribe([]string{"b"}, "u", first.ID)
	got := drain(s)
	if len(got) != 2 || got[0].EntityID != "2" || got[1].EntityID != "3" {
		t.Fatalf("replay = %+v", got)
	}
}

func TestResyncWhenHistoryIsGone(t *testing.T) {
	h := NewHub()
	for range historySize + 10 {
		h.Publish(Event{BoardID: "b"})
	}
	s := h.Subscribe([]string{"b"}, "u", 1) // event 2.. long gone
	got := drain(s)
	if len(got) != 1 || got[0].Type != "resync" {
		t.Fatalf("got %+v, want one resync", got)
	}
	// An id from the future (the server restarted and counts from 1 again).
	s2 := h.Subscribe([]string{"b"}, "u", 1<<40)
	if got := drain(s2); len(got) != 1 || got[0].Type != "resync" {
		t.Fatalf("future id: %+v", got)
	}
}

func TestSlowSubscriberGetsResyncNotBlocking(t *testing.T) {
	h := NewHub()
	slow := h.Subscribe([]string{"b"}, "u", 0)
	for range bufferSize * 2 {
		h.Publish(Event{BoardID: "b"}) // must not block
	}
	got := drain(slow)
	if len(got) == 0 || got[len(got)-1].Type != "resync" {
		t.Fatalf("slow subscriber should end with a resync, got %d events", len(got))
	}
	if _, ok := <-slow.C; ok {
		t.Error("slow subscriber's channel should be closed")
	}
}

func TestCloseEndsSubscriptions(t *testing.T) {
	h := NewHub()
	s := h.Subscribe([]string{"b"}, "u", 0)
	h.Close()
	if _, ok := <-s.C; ok {
		t.Error("channel should close")
	}
	h.Publish(Event{BoardID: "b"}) // no panic after close
	if s2 := h.Subscribe([]string{"b"}, "u", 0); func() bool { _, ok := <-s2.C; return ok }() {
		t.Error("subscribing after close should give a closed channel")
	}
}
