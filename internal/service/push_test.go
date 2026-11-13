// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/runlevel-six/chrysalis/internal/webpush"
)

type fakeSender struct {
	mu   sync.Mutex
	sent []string // endpoint + " " + body
	gone map[string]bool
}

func (f *fakeSender) Send(_ context.Context, s webpush.Subscription, body []byte, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gone[s.Endpoint] {
		return webpush.ErrGone
	}
	f.sent = append(f.sent, s.Endpoint+" "+string(body))
	return nil
}

func sub(endpoint string) webpush.Subscription {
	var s webpush.Subscription
	s.Endpoint = endpoint
	s.Keys.P256dh = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4" // betterleaks:allow (RFC 8291 Appendix A example value, published)
	s.Keys.Auth = "BTBZMqHH6r4Tts7J_aSIgg"                                                                    // betterleaks:allow (RFC 8291 Appendix A example value, published)
	return s
}

func TestPushDeliversRemindersExceptInQuietHours(t *testing.T) {
	f := newReminders(t)
	ctx := context.Background()
	keys, err := VAPIDKeys(ctx, f.b.st, "")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := VAPIDKeys(ctx, f.b.st, "")
	if again.PublicString() != keys.PublicString() {
		t.Fatal("the VAPID key must be made once and kept")
	}
	fs := &fakeSender{gone: map[string]bool{"https://push.example.net/old": true}}
	p := NewPush(f.b.st, func() time.Time { return f.now }, keys, fs, "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	f.b.SetPusher(p.Pusher())

	if err := p.Subscribe(ctx, f.alice, sub("https://push.example.net/phone"), "Phone"); err != nil {
		t.Fatal(err)
	}
	if err := p.Subscribe(ctx, f.alice, sub("https://push.example.net/old"), "Old laptop"); err != nil {
		t.Fatal(err)
	}
	if err := p.Subscribe(ctx, f.alice, sub("http://push.example.net/plain"), ""); err == nil {
		t.Error("an http endpoint was accepted")
	}

	f.enable(t, "America/Chicago")
	f.due(t, CardPatch{DueAt: set("2026-11-14")})
	if _, err := f.b.SetReminder(ctx, f.alice, f.card.ID, ReminderSpec{Kind: "due", TimeZone: "America/Chicago"}); err != nil {
		t.Fatal(err)
	}
	f.now = time.Date(2026, 11, 14, 15, 0, 0, 0, time.UTC) // 9:00 Chicago
	f.fire(t)
	p.Wait()
	if len(fs.sent) != 1 {
		t.Fatalf("sent %v", fs.sent)
	}
	var m message
	_ = json.Unmarshal([]byte(fs.sent[0][len("https://push.example.net/phone "):]), &m)
	if m.Title != "Chrysalis" || m.Body != "Heads up: “Call the clinic” is due today." || m.URL == "/" {
		t.Errorf("message %+v", m)
	}
	if ok, n, _ := p.Subscribed(ctx, f.alice, "https://push.example.net/old"); ok || n != 1 {
		t.Errorf("a gone subscription should be forgotten (ok=%v n=%d)", ok, n)
	}

	// Quiet hours 22:00–07:00: a reminder at 23:30 Chicago stays in the app.
	qs, qe := "22:00", "07:00"
	if _, err := f.p.Update(ctx, f.alice, PrefsPatch{QuietStart: &qs, QuietEnd: &qe}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 15, 5, 30, 0, 0, time.UTC) // 23:30 Chicago
	if _, err := f.b.SetReminder(ctx, f.alice, f.card.ID, ReminderSpec{Kind: "at", At: at.Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	f.now = at
	ns := f.fire(t)
	p.Wait()
	if len(fs.sent) != 1 || len(ns) != 2 {
		t.Fatalf("quiet hours: sent %d, listed %d", len(fs.sent), len(ns))
	}
	bad := "25:00"
	if _, err := f.p.Update(ctx, f.alice, PrefsPatch{QuietStart: &bad}); err == nil {
		t.Error("a bad time was accepted")
	}

	if n := p.Test(ctx, f.alice); n != 1 {
		t.Errorf("test reached %d devices", n)
	}
	if err := p.Unsubscribe(ctx, f.alice, "https://push.example.net/phone"); err != nil {
		t.Fatal(err)
	}
	if n := p.Test(ctx, f.alice); n != 0 {
		t.Errorf("after unsubscribing, test reached %d", n)
	}
}

func TestQuietHours(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 1, 1, h, m, 0, 0, time.UTC) }
	cases := []struct {
		start, end string
		t          time.Time
		want       bool
	}{
		{"22:00", "07:00", at(23, 0), true},
		{"22:00", "07:00", at(6, 59), true},
		{"22:00", "07:00", at(7, 0), false},
		{"13:00", "14:00", at(13, 30), true},
		{"13:00", "14:00", at(14, 0), false},
		{"", "07:00", at(3, 0), false},
	}
	for _, c := range cases {
		if got := quiet(c.start, c.end, c.t, time.UTC); got != c.want {
			t.Errorf("%s–%s at %s = %v", c.start, c.end, c.t.Format("15:04"), got)
		}
	}
}
