// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/runlevel-six/chrysalis/internal/sse"
)

// stream opens /api/v1/events on a real test server and returns decoded
// events as they arrive.
func stream(t *testing.T, srv *httptest.Server, user, query, lastID string) (<-chan sse.Event, *http.Response) {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/events"+query, nil)
	req.Header.Set("Remote-User", user)
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	out := make(chan sse.Event, 16)
	if res.StatusCode != http.StatusOK {
		close(out)
		return out, res
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	go func() {
		defer close(out)
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				var e sse.Event
				_ = json.Unmarshal([]byte(data), &e)
				out <- e
			}
		}
	}()
	return out, res
}

func next(t *testing.T, c <-chan sse.Event) sse.Event {
	t.Helper()
	select {
	case e, ok := <-c:
		if !ok {
			t.Fatal("stream ended")
		}
		return e
	case <-time.After(3 * time.Second):
		t.Fatal("no event within 3s")
	}
	return sse.Event{}
}

func TestLiveEvents(t *testing.T) {
	m := newMultiUser(t, testStatic)
	srv := httptest.NewServer(m.h)
	// Registered first, so it runs last: after the streams' bodies close.
	t.Cleanup(srv.Close)
	b := m.createBoard("alice", "Board")
	col := m.column("alice", b.ID, "Col")

	events, _ := stream(t, srv, "alice", "?board="+b.ID, "")

	// A change from one tab reaches the stream, marked with that tab.
	r := httptest.NewRequest("POST", "/api/v1/columns/"+col.ID+"/cards", strings.NewReader(`{"title":"Live"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Remote-User", "alice")
	r.Header.Set("X-Chrysalis-Client", "tab-one-1234")
	w := httptest.NewRecorder()
	m.h.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create card: %d", w.Code)
	}
	e := next(t, events)
	if e.Type != "change" || e.Kind != "card" || e.BoardID != b.ID || e.Origin != "tab-one-1234" || e.CardID == "" {
		t.Fatalf("event = %+v", e)
	}

	// View state goes to the user's own channel, not the board.
	m.mustJSON("alice", "PUT", "/api/v1/boards/"+b.ID+"/view", `{"hideCompleted":true}`, http.StatusNoContent)
	if v := next(t, events); v.Kind != "view" || v.BoardID != "" {
		t.Fatalf("view event = %+v", v)
	}

	// Reconnecting with the last id seen replays what was missed.
	m.mustJSON("alice", "PATCH", "/api/v1/columns/"+col.ID, `{"name":"Renamed"}`, http.StatusOK)
	missed := next(t, events)
	replayed, _ := stream(t, srv, "alice", "?board="+b.ID, itoa(e.ID))
	got := next(t, replayed)
	if got.Kind != "view" {
		t.Fatalf("first replayed = %+v", got)
	}
	if got := next(t, replayed); got.ID != missed.ID || got.Kind != "column" {
		t.Fatalf("second replayed = %+v, want %+v", got, missed)
	}
}

func TestEventsAuthorization(t *testing.T) {
	m := newMultiUser(t, testStatic)
	srv := httptest.NewServer(m.h)
	// Registered first, so it runs last: after the streams' bodies close.
	t.Cleanup(srv.Close)
	b := m.createBoard("alice", "Private")

	if _, res := stream(t, srv, "mallory", "?board="+b.ID, ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("non-member stream: %d", res.StatusCode)
	}
	// Mallory's own channel works, and never sees Alice's board.
	events, res := stream(t, srv, "mallory", "", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("own channel: %d", res.StatusCode)
	}
	m.mustJSON("alice", "PATCH", "/api/v1/boards/"+b.ID, `{"name":"Still private"}`, http.StatusOK)
	select {
	case e := <-events:
		t.Fatalf("mallory got %+v", e)
	case <-time.After(300 * time.Millisecond):
	}
}

func itoa(n uint64) string { return strconv.FormatUint(n, 10) }
