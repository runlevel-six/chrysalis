// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/runlevel-six/chrysalis/internal/sse"
	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

// Reminders (docs/DESIGN.md §3.9) are opt-in twice: the person turns them on
// in Settings, and sets one on each card they want reminding about. Each is
// personal: two people on a board each choose their own.
//
// A reminder goes off at the card's due time, some time before it, or at a
// time of its own. A date-only due date counts as 9 in the morning in the
// person's time zone. When it goes off it becomes an entry in the person's
// notification list (and, where set up, a push); it can be snoozed.

// Reminder is one person's reminder on one card.
type Reminder struct {
	CardID        string  `json:"cardId"`
	Kind          string  `json:"kind"`                    // due | before | at
	OffsetMinutes int64   `json:"offsetMinutes,omitempty"` // for before
	At            *string `json:"at,omitempty"`            // for at: a UTC time
	TimeZone      string  `json:"timeZone"`
	// RemindAt is when it will go off next (UTC), or null if it can't (a due
	// or before reminder on a card with no due date) or already has.
	RemindAt *string `json:"remindAt"`
	FiredAt  *string `json:"firedAt,omitempty"`
}

// ReminderSpec is what the client asks for.
type ReminderSpec struct {
	Kind          string `json:"kind"`
	OffsetMinutes int64  `json:"offsetMinutes"`
	At            string `json:"at"`
	TimeZone      string `json:"timeZone"`
}

// Notification is one entry in the in-app list.
type Notification struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`
	Text      string  `json:"text"`
	CardID    string  `json:"cardId,omitempty"`
	BoardID   string  `json:"boardId,omitempty"`
	CreatedAt string  `json:"createdAt"`
	ReadAt    *string `json:"readAt"`
}

// dateOnlyHour is when a reminder for a date-only due date goes off: the
// morning of the day, early enough to act on it.
const dateOnlyHour = 9

const maxOffset = 60 * 24 * 60 // 60 days, in minutes

func cleanZone(tz string) (string, bool) {
	if tz == "" {
		return "", true
	}
	if location(tz) == time.UTC && tz != "UTC" && tz != "Etc/UTC" {
		return "", false
	}
	return tz, true
}

func reminderFromRow(r db.CardReminder) Reminder {
	out := Reminder{CardID: r.CardID, Kind: r.Kind, OffsetMinutes: r.OffsetMinutes, TimeZone: r.TimeZone,
		At: nullStr(r.At), FiredAt: nullStr(r.FiredAt)}
	if r.FiredAt.Valid {
		return out
	}
	if r.SnoozedUntil.Valid {
		out.RemindAt = &r.SnoozedUntil.String
	} else {
		out.RemindAt = nullStr(r.RemindAt)
	}
	return out
}

// dueInstant is when a card is due, as an instant: its timestamp, or 9 am
// local on its date.
func dueInstant(c db.Card, loc *time.Location) (time.Time, bool) {
	if !c.DueAt.Valid {
		return time.Time{}, false
	}
	if c.DueHasTime != 0 {
		t, err := time.Parse(time.RFC3339Nano, c.DueAt.String)
		return t, err == nil
	}
	d, err := time.ParseInLocation(dateOnly, c.DueAt.String[:min(len(c.DueAt.String), len(dateOnly))], loc)
	if err != nil {
		return time.Time{}, false
	}
	return d.Add(dateOnlyHour * time.Hour), true
}

// remindAt works out when a reminder goes off, or false if it can't.
func remindAt(c db.Card, kind string, offset int64, at sql.NullString, tz string) (time.Time, bool) {
	if kind == "at" {
		t, err := time.Parse(time.RFC3339Nano, at.String)
		return t, err == nil && at.Valid
	}
	due, ok := dueInstant(c, location(tz))
	if !ok {
		return time.Time{}, false
	}
	if kind == "before" {
		due = due.Add(-time.Duration(offset) * time.Minute)
	}
	return due, true
}

// Reminder returns the user's reminder on a card, or nil.
func (s *Boards) Reminder(ctx context.Context, userID, cardID string) (*Reminder, error) {
	if _, err := liveCard(ctx, s.st.Queries, userID, cardID, false); err != nil {
		return nil, err
	}
	r, err := s.st.GetReminder(ctx, db.GetReminderParams{CardID: cardID, UserID: userID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := reminderFromRow(r)
	return &out, nil
}

// SetReminder sets (or replaces) the user's reminder on a card. Viewers may
// set them too: a reminder is personal, not a change to the board.
func (s *Boards) SetReminder(ctx context.Context, userID, cardID string, spec ReminderSpec) (Reminder, error) {
	tz, ok := cleanZone(spec.TimeZone)
	if !ok {
		return Reminder{}, &ValidationError{"That time zone isn't known."}
	}
	if tz == "" {
		tz = "UTC"
	}
	at := sql.NullString{}
	switch spec.Kind {
	case "due":
		spec.OffsetMinutes = 0
	case "before":
		if spec.OffsetMinutes <= 0 || spec.OffsetMinutes > maxOffset {
			return Reminder{}, &ValidationError{"Choose how long before, up to 60 days."}
		}
	case "at":
		t, err := time.Parse(time.RFC3339Nano, spec.At)
		if err != nil {
			return Reminder{}, &ValidationError{"Choose a date and time for the reminder."}
		}
		at = sql.NullString{String: store.FormatTime(t), Valid: true}
		spec.OffsetMinutes = 0
	default:
		return Reminder{}, &ValidationError{"kind must be due, before or at"}
	}
	var out Reminder
	err := s.tx(ctx, func(q *db.Queries) error {
		c, err := liveCard(ctx, q, userID, cardID, false)
		if err != nil {
			return err
		}
		when := sql.NullString{}
		if t, ok := remindAt(c, spec.Kind, spec.OffsetMinutes, at, tz); ok {
			when = sql.NullString{String: store.FormatTime(t), Valid: true}
		}
		now := store.FormatTime(s.now())
		row, err := q.UpsertReminder(ctx, db.UpsertReminderParams{
			CardID: cardID, UserID: userID, Kind: spec.Kind, OffsetMinutes: spec.OffsetMinutes, At: at,
			TimeZone: tz, RemindAt: when, CreatedAt: now, UpdatedAt: now,
		})
		out = reminderFromRow(row)
		return err
	})
	if err == nil {
		s.emit(ctx, sse.Event{UserID: userID, Kind: "reminders", EntityID: cardID, CardID: cardID})
	}
	return out, err
}

// ClearReminder removes the user's reminder on a card.
func (s *Boards) ClearReminder(ctx context.Context, userID, cardID string) error {
	if _, err := liveCard(ctx, s.st.Queries, userID, cardID, false); err != nil {
		return err
	}
	err := s.st.DeleteReminder(ctx, db.DeleteReminderParams{CardID: cardID, UserID: userID})
	if err == nil {
		s.emit(ctx, sse.Event{UserID: userID, Kind: "reminders", EntityID: cardID, CardID: cardID})
	}
	return err
}

// rescheduleReminders follows a card's new due date: every reminder on it
// is worked out again, and one that hasn't gone off yet for the new time
// will.
func (s *Boards) rescheduleReminders(ctx context.Context, q *db.Queries, c db.Card) error {
	rs, err := q.ListCardReminders(ctx, c.ID)
	if err != nil {
		return err
	}
	now := s.now()
	for _, r := range rs {
		if r.Kind == "at" {
			continue
		}
		when, fired := sql.NullString{}, r.FiredAt
		if t, ok := remindAt(c, r.Kind, r.OffsetMinutes, r.At, r.TimeZone); ok {
			when = sql.NullString{String: store.FormatTime(t), Valid: true}
			if t.After(now) {
				fired = sql.NullString{} // it will go off again at the new time
			}
		}
		if err := q.SetReminderSchedule(ctx, db.SetReminderScheduleParams{
			RemindAt: when, FiredAt: fired, UpdatedAt: store.FormatTime(now), CardID: r.CardID, UserID: r.UserID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ---------- Going off ----------

// Pusher delivers a notification beyond the app (Web Push, email); nil
// means in-app only.
type Pusher func(ctx context.Context, userID string, n Notification)

// SetPusher connects reminders to push delivery.
func (s *Boards) SetPusher(p Pusher) { s.push = p }

// FireReminders turns every reminder that's due into a notification. It's
// run every half minute by the server; each reminder goes off once.
func (s *Boards) FireReminders(ctx context.Context) (int, error) {
	now := s.now()
	due, err := s.st.ListDueReminders(ctx, sql.NullString{String: store.FormatTime(now), Valid: true})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range due {
		note, ok, err := s.fire(ctx, r, now)
		if err != nil {
			return n, err
		}
		if !ok {
			continue
		}
		n++
		s.emit(ctx, sse.Event{UserID: r.UserID, Kind: "notifications", EntityID: note.ID})
		if s.push != nil {
			s.push(ctx, r.UserID, note)
		}
	}
	return n, nil
}

func (s *Boards) fire(ctx context.Context, r db.CardReminder, now time.Time) (Notification, bool, error) {
	var note Notification
	sent := false
	err := s.tx(ctx, func(q *db.Queries) error {
		if err := q.MarkReminderFired(ctx, db.MarkReminderFiredParams{
			FiredAt: sql.NullString{String: store.FormatTime(now), Valid: true}, CardID: r.CardID, UserID: r.UserID,
		}); err != nil {
			return err
		}
		// Quietly skip what no longer needs a nudge: finished, archived or
		// trashed cards, reminders switched off, boards they've left.
		c, err := liveCard(ctx, q, r.UserID, r.CardID, false)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if c.CompletedAt.Valid || c.ArchivedAt.Valid {
			return nil
		}
		prefs, err := q.GetPrefs(ctx, r.UserID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && prefs.RemindersEnabled == 0) {
			return nil
		}
		if err != nil {
			return err
		}
		tz := prefs.TimeZone
		if tz == "" {
			tz = r.TimeZone
		}
		row, err := q.CreateNotification(ctx, db.CreateNotificationParams{
			ID: ulid.Make().String(), UserID: r.UserID,
			CardID:  sql.NullString{String: c.ID, Valid: true},
			BoardID: sql.NullString{String: c.BoardID, Valid: true},
			Kind:    "reminder", Text: reminderText(c, now, location(tz)), CreatedAt: store.FormatTime(now),
		})
		if err != nil {
			return err
		}
		note, sent = notificationFromRow(row), true
		return nil
	})
	return note, sent, err
}

// reminderText is gentle (§3.9): "Heads up: … is due tomorrow", never
// "You missed…".
func reminderText(c db.Card, now time.Time, loc *time.Location) string {
	title := "“" + c.Title + "”"
	due, ok := dueInstant(c, loc)
	if !ok {
		return "A reminder about " + title + "."
	}
	today := now.In(loc)
	y, m, d := today.Date()
	dy, dm, dd := due.In(loc).Date()
	days := int(time.Date(dy, dm, dd, 0, 0, 0, 0, loc).Sub(time.Date(y, m, d, 0, 0, 0, 0, loc)).Hours() / 24)
	at := ""
	if c.DueHasTime != 0 {
		at = " at " + due.In(loc).Format("3:04 PM")
	}
	switch {
	case days < 0:
		return "Heads up: " + title + " is still waiting for you."
	case days == 0:
		return "Heads up: " + title + " is due today" + at + "."
	case days == 1:
		return "Heads up: " + title + " is due tomorrow" + at + "."
	case days < 7:
		return fmt.Sprintf("Heads up: %s is due on %s%s.", title, due.In(loc).Format("Monday"), at)
	default:
		return fmt.Sprintf("Heads up: %s is due on %s%s.", title, due.In(loc).Format("Monday, January 2"), at)
	}
}

// ---------- The list ----------

func notificationFromRow(n db.Notification) Notification {
	return Notification{ID: n.ID, Kind: n.Kind, Text: n.Text, CardID: n.CardID.String, BoardID: n.BoardID.String,
		CreatedAt: n.CreatedAt, ReadAt: nullStr(n.ReadAt)}
}

// Notifications lists the user's notifications that haven't been dismissed,
// newest first.
func (s *Boards) Notifications(ctx context.Context, userID string) ([]Notification, error) {
	rows, err := s.st.ListNotifications(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Notification, 0, len(rows))
	for _, r := range rows {
		out = append(out, notificationFromRow(r))
	}
	return out, nil
}

// MarkNotificationsRead marks everything in the list as seen.
func (s *Boards) MarkNotificationsRead(ctx context.Context, userID string) error {
	err := s.st.MarkNotificationsRead(ctx, db.MarkNotificationsReadParams{
		ReadAt: sql.NullString{String: store.FormatTime(s.now()), Valid: true}, UserID: userID,
	})
	if err == nil {
		s.emit(ctx, sse.Event{UserID: userID, Kind: "notifications"})
	}
	return err
}

// DismissNotification takes one off the list ("" for all of them).
func (s *Boards) DismissNotification(ctx context.Context, userID, id string) error {
	now := sql.NullString{String: store.FormatTime(s.now()), Valid: true}
	var err error
	if id == "" {
		err = s.st.DismissAllNotifications(ctx, db.DismissAllNotificationsParams{DismissedAt: now, ReadAt: now, UserID: userID})
	} else {
		if _, err = s.st.GetNotification(ctx, db.GetNotificationParams{ID: id, UserID: userID}); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		err = s.st.DismissNotification(ctx, db.DismissNotificationParams{DismissedAt: now, ReadAt: now, ID: id, UserID: userID})
	}
	if err == nil {
		s.emit(ctx, sse.Event{UserID: userID, Kind: "notifications"})
	}
	return err
}

// SnoozeNotification takes a reminder off the list and brings it back at
// until (an hour, tonight, tomorrow, or a chosen time).
func (s *Boards) SnoozeNotification(ctx context.Context, userID, id string, until time.Time) error {
	if !until.After(s.now()) {
		return &ValidationError{"Choose a time later than now."}
	}
	if until.After(s.now().AddDate(1, 0, 0)) {
		return &ValidationError{"Choose a time within the next year."}
	}
	err := s.tx(ctx, func(q *db.Queries) error {
		n, err := q.GetNotification(ctx, db.GetNotificationParams{ID: id, UserID: userID})
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !n.CardID.Valid {
			return &ValidationError{"This can't be snoozed."}
		}
		c, err := liveCard(ctx, q, userID, n.CardID.String, false)
		if err != nil {
			return err
		}
		now := store.FormatTime(s.now())
		// A reminder deleted in the meantime comes back as a one-off.
		if _, err := q.GetReminder(ctx, db.GetReminderParams{CardID: c.ID, UserID: userID}); errors.Is(err, sql.ErrNoRows) {
			at := sql.NullString{String: store.FormatTime(until), Valid: true}
			if _, err := q.UpsertReminder(ctx, db.UpsertReminderParams{
				CardID: c.ID, UserID: userID, Kind: "at", At: at, TimeZone: "UTC", RemindAt: at, CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if err := q.SnoozeReminder(ctx, db.SnoozeReminderParams{
			SnoozedUntil: sql.NullString{String: store.FormatTime(until), Valid: true}, UpdatedAt: now, CardID: c.ID, UserID: userID,
		}); err != nil {
			return err
		}
		ts := sql.NullString{String: now, Valid: true}
		return q.DismissNotification(ctx, db.DismissNotificationParams{DismissedAt: ts, ReadAt: ts, ID: id, UserID: userID})
	})
	if err == nil {
		s.emit(ctx, sse.Event{UserID: userID, Kind: "notifications"})
	}
	return err
}

// PurgeNotifications removes notifications older than cutoff.
func PurgeNotifications(ctx context.Context, st *store.Store, cutoff time.Time) (int64, error) {
	return st.PurgeOldNotifications(ctx, store.FormatTime(cutoff))
}
