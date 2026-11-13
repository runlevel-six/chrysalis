// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/runlevel-six/chrysalis/internal/order"
	"github.com/runlevel-six/chrysalis/internal/sse"
	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

// centeringFixture is a store with alice and bob, a clock the test moves,
// and the centering service.
type centeringFixture struct {
	st     *store.Store
	c      *Centering
	alice  string
	bob    string
	now    time.Time
	events []sse.Event
}

func newCentering(t *testing.T) *centeringFixture {
	t.Helper()
	st, alice := newImportStore(t)
	bob, err := st.UpsertUser(context.Background(), db.UpsertUserParams{
		ID: "01JBOB0000000000000000000", OidcSub: "bob", Email: "bob@example.com", DisplayName: "Bob",
		CreatedAt: "2026-01-01T00:00:00.000Z", LastSeenAt: "2026-01-01T00:00:00.000Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	f := &centeringFixture{st: st, alice: alice, bob: bob.ID, now: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)}
	b := NewBoards(st, func() time.Time { return f.now })
	b.SetNotifier(func(e sse.Event) { f.events = append(f.events, e) })
	f.c = NewCentering(b)
	return f
}

// collection makes a collection for alice with the given entry texts.
func (f *centeringFixture) collection(t *testing.T, name string, texts ...string) (QuoteCollection, []Quote) {
	t.Helper()
	ctx := context.Background()
	col, err := f.c.CreateCollection(ctx, f.alice, name)
	if err != nil {
		t.Fatal(err)
	}
	var out []Quote
	for _, text := range texts {
		q, err := f.c.CreateEntry(ctx, f.alice, col.ID, EntryInput{Text: text, Attribution: "A gardener"})
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, q)
	}
	return col, out
}

func (f *centeringFixture) enable(t *testing.T, patch CenteringSettingsPatch) {
	t.Helper()
	on := true
	patch.Enabled = &on
	if _, err := f.c.UpdateSettings(context.Background(), f.alice, patch); err != nil {
		t.Fatal(err)
	}
}

func (f *centeringFixture) current(t *testing.T, tz, load string) CenteringLine {
	t.Helper()
	l, err := f.c.Current(context.Background(), f.alice, tz, load)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func text(l CenteringLine) string {
	if l.Entry == nil {
		return ""
	}
	return l.Entry.Text
}

func ptr[T any](v T) *T { return &v }

func TestCenteringOffByDefault(t *testing.T) {
	f := newCentering(t)
	f.collection(t, "Morning", "Be still, and breathe.")
	s, err := f.c.Settings(context.Background(), f.alice)
	if err != nil {
		t.Fatal(err)
	}
	if s.Enabled || s.Mode != "rotate" || s.Interval != "daily" || s.Order != "sequential" {
		t.Fatalf("defaults = %+v", s)
	}
	if l := f.current(t, "", ""); l.Enabled || l.Entry != nil {
		t.Fatalf("line while off = %+v", l)
	}
}

func TestCenteringCollectionsAndEntries(t *testing.T) {
	ctx := context.Background()
	f := newCentering(t)
	col, entries := f.collection(t, "Morning", "One", "Two", "Three")

	// A new collection feeds the line right away.
	s, _ := f.c.Settings(ctx, f.alice)
	if !slices.Equal(s.CollectionIDs, []string{col.ID}) {
		t.Fatalf("selected = %v", s.CollectionIDs)
	}
	list, err := f.c.Collections(ctx, f.alice)
	if err != nil || len(list) != 1 || list[0].EntryCount != 3 || list[0].Role != RoleOwner {
		t.Fatalf("collections = %+v, %v", list, err)
	}

	// Edit, reorder (move Three first), trash and restore.
	e, err := f.c.UpdateEntry(ctx, f.alice, entries[1].ID, EntryPatch{Text: ptr("  Two,\r\nrevised "), Tags: &[]string{"calm", " calm ", "", "rest"}, Reference: ptr("Line 2\nwrapped")})
	if err != nil || e.Text != "Two,\nrevised" || !slices.Equal(e.Tags, []string{"calm", "rest"}) || e.Reference != "Line 2 wrapped" || e.Attribution != "A gardener" {
		t.Fatalf("update = %+v, %v", e, err)
	}
	if _, err := f.c.UpdateEntry(ctx, f.alice, entries[2].ID, EntryPatch{Position: ptr("Z")}); err == nil {
		t.Fatal("bad position accepted")
	}
	before, err := order.KeyBetween("", entries[0].Position)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.c.UpdateEntry(ctx, f.alice, entries[2].ID, EntryPatch{Position: &before}); err != nil {
		t.Fatal(err)
	}
	got, _ := f.c.Entries(ctx, f.alice, col.ID)
	if got[0].ID != entries[2].ID {
		t.Fatalf("order after move = %v", []string{got[0].Text, got[1].Text, got[2].Text})
	}
	if _, err := f.c.SetEntryTrashed(ctx, f.alice, entries[0].ID, true); err != nil {
		t.Fatal(err)
	}
	trash, _ := f.c.Trash(ctx, f.alice)
	if len(trash.Entries) != 1 || trash.Entries[0].ID != entries[0].ID {
		t.Fatalf("trash = %+v", trash)
	}
	if got, _ := f.c.Entries(ctx, f.alice, col.ID); len(got) != 2 {
		t.Fatalf("live entries = %d", len(got))
	}
	if _, err := f.c.SetEntryTrashed(ctx, f.alice, entries[0].ID, false); err != nil {
		t.Fatal(err)
	}

	// Validation.
	for _, in := range []EntryInput{
		{Text: "   "},
		{Text: strings.Repeat("x", MaxEntryTextLen+1)},
		{Text: "ok", Attribution: strings.Repeat("x", MaxEntryAttributionLen+1)},
		{Text: "ok", Translation: strings.Repeat("x", MaxEntryTranslationLen+1)},
		{Text: "ok", Tags: []string{strings.Repeat("t", MaxEntryTagLen+1)}},
		{Text: "bell\a"},
	} {
		_, err := f.c.CreateEntry(ctx, f.alice, col.ID, in)
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("%+v: err = %v", in, err)
		}
	}
	tooMany := make([]string, MaxEntryTags+1)
	for i := range tooMany {
		tooMany[i] = strings.Repeat("t", i+1)
	}
	if _, err := f.c.CreateEntry(ctx, f.alice, col.ID, EntryInput{Text: "ok", Tags: tooMany}); err == nil {
		t.Fatal("too many tags accepted")
	}

	// Rename; trash and restore the collection.
	if c, err := f.c.UpdateCollection(ctx, f.alice, col.ID, CollectionPatch{Name: ptr("Evening")}); err != nil || c.Name != "Evening" {
		t.Fatalf("rename = %+v, %v", c, err)
	}
	if _, err := f.c.SetCollectionTrashed(ctx, f.alice, col.ID, true); err != nil {
		t.Fatal(err)
	}
	if list, _ := f.c.Collections(ctx, f.alice); len(list) != 0 {
		t.Fatalf("trashed collection still listed: %+v", list)
	}
	if s, _ := f.c.Settings(ctx, f.alice); len(s.CollectionIDs) != 0 {
		t.Fatalf("trashed collection still selected: %v", s.CollectionIDs)
	}
	if _, err := f.c.Entries(ctx, f.alice, col.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("entries of a trashed collection: %v", err)
	}
	if trash, _ := f.c.Trash(ctx, f.alice); len(trash.Collections) != 1 {
		t.Fatalf("trash = %+v", trash)
	}
	if _, err := f.c.SetCollectionTrashed(ctx, f.alice, col.ID, false); err != nil {
		t.Fatal(err)
	}
	if s, _ := f.c.Settings(ctx, f.alice); len(s.CollectionIDs) != 1 {
		t.Fatalf("restored collection not selected again: %v", s.CollectionIDs)
	}
}

func TestCenteringAuthorization(t *testing.T) {
	ctx := context.Background()
	f := newCentering(t)
	col, entries := f.collection(t, "Morning", "One")

	if list, _ := f.c.Collections(ctx, f.bob); len(list) != 0 {
		t.Fatalf("bob sees %+v", list)
	}
	checks := map[string]error{}
	_, checks["entries"] = f.c.Entries(ctx, f.bob, col.ID)
	_, checks["create entry"] = f.c.CreateEntry(ctx, f.bob, col.ID, EntryInput{Text: "x"})
	_, checks["update entry"] = f.c.UpdateEntry(ctx, f.bob, entries[0].ID, EntryPatch{Text: ptr("x")})
	_, checks["trash entry"] = f.c.SetEntryTrashed(ctx, f.bob, entries[0].ID, true)
	_, checks["rename"] = f.c.UpdateCollection(ctx, f.bob, col.ID, CollectionPatch{Name: ptr("x")})
	_, checks["trash collection"] = f.c.SetCollectionTrashed(ctx, f.bob, col.ID, true)
	_, _, checks["export"] = f.c.Export(ctx, f.bob, col.ID, "yaml")
	_, checks["import"] = f.c.Import(ctx, f.bob, ImportTarget{CollectionID: col.ID}, QuoteFile{Entries: []QuoteFileEntry{{Text: "x"}}}, false)
	_, checks["select"] = f.c.UpdateSettings(ctx, f.bob, CenteringSettingsPatch{CollectionIDs: &[]string{col.ID}})
	_, checks["pin"] = f.c.Pin(ctx, f.bob, entries[0].ID)
	for what, err := range checks {
		var verr *ValidationError
		if !errors.Is(err, ErrNotFound) && !errors.As(err, &verr) {
			t.Errorf("bob %s: err = %v", what, err)
		}
	}

	// Sharing (Phase 2 has the UI): a viewer can read and feed their line
	// from it, but not change it.
	if err := f.st.AddQuoteCollectionMember(ctx, db.AddQuoteCollectionMemberParams{CollectionID: col.ID, UserID: f.bob, Role: "viewer"}); err != nil {
		t.Fatal(err)
	}
	if got, err := f.c.Entries(ctx, f.bob, col.ID); err != nil || len(got) != 1 {
		t.Fatalf("viewer entries = %v, %v", got, err)
	}
	if _, err := f.c.CreateEntry(ctx, f.bob, col.ID, EntryInput{Text: "x"}); !errors.Is(err, ErrCollectionForbidden) {
		t.Fatalf("viewer create: %v", err)
	}
	if _, err := f.c.UpdateSettings(ctx, f.bob, CenteringSettingsPatch{Enabled: ptr(true), CollectionIDs: &[]string{col.ID}}); err != nil {
		t.Fatal(err)
	}
	l, err := f.c.Current(ctx, f.bob, "", "")
	if err != nil || text(l) != "One" {
		t.Fatalf("viewer line = %+v, %v", l, err)
	}
	if _, err := f.c.SetCollectionTrashed(ctx, f.bob, col.ID, true); !errors.Is(err, ErrCollectionForbidden) {
		t.Fatalf("viewer trash collection: %v", err)
	}
	// Changes reach every member.
	f.events = nil
	if _, err := f.c.CreateEntry(ctx, f.alice, col.ID, EntryInput{Text: "Two"}); err != nil {
		t.Fatal(err)
	}
	var users []string
	for _, e := range f.events {
		if e.Kind == "centering" {
			users = append(users, e.UserID)
		}
	}
	want := []string{f.alice, f.bob}
	slices.Sort(users)
	slices.Sort(want)
	if !slices.Equal(users, want) {
		t.Fatalf("notified %v", users)
	}
}

func TestCenteringDailyRotationFollowsLocalMidnight(t *testing.T) {
	f := newCentering(t)
	f.collection(t, "Morning", "One", "Two", "Three")
	f.enable(t, CenteringSettingsPatch{})
	const tz = "America/Chicago" // UTC-5 in June
	f.now = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	if got := text(f.current(t, tz, "")); got != "One" {
		t.Fatalf("first = %q", got)
	}
	// Later the same local day (and after UTC midnight): no change.
	f.now = time.Date(2026, 6, 2, 4, 59, 0, 0, time.UTC) // 23:59 in Chicago
	if got := text(f.current(t, tz, "")); got != "One" {
		t.Fatalf("same local day = %q", got)
	}
	f.events = nil
	f.now = time.Date(2026, 6, 2, 5, 0, 0, 0, time.UTC) // local midnight
	if got := text(f.current(t, tz, "")); got != "Two" {
		t.Fatalf("next day = %q", got)
	}
	if len(f.events) != 0 {
		t.Fatalf("rotating on read sent notices: %+v", f.events)
	}
	// A device in a zone that's still on yesterday doesn't pull it back.
	if got := text(f.current(t, "Pacific/Honolulu", "")); got != "Two" {
		t.Fatalf("zone behind = %q", got)
	}
	f.now = time.Date(2026, 6, 3, 5, 0, 0, 0, time.UTC)
	f.current(t, tz, "")
	f.now = time.Date(2026, 6, 4, 5, 0, 0, 0, time.UTC)
	if got := text(f.current(t, tz, "")); got != "One" {
		t.Fatalf("wraps to %q", got)
	}
	// An unknown zone falls back to UTC rather than failing.
	if got := text(f.current(t, "Not/AZone", "")); got != "One" {
		t.Fatalf("bad zone = %q", got)
	}
}

func TestCenteringHourlyWeeklyAndPageLoad(t *testing.T) {
	f := newCentering(t)
	f.collection(t, "Morning", "One", "Two", "Three")

	f.enable(t, CenteringSettingsPatch{Interval: ptr("hourly")})
	f.now = time.Date(2026, 6, 1, 12, 10, 0, 0, time.UTC)
	if got := text(f.current(t, "UTC", "")); got != "One" {
		t.Fatalf("hourly first = %q", got)
	}
	f.now = f.now.Add(49 * time.Minute)
	if got := text(f.current(t, "UTC", "")); got != "One" {
		t.Fatalf("same hour = %q", got)
	}
	f.now = f.now.Add(time.Minute)
	if got := text(f.current(t, "UTC", "")); got != "Two" {
		t.Fatalf("next hour = %q", got)
	}

	// Weekly: changes on Monday. 2026-06-01 is a Monday.
	f.enable(t, CenteringSettingsPatch{Interval: ptr("weekly")})
	f.now = time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC) // Wednesday
	if got := text(f.current(t, "UTC", "")); got != "Two" {
		t.Fatalf("switching interval changed the entry: %q", got)
	}
	f.now = time.Date(2026, 6, 7, 23, 0, 0, 0, time.UTC) // Sunday
	if got := text(f.current(t, "UTC", "")); got != "Two" {
		t.Fatalf("same week = %q", got)
	}
	f.now = time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC) // Monday
	if got := text(f.current(t, "UTC", "")); got != "Three" {
		t.Fatalf("next week = %q", got)
	}

	// Every page load: each new load moves on once; refetches don't.
	f.enable(t, CenteringSettingsPatch{Interval: ptr("page_load")})
	if got := text(f.current(t, "UTC", "tab-1")); got != "One" {
		t.Fatalf("load 1 = %q", got)
	}
	if got := text(f.current(t, "UTC", "tab-1")); got != "One" {
		t.Fatalf("refetch = %q", got)
	}
	if got := text(f.current(t, "UTC", "tab-2")); got != "Two" {
		t.Fatalf("load 2 = %q", got)
	}
	if got := text(f.current(t, "UTC", "tab-1")); got != "Two" {
		t.Fatalf("first tab refetching = %q", got)
	}
}

func TestPeriodAndMidnight(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 3, 8, 3, 30, 0, 0, time.UTC) // 22:30 Saturday in New York
	for interval, want := range map[string]string{"daily": "2026-03-07", "hourly": "2026-03-07T22", "weekly": "2026-03-02", "page_load": ""} {
		if got := period(interval, at, ny); got != want {
			t.Errorf("%s = %q, want %q", interval, got, want)
		}
	}
	// The clocks go forward that night; midnight is still midnight.
	if got := nextMidnight(at, ny); !got.Equal(time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC)) {
		t.Errorf("next midnight = %v", got.UTC())
	}
	if location("Local") != time.UTC || location("") != time.UTC || location("../../etc/passwd") != time.UTC {
		t.Error("unsafe or empty zones should be UTC")
	}
}

func TestCenteringShuffleAvoidsRepeats(t *testing.T) {
	f := newCentering(t)
	_, entries := f.collection(t, "Morning", "A", "B", "C", "D", "E")
	f.c.SetRand(func(n int) int { return n - 1 }) // always the last candidate: deterministic
	f.enable(t, CenteringSettingsPatch{Order: ptr("shuffle"), Interval: ptr("page_load")})
	ctx := context.Background()

	var shown []string
	for i := range len(entries) {
		shown = append(shown, text(f.current(t, "", "load-"+string(rune('a'+i)))))
	}
	sorted := slices.Clone(shown)
	slices.Sort(sorted)
	if !slices.Equal(sorted, []string{"A", "B", "C", "D", "E"}) {
		t.Fatalf("one cycle showed %v", shown)
	}
	// "Previous" walks back through this cycle; "next" walks forward again.
	l, err := f.c.Step(ctx, f.alice, -1)
	if err != nil || text(l) != shown[3] {
		t.Fatalf("previous = %q, %v (shown %v)", text(l), err, shown)
	}
	if l, _ := f.c.Step(ctx, f.alice, 1); text(l) != shown[4] {
		t.Fatalf("next = %q", text(l))
	}
	// A new cycle never starts with the entry just shown.
	next := text(f.current(t, "", "load-z"))
	if next == shown[4] {
		t.Fatalf("repeated %q across cycles", next)
	}

	// With real randomness, a cycle still covers everything once.
	f.c.SetRand(NewCentering(NewBoards(f.st, time.Now)).intn)
	if _, err := f.c.UpdateSettings(ctx, f.alice, CenteringSettingsPatch{Order: ptr("sequential")}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.c.UpdateSettings(ctx, f.alice, CenteringSettingsPatch{Order: ptr("shuffle")}); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for i := range len(entries) {
		seen[text(f.current(t, "", "real-"+string(rune('a'+i))))] = true
	}
	if len(seen) != len(entries) {
		t.Fatalf("a shuffled cycle repeated: %v", seen)
	}
}

func TestCenteringStepPinHide(t *testing.T) {
	ctx := context.Background()
	f := newCentering(t)
	_, entries := f.collection(t, "Morning", "One", "Two", "Three")
	f.enable(t, CenteringSettingsPatch{})
	f.current(t, "UTC", "")

	f.events = nil
	l, err := f.c.Step(ctx, f.alice, -1)
	if err != nil || text(l) != "Three" {
		t.Fatalf("previous wraps to %q, %v", text(l), err)
	}
	if len(f.events) != 1 || f.events[0].UserID != f.alice || f.events[0].Kind != "centering" {
		t.Fatalf("events = %+v", f.events)
	}
	if l, _ := f.c.Step(ctx, f.alice, 1); text(l) != "One" {
		t.Fatalf("next = %q", text(l))
	}
	if _, err := f.c.Step(ctx, f.alice, 2); err == nil {
		t.Fatal("dir 2 accepted")
	}
	// Every device sees the stepped entry.
	if got := text(f.current(t, "Asia/Tokyo", "")); got != "One" {
		t.Fatalf("after stepping = %q", got)
	}

	// Pin: the entry stays across boundaries until unpinned.
	l, err = f.c.Pin(ctx, f.alice, "")
	if err != nil || !l.Pinned || l.Mode != "pinned" || text(l) != "One" {
		t.Fatalf("pin = %+v, %v", l, err)
	}
	f.now = f.now.AddDate(0, 0, 3)
	if got := f.current(t, "UTC", ""); text(got) != "One" || !got.Pinned {
		t.Fatalf("pinned after days = %+v", got)
	}
	if s, _ := f.c.Settings(ctx, f.alice); s.Mode != "pinned" {
		t.Fatalf("settings mode = %q", s.Mode)
	}
	if l, err := f.c.Unpin(ctx, f.alice); err != nil || l.Pinned || text(l) != "One" {
		t.Fatalf("unpin = %+v, %v", l, err)
	}
	if _, err := f.c.Pin(ctx, f.alice, "nope"); err == nil {
		t.Fatal("pinned an unknown entry")
	}
	// Choosing "pinned" in settings pins what's showing.
	if _, err := f.c.UpdateSettings(ctx, f.alice, CenteringSettingsPatch{Mode: ptr("pinned")}); err != nil {
		t.Fatal(err)
	}
	if got := f.current(t, "UTC", ""); !got.Pinned || text(got) != "One" {
		t.Fatalf("pinned from settings = %+v", got)
	}
	// Deleting the pinned entry goes back to rotating.
	if _, err := f.c.SetEntryTrashed(ctx, f.alice, entries[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if got := f.current(t, "UTC", ""); got.Pinned || text(got) != "Two" {
		t.Fatalf("after deleting the pinned entry = %+v", got)
	}

	// Hide for today: until the next local midnight, on every device.
	f.now = time.Date(2026, 6, 10, 20, 0, 0, 0, time.UTC) // 15:00 in Chicago
	l, err = f.c.HideForToday(ctx, f.alice, "America/Chicago")
	if err != nil || !l.Hidden || l.Entry != nil || l.HiddenUntil != "2026-06-11T05:00:00.000Z" {
		t.Fatalf("hide = %+v, %v", l, err)
	}
	f.now = time.Date(2026, 6, 11, 4, 59, 0, 0, time.UTC)
	if got := f.current(t, "Europe/London", ""); !got.Hidden || got.Entry != nil {
		t.Fatalf("still hidden elsewhere = %+v", got)
	}
	f.now = time.Date(2026, 6, 11, 5, 0, 0, 0, time.UTC)
	if got := f.current(t, "America/Chicago", ""); got.Hidden || got.Entry == nil {
		t.Fatalf("after midnight = %+v", got)
	}
	if _, err := f.c.HideForToday(ctx, f.alice, "UTC"); err != nil {
		t.Fatal(err)
	}
	if l, err := f.c.Unhide(ctx, f.alice); err != nil || l.Hidden || l.Entry == nil {
		t.Fatalf("unhide = %+v, %v", l, err)
	}
}

func TestCenteringCarriesOnAfterDeletion(t *testing.T) {
	ctx := context.Background()
	f := newCentering(t)
	_, entries := f.collection(t, "Morning", "One", "Two", "Three")
	f.enable(t, CenteringSettingsPatch{})
	if _, err := f.c.Pin(ctx, f.alice, entries[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.c.Unpin(ctx, f.alice); err != nil {
		t.Fatal(err)
	}
	if _, err := f.c.SetEntryTrashed(ctx, f.alice, entries[1].ID, true); err != nil {
		t.Fatal(err)
	}
	if got := text(f.current(t, "UTC", "")); got != "Three" {
		t.Fatalf("after deleting the current entry = %q, want the one after it", got)
	}

	// Unselecting every collection empties the line, calmly.
	if _, err := f.c.UpdateSettings(ctx, f.alice, CenteringSettingsPatch{CollectionIDs: &[]string{}}); err != nil {
		t.Fatal(err)
	}
	if got := f.current(t, "UTC", ""); got.Entry != nil || got.PoolSize != 0 || !got.Enabled {
		t.Fatalf("empty pool = %+v", got)
	}
	if _, err := f.c.UpdateSettings(ctx, f.alice, CenteringSettingsPatch{Mode: ptr("pinned")}); err == nil {
		t.Fatal("pinned with nothing to pin")
	}
	if _, err := f.c.UpdateSettings(ctx, f.alice, CenteringSettingsPatch{Interval: ptr("monthly")}); err == nil {
		t.Fatal("unknown interval accepted")
	}
}

func TestCenteringCollectionOrderFeedsSequence(t *testing.T) {
	ctx := context.Background()
	f := newCentering(t)
	a, _ := f.collection(t, "A", "a1", "a2")
	b, _ := f.collection(t, "B", "b1")
	f.enable(t, CenteringSettingsPatch{})
	var got []string
	got = append(got, text(f.current(t, "UTC", "")))
	for range 3 {
		l, _ := f.c.Step(ctx, f.alice, 1)
		got = append(got, text(l))
	}
	if want := []string{"a1", "a2", "b1", "a1"}; !slices.Equal(got, want) {
		t.Fatalf("sequence = %v, want %v", got, want)
	}
	// Feed only B.
	if _, err := f.c.UpdateSettings(ctx, f.alice, CenteringSettingsPatch{CollectionIDs: &[]string{b.ID, b.ID}}); err != nil {
		t.Fatal(err)
	}
	if s, _ := f.c.Settings(ctx, f.alice); !slices.Equal(s.CollectionIDs, []string{b.ID}) {
		t.Fatalf("selected = %v", s.CollectionIDs)
	}
	if got := text(f.current(t, "UTC", "")); got != "b1" {
		t.Fatalf("only B = %q", got)
	}
	_ = a
}

func TestQuoteFilesRoundTrip(t *testing.T) {
	ctx := context.Background()
	f := newCentering(t)
	yamlFile := []byte(`collection: Morning
entries:
  - text: Be still, and breathe.
    attribution: A gardener
    tags: [calm, rest]
  - text: |-
      Two lines,
      kept as written.
    reference: Notebook 3
    translation: Plain
`)
	qf, err := ParseQuoteFile(yamlFile, "")
	if err != nil || qf.Collection != "Morning" || len(qf.Entries) != 2 {
		t.Fatalf("parse = %+v, %v", qf, err)
	}
	res, err := f.c.Import(ctx, f.alice, ImportTarget{}, qf, false)
	if err != nil || !res.Created || res.Added != 2 || res.Collection.Name != "Morning" || res.Collection.EntryCount != 2 {
		t.Fatalf("import = %+v, %v", res, err)
	}
	// Imports are additive, into the same collection (found by name).
	res, err = f.c.Import(ctx, f.alice, ImportTarget{Name: "morning"}, qf, false)
	if err != nil || res.Created || res.Collection.EntryCount != 4 {
		t.Fatalf("second import = %+v, %v", res, err)
	}
	// Replace trashes what was there.
	one := QuoteFile{Entries: []QuoteFileEntry{{Text: "Only this."}}}
	res, err = f.c.Import(ctx, f.alice, ImportTarget{CollectionID: res.Collection.ID}, one, true)
	if err != nil || res.Replaced != 4 || res.Collection.EntryCount != 1 {
		t.Fatalf("replace = %+v, %v", res, err)
	}
	if trash, _ := f.c.Trash(ctx, f.alice); len(trash.Entries) != 4 {
		t.Fatalf("replaced entries aren't in the trash: %d", len(trash.Entries))
	}
	if _, err := f.c.Import(ctx, f.alice, ImportTarget{CollectionID: res.Collection.ID}, qf, true); err != nil {
		t.Fatal(err)
	}

	for _, format := range []string{"yaml", "json"} {
		data, name, err := f.c.Export(ctx, f.alice, res.Collection.ID, format)
		if err != nil || name != "morning."+format {
			t.Fatalf("export %s = %q, %v", format, name, err)
		}
		back, err := ParseQuoteFile(data, format)
		if err != nil {
			t.Fatalf("re-parse %s: %v\n%s", format, err, data)
		}
		if back.Collection != "Morning" || len(back.Entries) != 2 || back.Entries[1].Text != "Two lines,\nkept as written." ||
			!slices.Equal(back.Entries[0].Tags, []string{"calm", "rest"}) || back.Entries[1].Translation != "Plain" {
			t.Fatalf("%s round trip = %+v\n%s", format, back, data)
		}
	}
	if _, _, err := f.c.Export(ctx, f.alice, res.Collection.ID, "xml"); err == nil {
		t.Fatal("xml export accepted")
	}

	// JSON in, too.
	qf, err = ParseQuoteFile([]byte(`{"collection":"Evening","entries":[{"text":"Rest now.","attribution":"A gardener"}]}`), "")
	if err != nil || qf.Collection != "Evening" {
		t.Fatalf("json parse = %+v, %v", qf, err)
	}
}

func TestQuoteFileErrorsAreCalm(t *testing.T) {
	ctx := context.Background()
	f := newCentering(t)
	for _, tc := range []struct {
		name, data, want string
	}{
		{"empty", "  \n", "empty"},
		{"unknown field", "entries:\n  - text: hi\n    author: me\n", `field author not found`},
		{"bad yaml", "entries: [\n", "couldn't be read"},
		{"bad json", `{"entries": [}`, "isn't valid"},
		{"json unknown", `{"entries": [{"txt": "x"}]}`, "unknown field"},
		{"two collections", `{"entries": []} {"entries": []}`, "one collection"},
	} {
		_, err := ParseQuoteFile([]byte(tc.data), "")
		var verr *ValidationError
		if !errors.As(err, &verr) || !strings.Contains(verr.Msg, tc.want) || strings.Contains(verr.Msg, "service.") {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	if _, err := ParseQuoteFile(make([]byte, MaxQuoteFileBytes+1), ""); err == nil {
		t.Error("oversized file accepted")
	}

	big := QuoteFile{Collection: "Big"}
	for range MaxImportEntries + 1 {
		big.Entries = append(big.Entries, QuoteFileEntry{Text: "x"})
	}
	data, err := json.Marshal(big)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseQuoteFile(data, "json"); err == nil {
		t.Error("too many entries accepted")
	}
	_, err = f.c.Import(ctx, f.alice, ImportTarget{Name: "Morning"}, QuoteFile{Entries: []QuoteFileEntry{{Text: "ok"}, {Text: " "}}}, false)
	var verr *ValidationError
	if !errors.As(err, &verr) || !strings.HasPrefix(verr.Msg, "Entry 2:") {
		t.Fatalf("bad entry: %v", err)
	}
	if list, _ := f.c.Collections(ctx, f.alice); len(list) != 0 {
		t.Fatal("a failed import left a collection behind")
	}
	if _, err := f.c.Import(ctx, f.alice, ImportTarget{}, QuoteFile{Entries: []QuoteFileEntry{{Text: "ok"}}}, false); err == nil {
		t.Fatal("import with no collection name accepted")
	}
	if _, err := f.c.Import(ctx, f.alice, ImportTarget{Name: "X"}, QuoteFile{}, false); err == nil {
		t.Fatal("import with no entries accepted")
	}
}

func TestPurgeTrashRemovesEntriesAndCollections(t *testing.T) {
	ctx := context.Background()
	f := newCentering(t)
	col, entries := f.collection(t, "Morning", "One", "Two")
	other, _ := f.collection(t, "Evening", "Three")
	if _, err := f.c.SetEntryTrashed(ctx, f.alice, entries[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.c.SetCollectionTrashed(ctx, f.alice, other.ID, true); err != nil {
		t.Fatal(err)
	}
	n, err := PurgeTrash(ctx, f.st, f.now.Add(time.Hour))
	if err != nil || n != 2 {
		t.Fatalf("purged %d, %v", n, err)
	}
	if _, err := f.st.GetQuote(ctx, entries[0].ID); err == nil {
		t.Fatal("trashed entry survived the purge")
	}
	if _, err := f.st.GetQuoteCollection(ctx, other.ID); err == nil {
		t.Fatal("trashed collection survived the purge")
	}
	if got, _ := f.c.Entries(ctx, f.alice, col.ID); len(got) != 1 {
		t.Fatalf("live entries = %d", len(got))
	}
}

func TestExportFileName(t *testing.T) {
	for in, want := range map[string]string{"Morning Light": "morning-light", "  ": "centering-line", "Café — notes!": "caf-notes", "a__b": "a-b"} {
		if got := exportFileName(in); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}
