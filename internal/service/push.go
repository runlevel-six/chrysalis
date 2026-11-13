// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"

	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
	"github.com/runlevel-six/chrysalis/internal/webpush"
)

// Push delivers reminders to people's devices with Web Push (§3.9), on top
// of the in-app list. Each browser or installed app is turned on separately,
// by the person, from Settings; nothing is pushed during their quiet hours.
type Push struct {
	st     *store.Store
	now    func() time.Time
	sender interface {
		Send(context.Context, webpush.Subscription, []byte, time.Duration) error
	}
	keys    webpush.Keys
	appName string
	log     *slog.Logger
	wg      sync.WaitGroup
}

const vapidSetting = "vapid_private_key"

// VAPIDKeys returns the server's push identity: the configured key if there
// is one, else the one it made for itself on first use (kept in the
// database, so it survives restarts and is in backups).
func VAPIDKeys(ctx context.Context, st *store.Store, configured string) (webpush.Keys, error) {
	if configured != "" {
		return webpush.ParseKeys(configured)
	}
	v, err := st.GetServerSetting(ctx, vapidSetting)
	if err == nil {
		return webpush.ParseKeys(v)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return webpush.Keys{}, err
	}
	k, err := webpush.GenerateKeys()
	if err != nil {
		return webpush.Keys{}, err
	}
	if err := st.InsertServerSetting(ctx, db.InsertServerSettingParams{Key: vapidSetting, Value: k.PrivateString()}); err != nil {
		return webpush.Keys{}, err
	}
	// Another instance may have won the race; use whatever is stored.
	v, err = st.GetServerSetting(ctx, vapidSetting)
	if err != nil {
		return webpush.Keys{}, err
	}
	return webpush.ParseKeys(v)
}

// NewPush makes the push service. sender may be a *webpush.Sender or a fake.
func NewPush(st *store.Store, now func() time.Time, keys webpush.Keys, sender interface {
	Send(context.Context, webpush.Subscription, []byte, time.Duration) error
}, appName string, log *slog.Logger) *Push {
	if appName == "" {
		appName = "Chrysalis"
	}
	return &Push{st: st, now: now, sender: sender, keys: keys, appName: appName, log: log}
}

// PublicKey is what browsers subscribe with.
func (p *Push) PublicKey() string { return p.keys.PublicString() }

const maxDevices = 20

// Subscribe records a browser's subscription for the user. A browser that
// was subscribed for someone else (a shared computer) now belongs to them.
func (p *Push) Subscribe(ctx context.Context, userID string, sub webpush.Subscription, label string) error {
	if err := sub.Check(); err != nil {
		return &ValidationError{err.Error()}
	}
	label = strings.TrimSpace(label)
	if utf8.RuneCountInString(label) > 80 {
		label = string([]rune(label)[:80])
	}
	existing, err := p.st.ListPushSubscriptions(ctx, userID)
	if err != nil {
		return err
	}
	known := false
	for _, e := range existing {
		known = known || e.Endpoint == sub.Endpoint
	}
	if !known && len(existing) >= maxDevices {
		return &ValidationError{"That's a lot of devices; turn reminders off on one you no longer use first."}
	}
	_, err = p.st.UpsertPushSubscription(ctx, db.UpsertPushSubscriptionParams{
		ID: ulid.Make().String(), UserID: userID, Endpoint: sub.Endpoint, P256dh: sub.Keys.P256dh, Auth: sub.Keys.Auth,
		Label: label, CreatedAt: store.FormatTime(p.now()),
	})
	return err
}

// Unsubscribe forgets one browser for the user.
func (p *Push) Unsubscribe(ctx context.Context, userID, endpoint string) error {
	return p.st.DeletePushSubscription(ctx, db.DeletePushSubscriptionParams{Endpoint: endpoint, UserID: userID})
}

// Subscribed reports whether endpoint is subscribed for the user.
func (p *Push) Subscribed(ctx context.Context, userID, endpoint string) (bool, int, error) {
	subs, err := p.st.ListPushSubscriptions(ctx, userID)
	if err != nil {
		return false, 0, err
	}
	for _, s := range subs {
		if s.Endpoint == endpoint {
			return true, len(subs), nil
		}
	}
	return false, len(subs), nil
}

// message is what the service worker receives.
type message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
}

// Deliver pushes a notification to the user's devices, unless it's their
// quiet hours. It returns how many devices took it.
func (p *Push) Deliver(ctx context.Context, userID string, n Notification) int {
	prefs, err := p.st.GetPrefs(ctx, userID)
	if err == nil && quiet(prefs.QuietStart, prefs.QuietEnd, p.now(), location(prefs.TimeZone)) {
		return 0
	}
	url := "/"
	if n.BoardID != "" && n.CardID != "" {
		url = "/b/" + n.BoardID + "?card=" + n.CardID
	}
	return p.send(ctx, userID, message{Title: p.appName, Body: n.Text, URL: url, Tag: n.CardID})
}

// Test sends a gentle test message to the user's devices.
func (p *Push) Test(ctx context.Context, userID string) int {
	return p.send(ctx, userID, message{Title: p.appName, Body: "Reminders will show up on this device like this.", URL: "/settings", Tag: "test"})
}

func (p *Push) send(ctx context.Context, userID string, m message) int {
	subs, err := p.st.ListPushSubscriptions(ctx, userID)
	if err != nil {
		p.log.Error("list push subscriptions", "err", err)
		return 0
	}
	body, _ := json.Marshal(m)
	ok := 0
	for _, s := range subs {
		var sub webpush.Subscription
		sub.Endpoint, sub.Keys.P256dh, sub.Keys.Auth = s.Endpoint, s.P256dh, s.Auth
		err := p.sender.Send(ctx, sub, body, 24*time.Hour)
		switch {
		case errors.Is(err, webpush.ErrGone):
			// The browser unsubscribed, or the app was removed.
			_ = p.st.DeletePushSubscriptionByID(ctx, s.ID)
		case err != nil:
			p.log.Warn("push failed", "err", err)
		default:
			ok++
			_ = p.st.TouchPushSubscription(ctx, db.TouchPushSubscriptionParams{
				LastOkAt: sql.NullString{String: store.FormatTime(p.now()), Valid: true}, ID: s.ID,
			})
		}
	}
	return ok
}

// Pusher adapts Push for reminders: deliveries run in the background, so a
// slow push service never holds up the next reminder.
func (p *Push) Pusher() Pusher {
	return func(parent context.Context, userID string, n Notification) {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			// Not cancelled with the caller (the reminder loop moves on), but
			// bounded on its own.
			ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), time.Minute)
			defer cancel()
			p.Deliver(ctx, userID, n)
		}()
	}
}

// Wait waits for background deliveries (tests, shutdown).
func (p *Push) Wait() { p.wg.Wait() }

// quiet reports whether now falls in the quiet hours start–end (local,
// "HH:MM"), which may run past midnight.
func quiet(start, end string, now time.Time, loc *time.Location) bool {
	if start == "" || end == "" || start == end {
		return false
	}
	t := now.In(loc).Format("15:04")
	if start < end {
		return t >= start && t < end
	}
	return t >= start || t < end
}
