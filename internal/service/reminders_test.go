// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/runlevel-six/chrysalis/internal/sse"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

type reminderFixture struct {
	b      *Boards
	p      *PrefsService
	alice  string
	card   Card
	now    time.Time
	events []sse.Event
	pushed []Notification
}

func newReminders(t *testing.T) *reminderFixture {
	t.Helper()
	st, alice := newImportStore(t)
	f := &reminderFixture{alice: alice, now: time.Date(2026, 11, 13, 15, 0, 0, 0, time.UTC)} // Friday
	f.b = NewBoards(st, func() time.Time { return f.now })
	f.b.SetNotifier(func(e sse.Event) { f.events = append(f.events, e) })
	f.b.SetPusher(func(_ context.Context, _ string, n Notification) { f.pushed = append(f.pushed, n) })
	f.p = NewPrefs(st, f.b, func(string) bool { return true })
	ctx := context.Background()
	board, err := f.b.Create(ctx, alice, "Home")
	if err != nil {
		t.Fatal(err)
	}
	col, err := f.b.CreateColumn(ctx, alice, board.ID, "To do", nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.card, err = f.b.CreateCard(ctx, alice, col.ID, "Call the clinic", nil); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *reminderFixture) enable(t *testing.T, tz string) {
	t.Helper()
	on := true
	if _, err := f.p.Update(context.Background(), f.alice, PrefsPatch{RemindersEnabled: &on, TimeZone: &tz}); err != nil {
		t.Fatal(err)
	}
}

func (f *reminderFixture) due(t *testing.T, patch CardPatch) {
	t.Helper()
	var err error
	if f.card, err = f.b.UpdateCard(context.Background(), f.alice, f.card.ID, patch); err != nil {
		t.Fatal(err)
	}
}

func (f *reminderFixture) fire(t *testing.T) []Notification {
	t.Helper()
	if _, err := f.b.FireReminders(context.Background()); err != nil {
		t.Fatal(err)
	}
	ns, err := f.b.Notifications(context.Background(), f.alice)
	if err != nil {
		t.Fatal(err)
	}
	return ns
}

func set[T any](v T) Nullable[T] { return Nullable[T]{Set: true, Value: &v} }

func TestReminderOffUntilEnabled(t *testing.T) {
	f := newReminders(t)
	ctx := context.Background()
	f.due(t, CardPatch{DueAt: set("2026-11-14")})
	if _, err := f.b.SetReminder(ctx, f.alice, f.card.ID, ReminderSpec{Kind: "due", TimeZone: "America/Chicago"}); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(48 * time.Hour)
	if ns := f.fire(t); len(ns) != 0 {
		t.Fatalf("reminders are off by default, got %+v", ns)
	}
}

func TestReminderDateOnlyGoesOffAtNineLocal(t *testing.T) {
	f := newReminders(t)
	ctx := context.Background()
	f.enable(t, "America/Chicago")
	f.due(t, CardPatch{DueAt: set("2026-11-14")})
	r, err := f.b.SetReminder(ctx, f.alice, f.card.ID, ReminderSpec{Kind: "before", OffsetMinutes: 24 * 60, TimeZone: "America/Chicago"})
	if err != nil {
		t.Fatal(err)
	}
	// A day before 9:00 Chicago (UTC-6) on the 14th: 15:00 UTC on the 13th.
	if r.RemindAt == nil || *r.RemindAt != "2026-11-13T15:00:00.000Z" {
		t.Fatalf("remindAt = %v", r.RemindAt)
	}
	f.now = time.Date(2026, 11, 13, 14, 59, 0, 0, time.UTC)
	if ns := f.fire(t); len(ns) != 0 {
		t.Fatalf("too early: %+v", ns)
	}
	f.now = time.Date(2026, 11, 13, 15, 0, 30, 0, time.UTC)
	ns := f.fire(t)
	if len(ns) != 1 || ns[0].Text != "Heads up: “Call the clinic” is due tomorrow." || ns[0].ReadAt != nil {
		t.Fatalf("notifications = %+v", ns)
	}
	if len(f.pushed) != 1 {
		t.Errorf("pushed %d", len(f.pushed))
	}
	// Once only.
	if ns := f.fire(t); len(ns) != 1 {
		t.Fatalf("fired again: %+v", ns)
	}
}

func TestReminderFollowsTheDueDateAndTime(t *testing.T) {
	f := newReminders(t)
	ctx := context.Background()
	f.enable(t, "UTC")
	f.due(t, CardPatch{DueAt: set("2026-11-20T17:30:00Z"), DueHasTime: ptr(true)})
	if _, err := f.b.SetReminder(ctx, f.alice, f.card.ID, ReminderSpec{Kind: "due", TimeZone: "UTC"}); err != nil {
		t.Fatal(err)
	}
	f.due(t, CardPatch{DueAt: set("2026-11-21T08:00:00Z")})
	r, _ := f.b.Reminder(ctx, f.alice, f.card.ID)
	if r == nil || r.RemindAt == nil || *r.RemindAt != "2026-11-21T08:00:00.000Z" {
		t.Fatalf("after moving the due date, remindAt = %+v", r)
	}
	f.now = time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)
	ns := f.fire(t)
	if len(ns) != 1 || !strings.Contains(ns[0].Text, "is due today at 8:00 AM") {
		t.Fatalf("%+v", ns)
	}
	// Moving the due date later re-arms it.
	f.due(t, CardPatch{DueAt: set("2026-11-22T08:00:00Z")})
	if r, _ := f.b.Reminder(ctx, f.alice, f.card.ID); r == nil || r.FiredAt != nil || r.RemindAt == nil {
		t.Fatalf("not re-armed: %+v", r)
	}
	// No due date: a due-relative reminder waits.
	f.due(t, CardPatch{DueAt: Nullable[string]{Set: true}})
	if r, _ := f.b.Reminder(ctx, f.alice, f.card.ID); r == nil || r.RemindAt != nil {
		t.Fatalf("without a due date: %+v", r)
	}
}

func TestReminderSkipsFinishedCardsAndSnoozes(t *testing.T) {
	f := newReminders(t)
	ctx := context.Background()
	f.enable(t, "UTC")
	at := f.now.Add(time.Hour).Format(time.RFC3339)
	if _, err := f.b.SetReminder(ctx, f.alice, f.card.ID, ReminderSpec{Kind: "at", At: at}); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(2 * time.Hour)
	ns := f.fire(t)
	if len(ns) != 1 || ns[0].Text != "A reminder about “Call the clinic”." {
		t.Fatalf("%+v", ns)
	}

	// Snooze an hour: off the list, back in an hour.
	if err := f.b.SnoozeNotification(ctx, f.alice, ns[0].ID, f.now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if ns := f.fire(t); len(ns) != 0 {
		t.Fatalf("still listed after snoozing: %+v", ns)
	}
	f.now = f.now.Add(61 * time.Minute)
	if ns := f.fire(t); len(ns) != 1 {
		t.Fatalf("snooze didn't come back: %+v", ns)
	}
	if err := f.b.SnoozeNotification(ctx, f.alice, ns[0].ID, f.now.Add(-time.Minute)); err == nil {
		t.Error("snoozing into the past should be refused")
	}

	// A finished card doesn't nag.
	ns = f.fire(t)
	if err := f.b.SnoozeNotification(ctx, f.alice, ns[0].ID, f.now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	f.due(t, CardPatch{Completed: ptr(true)})
	f.now = f.now.Add(2 * time.Hour)
	if ns := f.fire(t); len(ns) != 0 {
		t.Fatalf("a done card reminded: %+v", ns)
	}

	if err := f.b.MarkNotificationsRead(ctx, f.alice); err != nil {
		t.Fatal(err)
	}
	if err := f.b.DismissNotification(ctx, f.alice, ""); err != nil {
		t.Fatal(err)
	}
}

func TestReminderText(t *testing.T) {
	f := newReminders(t)
	f.enable(t, "UTC")
	cases := map[string]string{
		"2026-11-13": "is due today.",
		"2026-11-14": "is due tomorrow.",
		"2026-11-17": "is due on Tuesday.",
		"2026-12-01": "is due on Tuesday, December 1.",
		"2026-11-10": "is still waiting for you.",
	}
	for due, want := range cases {
		f.due(t, CardPatch{DueAt: set(due)})
		got := reminderText(mustCardRow(t, f), f.now, time.UTC)
		if !strings.HasSuffix(got, want) || strings.Contains(got, "missed") || strings.Contains(got, "overdue") {
			t.Errorf("due %s: %q, want …%q", due, got, want)
		}
	}
}

func mustCardRow(t *testing.T, f *reminderFixture) db.Card {
	t.Helper()
	c, err := f.b.st.GetCard(context.Background(), f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestReminderValidation(t *testing.T) {
	f := newReminders(t)
	ctx := context.Background()
	bad := []ReminderSpec{
		{Kind: "sometime"},
		{Kind: "before", OffsetMinutes: 0},
		{Kind: "before", OffsetMinutes: 365 * 24 * 60},
		{Kind: "at", At: "tomorrow"},
		{Kind: "due", TimeZone: "Mars/Olympus"},
	}
	for _, spec := range bad {
		if _, err := f.b.SetReminder(ctx, f.alice, f.card.ID, spec); err == nil {
			t.Errorf("%+v accepted", spec)
		}
	}
}
