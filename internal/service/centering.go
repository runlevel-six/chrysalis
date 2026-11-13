// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

// The centering line (docs/DESIGN.md §3.13): one quiet line of Scripture or
// a quote at the top of the board. People keep collections of entries; their
// settings and the line's rotation state live server-side, so every device
// shows the same entry (§3.3).

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"

	"github.com/runlevel-six/chrysalis/internal/order"
	"github.com/runlevel-six/chrysalis/internal/sse"
	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

// ErrCollectionForbidden means the user can see the collection but not
// change it (a viewer, or an editor doing something only the owner can).
var ErrCollectionForbidden = errors.New("forbidden: collection")

// Limits on the centering line's content. Entries are short by nature; these
// keep a stray file from filling the database.
const (
	MaxCollectionNameLen     = 100
	MaxCollectionsPerUser    = 100
	MaxEntriesPerCollection  = 5000
	MaxEntryTextLen          = 2000
	MaxEntryAttributionLen   = 200
	MaxEntryReferenceLen     = 200
	MaxEntryTranslationLen   = 50
	MaxEntryTags             = 20
	MaxEntryTagLen           = 40
	maxSeenLoads             = 20
	maxSelectedCollections   = MaxCollectionsPerUser
	maxTimezoneNameLen       = 64
	centeringNoticeKind      = "centering"
	defaultCenteringInterval = "daily"
)

// QuoteCollection is a collection of centering-line entries as the current
// user sees it.
type QuoteCollection struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Position   string `json:"position"`
	Role       Role   `json:"role"`
	EntryCount int64  `json:"entryCount"`
	DeletedAt  string `json:"deletedAt,omitempty"`
	Version    int64  `json:"version"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

// Quote is one entry: a line of Scripture or a quote.
type Quote struct {
	ID           string   `json:"id"`
	CollectionID string   `json:"collectionId"`
	Text         string   `json:"text"`
	Attribution  string   `json:"attribution"`
	Reference    string   `json:"reference"`
	Translation  string   `json:"translation"`
	Tags         []string `json:"tags"`
	Position     string   `json:"position"`
	DeletedAt    string   `json:"deletedAt,omitempty"`
	Version      int64    `json:"version"`
	UpdatedAt    string   `json:"updatedAt"`
}

// CenteringSettings are a person's choices for their line.
type CenteringSettings struct {
	Enabled       bool     `json:"enabled"`
	Mode          string   `json:"mode"`     // rotate | pinned
	Interval      string   `json:"interval"` // page_load | hourly | daily | weekly
	Order         string   `json:"order"`    // sequential | shuffle
	CollectionIDs []string `json:"collectionIds"`
}

// CenteringSettingsPatch changes some settings; nil fields stay as they are.
type CenteringSettingsPatch struct {
	Enabled       *bool     `json:"enabled"`
	Mode          *string   `json:"mode"`
	Interval      *string   `json:"interval"`
	Order         *string   `json:"order"`
	CollectionIDs *[]string `json:"collectionIds"`
}

// CenteringLine is what the line shows right now.
type CenteringLine struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
	// Hidden for today: nothing shows until HiddenUntil (the next local
	// midnight when the person hid it).
	Hidden      bool   `json:"hidden"`
	HiddenUntil string `json:"hiddenUntil,omitempty"`
	Entry       *Quote `json:"entry"`
	Pinned      bool   `json:"pinned"`
	// How many entries feed the line, so the app can say "add some" when
	// there are none.
	PoolSize int `json:"poolSize"`
}

// CenteringTrash lists what can be restored.
type CenteringTrash struct {
	Collections []QuoteCollection `json:"collections"`
	Entries     []Quote           `json:"entries"`
}

// Centering runs the centering line. It shares Boards's clock, transactions
// and live-sync notifier.
type Centering struct {
	b    *Boards
	st   *store.Store
	intn func(n int) int // random choice for shuffle; replaceable in tests
}

func NewCentering(b *Boards) *Centering {
	// Picking the next shuffled entry needs no cryptographic randomness.
	return &Centering{b: b, st: b.st, intn: rand.IntN} // #nosec G404 -- not security sensitive
}

// SetRand replaces the random source used by shuffle (tests).
func (c *Centering) SetRand(intn func(n int) int) { c.intn = intn }

func collectionFromRow(r db.QuoteCollection, role string, count int64) QuoteCollection {
	return QuoteCollection{
		ID: r.ID, Name: r.Name, Position: r.Position, Role: Role(role), EntryCount: count,
		DeletedAt: r.DeletedAt.String, Version: r.Version, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func quoteFromRow(r db.Quote) Quote {
	tags := []string{}
	_ = json.Unmarshal([]byte(r.TagsJson), &tags)
	if tags == nil {
		tags = []string{}
	}
	return Quote{
		ID: r.ID, CollectionID: r.CollectionID, Text: r.Text, Attribution: r.Attribution, Reference: r.Reference,
		Translation: r.Translation, Tags: tags, Position: r.Position, DeletedAt: r.DeletedAt.String,
		Version: r.Version, UpdatedAt: r.UpdatedAt,
	}
}

// collectionRole returns the user's role on a live collection, or
// ErrNotFound. With write set, viewers get ErrCollectionForbidden.
func collectionRole(ctx context.Context, q *db.Queries, userID, collectionID string, write bool) (Role, error) {
	r, err := q.GetQuoteCollectionRole(ctx, db.GetQuoteCollectionRoleParams{UserID: userID, CollectionID: collectionID})
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if write && Role(r) == RoleViewer {
		return "", ErrCollectionForbidden
	}
	return Role(r), nil
}

// notifyMembers tells everyone who can see a collection that it changed.
func (c *Centering) notifyMembers(ctx context.Context, collectionID string) {
	ids, err := c.st.ListQuoteCollectionMemberIDs(ctx, collectionID)
	if err != nil {
		return
	}
	for _, id := range ids {
		c.b.emit(ctx, sse.Event{UserID: id, Kind: centeringNoticeKind, EntityID: collectionID})
	}
}

func (c *Centering) notifyUser(ctx context.Context, userID string) {
	c.b.emit(ctx, sse.Event{UserID: userID, Kind: centeringNoticeKind})
}

// ---- Collections ----

// Collections lists the live collections the user can see, in order.
func (c *Centering) Collections(ctx context.Context, userID string) ([]QuoteCollection, error) {
	rows, err := c.st.ListQuoteCollectionsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]QuoteCollection, 0, len(rows))
	for _, r := range rows {
		out = append(out, collectionFromRow(r.QuoteCollection, r.Role, r.EntryCount))
	}
	return out, nil
}

// CreateCollection makes a new collection owned by the user, after their
// others. It starts feeding their line right away: making a collection is
// the clearest sign they want to see it.
func (c *Centering) CreateCollection(ctx context.Context, userID, name string) (QuoteCollection, error) {
	var out QuoteCollection
	err := c.b.tx(ctx, func(q *db.Queries) error {
		var err error
		out, err = c.createCollection(ctx, q, userID, name)
		return err
	})
	if err == nil {
		c.notifyUser(ctx, userID)
	}
	return out, err
}

func (c *Centering) createCollection(ctx context.Context, q *db.Queries, userID, name string) (QuoteCollection, error) {
	name, err := cleanName(name, "collection", MaxCollectionNameLen)
	if err != nil {
		return QuoteCollection{}, err
	}
	n, err := q.CountQuoteCollectionsForOwner(ctx, userID)
	if err != nil {
		return QuoteCollection{}, err
	}
	if n >= MaxCollectionsPerUser {
		return QuoteCollection{}, &ValidationError{fmt.Sprintf("You can keep up to %d collections.", MaxCollectionsPerUser)}
	}
	pos, err := positionOrAppend(nil, func() (string, error) { return q.LastQuoteCollectionPosition(ctx, userID) })
	if err != nil {
		return QuoteCollection{}, err
	}
	now := store.FormatTime(c.b.now())
	row, err := q.CreateQuoteCollection(ctx, db.CreateQuoteCollectionParams{
		ID: ulid.Make().String(), OwnerID: userID, Name: name, Position: pos, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return QuoteCollection{}, err
	}
	if err := q.AddQuoteCollectionMember(ctx, db.AddQuoteCollectionMemberParams{CollectionID: row.ID, UserID: userID, Role: string(RoleOwner)}); err != nil {
		return QuoteCollection{}, err
	}
	st, err := loadCentering(ctx, q, userID)
	if err != nil {
		return QuoteCollection{}, err
	}
	if len(st.collections) < maxSelectedCollections {
		st.collections = append(st.collections, row.ID)
		if err := st.save(ctx, q, c.b.now()); err != nil {
			return QuoteCollection{}, err
		}
	}
	return collectionFromRow(row, string(RoleOwner), 0), nil
}

// CollectionPatch holds the fields to change; nil means leave alone.
type CollectionPatch struct {
	Name     *string `json:"name"`
	Position *string `json:"position"`
}

// UpdateCollection renames or reorders a collection. Only its owner can.
func (c *Centering) UpdateCollection(ctx context.Context, userID, collectionID string, p CollectionPatch) (QuoteCollection, error) {
	var out QuoteCollection
	err := c.b.tx(ctx, func(q *db.Queries) error {
		r, err := collectionRole(ctx, q, userID, collectionID, true)
		if err != nil {
			return err
		}
		if r != RoleOwner {
			return ErrCollectionForbidden
		}
		row, err := q.GetQuoteCollection(ctx, collectionID)
		if err != nil {
			return err
		}
		params := db.UpdateQuoteCollectionParams{ID: row.ID, Name: row.Name, Position: row.Position, UpdatedAt: store.FormatTime(c.b.now())}
		if p.Name != nil {
			if params.Name, err = cleanName(*p.Name, "collection", MaxCollectionNameLen); err != nil {
				return err
			}
		}
		if p.Position != nil {
			if order.Valid(*p.Position) != nil {
				return &ValidationError{"invalid position"}
			}
			params.Position = *p.Position
		}
		row, err = q.UpdateQuoteCollection(ctx, params)
		if err != nil {
			return err
		}
		n, err := q.CountQuotes(ctx, row.ID)
		out = collectionFromRow(row, string(r), n)
		return err
	})
	if err == nil {
		c.notifyMembers(ctx, collectionID)
	}
	return out, err
}

// SetCollectionTrashed moves a collection to the trash or restores it. Only
// its owner can. Its entries stay as they were and come back with it.
func (c *Centering) SetCollectionTrashed(ctx context.Context, userID, collectionID string, trashed bool) (QuoteCollection, error) {
	var out QuoteCollection
	err := c.b.tx(ctx, func(q *db.Queries) error {
		r, err := q.GetQuoteCollectionMemberRole(ctx, db.GetQuoteCollectionMemberRoleParams{UserID: userID, CollectionID: collectionID})
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if Role(r) != RoleOwner {
			return ErrCollectionForbidden
		}
		now := store.FormatTime(c.b.now())
		del := sql.NullString{}
		if trashed {
			del = sql.NullString{String: now, Valid: true}
		}
		row, err := q.SetQuoteCollectionDeleted(ctx, db.SetQuoteCollectionDeletedParams{ID: collectionID, DeletedAt: del, UpdatedAt: now})
		if err != nil {
			return err
		}
		n, err := q.CountQuotes(ctx, row.ID)
		out = collectionFromRow(row, r, n)
		return err
	})
	if err == nil {
		c.notifyMembers(ctx, collectionID)
	}
	return out, err
}

// Trash lists the user's trashed collections and entries trashed one by one
// from collections they can edit.
func (c *Centering) Trash(ctx context.Context, userID string) (CenteringTrash, error) {
	out := CenteringTrash{Collections: []QuoteCollection{}, Entries: []Quote{}}
	cols, err := c.st.ListTrashedQuoteCollectionsForOwner(ctx, userID)
	if err != nil {
		return out, err
	}
	for _, r := range cols {
		n, err := c.st.CountQuotes(ctx, r.ID)
		if err != nil {
			return out, err
		}
		out.Collections = append(out.Collections, collectionFromRow(r, string(RoleOwner), n))
	}
	entries, err := c.st.ListTrashedQuotesForUser(ctx, userID)
	if err != nil {
		return out, err
	}
	for _, e := range entries {
		out.Entries = append(out.Entries, quoteFromRow(e))
	}
	return out, nil
}

// ---- Entries ----

// Entries lists a collection's live entries in order.
func (c *Centering) Entries(ctx context.Context, userID, collectionID string) ([]Quote, error) {
	if _, err := collectionRole(ctx, c.st.Queries, userID, collectionID, false); err != nil {
		return nil, err
	}
	rows, err := c.st.ListQuotes(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	out := make([]Quote, 0, len(rows))
	for _, r := range rows {
		out = append(out, quoteFromRow(r))
	}
	return out, nil
}

// EntryInput is a new entry. Position nil means the end.
type EntryInput struct {
	Text        string   `json:"text"`
	Attribution string   `json:"attribution"`
	Reference   string   `json:"reference"`
	Translation string   `json:"translation"`
	Tags        []string `json:"tags"`
	Position    *string  `json:"position"`
}

// EntryPatch holds the fields to change; nil means leave alone.
type EntryPatch struct {
	Text        *string   `json:"text"`
	Attribution *string   `json:"attribution"`
	Reference   *string   `json:"reference"`
	Translation *string   `json:"translation"`
	Tags        *[]string `json:"tags"`
	Position    *string   `json:"position"`
}

// cleanEntry is an entry's fields, validated.
type cleanEntry struct {
	text, attribution, reference, translation, tagsJSON string
}

func cleanEntryText(v string) string {
	v = strings.ReplaceAll(v, "\r\n", "\n")
	v = strings.ReplaceAll(v, "\r", "\n")
	return strings.TrimSpace(v)
}

// cleanLine trims a one-line field and folds any line breaks into spaces.
func cleanLine(v string) string {
	return strings.Join(strings.Fields(v), " ")
}

func hasControl(s string, allowNewline bool) bool {
	for _, r := range s {
		if r == '\n' && allowNewline || r == '\t' {
			continue
		}
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func checkLen(v, what string, max int) error {
	if utf8.RuneCountInString(v) > max {
		return &ValidationError{fmt.Sprintf("The %s can be up to %d characters.", what, max)}
	}
	return nil
}

func cleanTags(tags []string) (string, error) {
	out := []string{}
	for _, t := range tags {
		t = cleanLine(t)
		if t == "" || slices.Contains(out, t) {
			continue
		}
		if utf8.RuneCountInString(t) > MaxEntryTagLen {
			return "", &ValidationError{fmt.Sprintf("Tags can be up to %d characters.", MaxEntryTagLen)}
		}
		out = append(out, t)
	}
	if len(out) > MaxEntryTags {
		return "", &ValidationError{fmt.Sprintf("An entry can have up to %d tags.", MaxEntryTags)}
	}
	raw, err := json.Marshal(out)
	return string(raw), err
}

func validateEntry(text, attribution, reference, translation string, tags []string) (cleanEntry, error) {
	e := cleanEntry{text: cleanEntryText(text), attribution: cleanLine(attribution), reference: cleanLine(reference), translation: cleanLine(translation)}
	if e.text == "" {
		return e, &ValidationError{"An entry needs some text."}
	}
	if hasControl(e.text, true) || hasControl(e.attribution, false) || hasControl(e.reference, false) || hasControl(e.translation, false) {
		return e, &ValidationError{"That entry has characters that can't be shown."}
	}
	for _, f := range []struct {
		v, what string
		max     int
	}{
		{e.text, "text", MaxEntryTextLen},
		{e.attribution, "attribution", MaxEntryAttributionLen},
		{e.reference, "reference", MaxEntryReferenceLen},
		{e.translation, "translation", MaxEntryTranslationLen},
	} {
		if err := checkLen(f.v, f.what, f.max); err != nil {
			return e, err
		}
	}
	var err error
	e.tagsJSON, err = cleanTags(tags)
	return e, err
}

// CreateEntry adds an entry to a collection the user can edit.
func (c *Centering) CreateEntry(ctx context.Context, userID, collectionID string, in EntryInput) (Quote, error) {
	e, err := validateEntry(in.Text, in.Attribution, in.Reference, in.Translation, in.Tags)
	if err != nil {
		return Quote{}, err
	}
	var out Quote
	err = c.b.tx(ctx, func(q *db.Queries) error {
		if _, err := collectionRole(ctx, q, userID, collectionID, true); err != nil {
			return err
		}
		n, err := q.CountQuotes(ctx, collectionID)
		if err != nil {
			return err
		}
		if n >= MaxEntriesPerCollection {
			return &ValidationError{fmt.Sprintf("A collection can hold up to %d entries.", MaxEntriesPerCollection)}
		}
		pos, err := positionOrAppend(in.Position, func() (string, error) { return q.LastQuotePosition(ctx, collectionID) })
		if err != nil {
			return err
		}
		row, err := insertQuote(ctx, q, collectionID, e, pos, store.FormatTime(c.b.now()))
		out = quoteFromRow(row)
		return err
	})
	if err == nil {
		c.notifyMembers(ctx, collectionID)
	}
	return out, err
}

func insertQuote(ctx context.Context, q *db.Queries, collectionID string, e cleanEntry, pos, now string) (db.Quote, error) {
	return q.CreateQuote(ctx, db.CreateQuoteParams{
		ID: ulid.Make().String(), CollectionID: collectionID, Text: e.text, Attribution: e.attribution,
		Reference: e.reference, Translation: e.translation, TagsJson: e.tagsJSON, Position: pos,
		CreatedAt: now, UpdatedAt: now,
	})
}

// UpdateEntry edits an entry. Concurrent edits are last-write-wins (§5.5).
func (c *Centering) UpdateEntry(ctx context.Context, userID, entryID string, p EntryPatch) (Quote, error) {
	var out Quote
	err := c.b.tx(ctx, func(q *db.Queries) error {
		row, err := q.GetQuote(ctx, entryID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && row.DeletedAt.Valid) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := collectionRole(ctx, q, userID, row.CollectionID, true); err != nil {
			return err
		}
		cur := quoteFromRow(row)
		text, attribution, reference, translation, tags := cur.Text, cur.Attribution, cur.Reference, cur.Translation, cur.Tags
		if p.Text != nil {
			text = *p.Text
		}
		if p.Attribution != nil {
			attribution = *p.Attribution
		}
		if p.Reference != nil {
			reference = *p.Reference
		}
		if p.Translation != nil {
			translation = *p.Translation
		}
		if p.Tags != nil {
			tags = *p.Tags
		}
		e, err := validateEntry(text, attribution, reference, translation, tags)
		if err != nil {
			return err
		}
		pos := row.Position
		if p.Position != nil {
			if order.Valid(*p.Position) != nil {
				return &ValidationError{"invalid position"}
			}
			pos = *p.Position
		}
		row, err = q.UpdateQuote(ctx, db.UpdateQuoteParams{
			ID: row.ID, Text: e.text, Attribution: e.attribution, Reference: e.reference, Translation: e.translation,
			TagsJson: e.tagsJSON, Position: pos, UpdatedAt: store.FormatTime(c.b.now()),
		})
		out = quoteFromRow(row)
		return err
	})
	if err == nil {
		c.notifyMembers(ctx, out.CollectionID)
	}
	return out, err
}

// SetEntryTrashed moves an entry to the trash or back out.
func (c *Centering) SetEntryTrashed(ctx context.Context, userID, entryID string, trashed bool) (Quote, error) {
	var out Quote
	err := c.b.tx(ctx, func(q *db.Queries) error {
		row, err := q.GetQuote(ctx, entryID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := collectionRole(ctx, q, userID, row.CollectionID, true); err != nil {
			return err
		}
		now := store.FormatTime(c.b.now())
		del := sql.NullString{}
		if trashed {
			del = sql.NullString{String: now, Valid: true}
		}
		row, err = q.SetQuoteDeleted(ctx, db.SetQuoteDeletedParams{ID: entryID, DeletedAt: del, UpdatedAt: now})
		out = quoteFromRow(row)
		return err
	})
	if err == nil {
		c.notifyMembers(ctx, out.CollectionID)
	}
	return out, err
}

// ---- Settings and rotation state ----

// centeringState is a person's user_centering row, with its JSON lists
// decoded.
type centeringState struct {
	row         db.UserCentering
	collections []string
	seenLoads   []string
	shuffleSeen []string
}

func loadCentering(ctx context.Context, q *db.Queries, userID string) (centeringState, error) {
	row, err := q.GetCentering(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		row = db.UserCentering{
			UserID: userID, Mode: "rotate", RotateEvery: defaultCenteringInterval, OrderMode: "sequential",
			CollectionIdsJson: "[]", SeenLoadsJson: "[]", ShuffleSeenJson: "[]",
		}
	} else if err != nil {
		return centeringState{}, err
	}
	s := centeringState{row: row}
	decode := func(raw string) []string {
		var v []string
		if json.Unmarshal([]byte(raw), &v) != nil || v == nil {
			return []string{}
		}
		return v
	}
	s.collections, s.seenLoads, s.shuffleSeen = decode(row.CollectionIdsJson), decode(row.SeenLoadsJson), decode(row.ShuffleSeenJson)
	return s, nil
}

func (s *centeringState) save(ctx context.Context, q *db.Queries, now time.Time) error {
	enc := func(v []string) string {
		raw, _ := json.Marshal(v)
		return string(raw)
	}
	r := s.row
	return q.UpsertCentering(ctx, db.UpsertCenteringParams{
		UserID: r.UserID, Enabled: r.Enabled, Mode: r.Mode, RotateEvery: r.RotateEvery, OrderMode: r.OrderMode,
		CollectionIdsJson: enc(s.collections), PinnedQuoteID: r.PinnedQuoteID, CurrentQuoteID: r.CurrentQuoteID,
		CurrentPeriod: r.CurrentPeriod, SeenLoadsJson: enc(s.seenLoads), ShuffleSeenJson: enc(s.shuffleSeen),
		HiddenUntil: r.HiddenUntil, UpdatedAt: store.FormatTime(now),
	})
}

// visibleCollections keeps the selected collections the user can still see,
// in their saved order.
func visibleCollections(ctx context.Context, q *db.Queries, userID string, ids []string) ([]string, error) {
	rows, err := q.ListQuoteCollectionsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	live := map[string]bool{}
	for _, r := range rows {
		live[r.QuoteCollection.ID] = true
	}
	out := []string{}
	for _, id := range ids {
		if live[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

func (s centeringState) settings(collections []string) CenteringSettings {
	return CenteringSettings{
		Enabled: s.row.Enabled != 0, Mode: s.row.Mode, Interval: s.row.RotateEvery, Order: s.row.OrderMode,
		CollectionIDs: collections,
	}
}

// Settings returns the person's centering-line settings.
func (c *Centering) Settings(ctx context.Context, userID string) (CenteringSettings, error) {
	st, err := loadCentering(ctx, c.st.Queries, userID)
	if err != nil {
		return CenteringSettings{}, err
	}
	cols, err := visibleCollections(ctx, c.st.Queries, userID, st.collections)
	if err != nil {
		return CenteringSettings{}, err
	}
	return st.settings(cols), nil
}

// UpdateSettings changes the person's settings. Switching to pinned pins
// the entry showing now; switching back to rotate carries on from it.
func (c *Centering) UpdateSettings(ctx context.Context, userID string, p CenteringSettingsPatch) (CenteringSettings, error) {
	var out CenteringSettings
	err := c.b.tx(ctx, func(q *db.Queries) error {
		st, err := loadCentering(ctx, q, userID)
		if err != nil {
			return err
		}
		pick := func(v *string, allowed []string, field string, dst *string) error {
			if v == nil {
				return nil
			}
			if !slices.Contains(allowed, *v) {
				return &ValidationError{field + " has an unknown value"}
			}
			*dst = *v
			return nil
		}
		if p.Enabled != nil {
			st.row.Enabled = b2i(*p.Enabled)
		}
		oldInterval, oldOrder := st.row.RotateEvery, st.row.OrderMode
		if err := pick(p.Interval, []string{"page_load", "hourly", "daily", "weekly"}, "interval", &st.row.RotateEvery); err != nil {
			return err
		}
		if err := pick(p.Order, []string{"sequential", "shuffle"}, "order", &st.row.OrderMode); err != nil {
			return err
		}
		if st.row.RotateEvery != oldInterval {
			// Start counting from the new interval's current period, without
			// changing what's showing now.
			st.row.CurrentPeriod = ""
		}
		if st.row.OrderMode != oldOrder {
			st.shuffleSeen = []string{}
		}
		if p.CollectionIDs != nil {
			ids := []string{}
			for _, id := range *p.CollectionIDs {
				if !slices.Contains(ids, id) {
					ids = append(ids, id)
				}
			}
			if len(ids) > maxSelectedCollections {
				return &ValidationError{"too many collections"}
			}
			visible, err := visibleCollections(ctx, q, userID, ids)
			if err != nil {
				return err
			}
			if len(visible) != len(ids) {
				return &ValidationError{"That collection isn't one you can see."}
			}
			st.collections = ids
		}
		mode := st.row.Mode
		if err := pick(p.Mode, []string{"rotate", "pinned"}, "mode", &mode); err != nil {
			return err
		}
		pool, err := c.pool(ctx, q, userID, st.collections)
		if err != nil {
			return err
		}
		switch {
		case mode == "pinned" && st.row.Mode != "pinned":
			id := st.row.CurrentQuoteID.String
			if !st.row.CurrentQuoteID.Valid || pool.index(id) < 0 {
				if len(pool.items) == 0 {
					return &ValidationError{"Add an entry first, then pin it."}
				}
				id = pool.items[0].ID
			}
			st.pin(id)
		case mode == "rotate":
			st.unpin()
		}
		if err := st.save(ctx, q, c.b.now()); err != nil {
			return err
		}
		cols, err := visibleCollections(ctx, q, userID, st.collections)
		out = st.settings(cols)
		return err
	})
	if err == nil {
		c.notifyUser(ctx, userID)
	}
	return out, err
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func (s *centeringState) pin(id string) {
	s.row.Mode = "pinned"
	s.row.PinnedQuoteID = sql.NullString{String: id, Valid: true}
	s.row.CurrentQuoteID = s.row.PinnedQuoteID
}

func (s *centeringState) unpin() {
	s.row.Mode = "rotate"
	s.row.PinnedQuoteID = sql.NullString{}
}

// quotePool is the entries feeding the line, in collection order and then
// entry order.
type quotePool struct {
	items    []db.Quote
	colIndex map[string]int // collection id -> its place in the pool's order
}

func (p quotePool) index(id string) int {
	return slices.IndexFunc(p.items, func(q db.Quote) bool { return q.ID == id })
}

func (p quotePool) has(id string) bool { return p.index(id) >= 0 }

func (c *Centering) pool(ctx context.Context, q *db.Queries, userID string, collections []string) (quotePool, error) {
	p := quotePool{colIndex: map[string]int{}}
	if len(collections) == 0 {
		return p, nil
	}
	rows, err := q.ListQuotePool(ctx, db.ListQuotePoolParams{UserID: userID, CollectionIds: collections})
	if err != nil {
		return p, err
	}
	p.items = rows
	for _, r := range rows {
		if _, ok := p.colIndex[r.CollectionID]; !ok {
			p.colIndex[r.CollectionID] = len(p.colIndex)
		}
	}
	return p, nil
}

// location turns the client's IANA time zone name into a location. Anything
// unknown (or missing) means UTC, so a bad value can only shift when the
// line changes, never break it.
func location(tz string) *time.Location {
	if tz == "" || tz == "Local" || len(tz) > maxTimezoneNameLen {
		return time.UTC
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}

// period names the rotation period now falls in, in the person's own time
// zone. Later periods sort after earlier ones, so comparing the strings says
// whether a boundary has passed.
func period(interval string, now time.Time, loc *time.Location) string {
	t := now.In(loc)
	switch interval {
	case "hourly":
		return t.Format("2006-01-02T15")
	case "weekly":
		// Weeks start on Monday (ISO 8601).
		monday := t.AddDate(0, 0, -((int(t.Weekday()) + 6) % 7))
		return monday.Format(dateOnly)
	case "page_load":
		return ""
	}
	return t.Format(dateOnly)
}

// nextMidnight is the start of tomorrow in loc.
func nextMidnight(now time.Time, loc *time.Location) time.Time {
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, loc)
}

// step returns the entry after (dir 1) or before (dir -1) the current one.
func (c *Centering) step(ctx context.Context, q *db.Queries, st *centeringState, pool quotePool, dir int) (string, error) {
	n := len(pool.items)
	if n == 0 {
		return "", nil
	}
	cur := st.row.CurrentQuoteID.String
	if st.row.OrderMode == "shuffle" {
		// Forget anything that's no longer in the pool.
		seen := []string{}
		for _, id := range st.shuffleSeen {
			if pool.has(id) && !slices.Contains(seen, id) {
				seen = append(seen, id)
			}
		}
		st.shuffleSeen = seen
		i := slices.Index(seen, cur)
		if dir < 0 {
			if i > 0 {
				return seen[i-1], nil
			}
			// Nothing earlier this cycle: fall back to the entry before it.
			return c.sequential(ctx, q, cur, pool, dir)
		}
		if i >= 0 && i+1 < len(seen) {
			return seen[i+1], nil // walking forward again after "previous"
		}
		var fresh []string
		for _, it := range pool.items {
			if !slices.Contains(seen, it.ID) {
				fresh = append(fresh, it.ID)
			}
		}
		if len(fresh) == 0 {
			// Everything has been shown: start a new cycle, without showing
			// the same entry twice in a row.
			st.shuffleSeen = []string{}
			for _, it := range pool.items {
				if it.ID != cur || n == 1 {
					fresh = append(fresh, it.ID)
				}
			}
		}
		pick := fresh[c.intn(len(fresh))]
		st.shuffleSeen = append(st.shuffleSeen, pick)
		return pick, nil
	}
	return c.sequential(ctx, q, cur, pool, dir)
}

// sequential steps through the pool in order, wrapping at the ends. If the
// current entry has left the pool (deleted, say), it carries on from where
// that entry was.
func (c *Centering) sequential(ctx context.Context, q *db.Queries, cur string, pool quotePool, dir int) (string, error) {
	n := len(pool.items)
	if i := pool.index(cur); i >= 0 {
		return pool.items[((i+dir)%n+n)%n].ID, nil
	}
	first, last := pool.items[0].ID, pool.items[n-1].ID
	if cur == "" {
		if dir < 0 {
			return last, nil
		}
		return first, nil
	}
	row, err := q.GetQuote(ctx, cur)
	if errors.Is(err, sql.ErrNoRows) {
		return first, nil
	}
	if err != nil {
		return "", err
	}
	ci, ok := pool.colIndex[row.CollectionID]
	if !ok {
		return first, nil
	}
	less := func(a db.Quote) int { // a compared with the old current entry
		ai := pool.colIndex[a.CollectionID]
		switch {
		case ai != ci:
			return ai - ci
		case a.Position != row.Position:
			return strings.Compare(a.Position, row.Position)
		}
		return strings.Compare(a.ID, row.ID)
	}
	if dir > 0 {
		for _, it := range pool.items {
			if less(it) > 0 {
				return it.ID, nil
			}
		}
		return first, nil
	}
	for i := n - 1; i >= 0; i-- {
		if less(pool.items[i]) < 0 {
			return pool.items[i].ID, nil
		}
	}
	return last, nil
}

func (c *Centering) line(st centeringState, pool quotePool, now time.Time) CenteringLine {
	out := CenteringLine{Enabled: st.row.Enabled != 0, Mode: st.row.Mode, PoolSize: len(pool.items)}
	if st.row.HiddenUntil.Valid {
		if until, err := store.ParseTime(st.row.HiddenUntil.String); err == nil && now.Before(until) {
			out.Hidden, out.HiddenUntil = true, st.row.HiddenUntil.String
			return out
		}
	}
	if i := pool.index(st.row.CurrentQuoteID.String); i >= 0 {
		e := quoteFromRow(pool.items[i])
		out.Entry = &e
		out.Pinned = st.row.Mode == "pinned" && st.row.PinnedQuoteID.String == e.ID
	}
	return out
}

// Current returns what the line shows now, moving on to the next entry when
// a rotation boundary has passed. tz is the client's IANA time zone, which
// sets where "midnight" falls. load identifies one page load (the app sends
// its tab id), so with the page-load interval each load moves the line once
// while refetches from the same page don't.
//
// Moving on here sends no live-sync notice: other devices pick up the new
// entry on their own next load, rather than changing mid-glance (§3.13).
func (c *Centering) Current(ctx context.Context, userID, tz, load string) (CenteringLine, error) {
	var out CenteringLine
	err := c.b.tx(ctx, func(q *db.Queries) error {
		st, err := loadCentering(ctx, q, userID)
		if err != nil {
			return err
		}
		now := c.b.now()
		if st.row.Enabled == 0 {
			out = CenteringLine{Mode: st.row.Mode}
			return nil
		}
		pool, err := c.pool(ctx, q, userID, st.collections)
		if err != nil {
			return err
		}
		out = c.line(st, pool, now)
		if out.Hidden || len(pool.items) == 0 {
			return nil
		}
		if st.row.Mode == "pinned" && pool.has(st.row.PinnedQuoteID.String) {
			return nil
		}
		changed := false
		// Nothing chosen yet, or the entry left the pool.
		advance := !pool.has(st.row.CurrentQuoteID.String)
		if st.row.RotateEvery == "page_load" {
			if load != "" && !slices.Contains(st.seenLoads, load) {
				st.seenLoads = append(st.seenLoads, load)
				if len(st.seenLoads) > maxSeenLoads {
					st.seenLoads = st.seenLoads[len(st.seenLoads)-maxSeenLoads:]
				}
				advance, changed = true, true
			}
		} else {
			p := period(st.row.RotateEvery, now, location(tz))
			switch {
			case st.row.CurrentPeriod == "":
				st.row.CurrentPeriod, changed = p, true
			case p > st.row.CurrentPeriod:
				// Only forward: a device in a time zone that's behind can't
				// pull the line back to yesterday's entry.
				st.row.CurrentPeriod, advance, changed = p, true, true
			}
		}
		if advance {
			next, err := c.step(ctx, q, &st, pool, 1)
			if err != nil {
				return err
			}
			if st.row.Mode == "pinned" {
				st.unpin() // the pinned entry is gone
			}
			st.row.CurrentQuoteID = sql.NullString{String: next, Valid: next != ""}
			changed = true
		}
		if changed {
			if err := st.save(ctx, q, now); err != nil {
				return err
			}
		}
		out = c.line(st, pool, now)
		return nil
	})
	return out, err
}

// change loads the person's state, applies fn, saves, and answers with the
// line as it is now. Every device hears about it.
func (c *Centering) change(ctx context.Context, userID string, fn func(q *db.Queries, st *centeringState, pool quotePool) error) (CenteringLine, error) {
	var out CenteringLine
	err := c.b.tx(ctx, func(q *db.Queries) error {
		st, err := loadCentering(ctx, q, userID)
		if err != nil {
			return err
		}
		pool, err := c.pool(ctx, q, userID, st.collections)
		if err != nil {
			return err
		}
		if err := fn(q, &st, pool); err != nil {
			return err
		}
		now := c.b.now()
		if err := st.save(ctx, q, now); err != nil {
			return err
		}
		out = c.line(st, pool, now)
		return nil
	})
	if err == nil {
		c.notifyUser(ctx, userID)
	}
	return out, err
}

// Step shows the next (dir 1) or previous (dir -1) entry, on every device.
// Stepping away from a pinned entry unpins it.
func (c *Centering) Step(ctx context.Context, userID string, dir int) (CenteringLine, error) {
	if dir != 1 && dir != -1 {
		return CenteringLine{}, &ValidationError{"dir must be 1 or -1"}
	}
	return c.change(ctx, userID, func(q *db.Queries, st *centeringState, pool quotePool) error {
		next, err := c.step(ctx, q, st, pool, dir)
		if err != nil {
			return err
		}
		st.unpin()
		st.row.CurrentQuoteID = sql.NullString{String: next, Valid: next != ""}
		return nil
	})
}

// Pin keeps one entry on the line until it's unpinned: entryID, or the one
// showing now when entryID is empty.
func (c *Centering) Pin(ctx context.Context, userID, entryID string) (CenteringLine, error) {
	return c.change(ctx, userID, func(_ *db.Queries, st *centeringState, pool quotePool) error {
		if entryID == "" {
			entryID = st.row.CurrentQuoteID.String
		}
		if !pool.has(entryID) {
			return &ValidationError{"That entry isn't on your centering line."}
		}
		st.pin(entryID)
		return nil
	})
}

// Unpin goes back to rotating, carrying on from the pinned entry.
func (c *Centering) Unpin(ctx context.Context, userID string) (CenteringLine, error) {
	return c.change(ctx, userID, func(_ *db.Queries, st *centeringState, _ quotePool) error {
		st.unpin()
		return nil
	})
}

// HideForToday hides the line on every device until the next local
// midnight in tz.
func (c *Centering) HideForToday(ctx context.Context, userID, tz string) (CenteringLine, error) {
	return c.change(ctx, userID, func(_ *db.Queries, st *centeringState, _ quotePool) error {
		until := nextMidnight(c.b.now(), location(tz))
		st.row.HiddenUntil = sql.NullString{String: store.FormatTime(until), Valid: true}
		return nil
	})
}

// Unhide brings the line back (the undo for HideForToday).
func (c *Centering) Unhide(ctx context.Context, userID string) (CenteringLine, error) {
	return c.change(ctx, userID, func(_ *db.Queries, st *centeringState, _ quotePool) error {
		st.row.HiddenUntil = sql.NullString{}
		return nil
	})
}
