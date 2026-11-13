// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"sort"
	"time"
)

// Calendar is what the calendar view shows (docs/DESIGN.md §3.8): cards due
// in a date range on one board or every board the user can see, plus the
// boards, columns and labels needed to draw and open them. Undated cards
// come along when asked for, for the side panel.
type Calendar struct {
	Boards  []Board  `json:"boards"`
	Columns []Column `json:"columns"`
	Labels  []Label  `json:"labels"`
	Cards   []Card   `json:"cards"`
	Undated []Card   `json:"undated"`
	// UndatedMore is true when there were more undated cards than were sent.
	UndatedMore bool `json:"undatedMore"`
}

// MaxUndated bounds the side panel; it's a convenience, not an index.
const MaxUndated = 300

// MaxCalendarDays bounds a request: a month view with its spill-over weeks
// is 42 days.
const MaxCalendarDays = 62

// CalendarRange returns cards due from..to (inclusive, YYYY-MM-DD) on boardID
// or, if boardID is "", on every live board the user is a member of.
//
// Timed due dates are stored in UTC, so a card can land on a neighbouring
// local day; the range is widened by a day each way and the client places
// each card on its own local date.
func (s *Boards) CalendarRange(ctx context.Context, userID, boardID, from, to string, undated bool) (Calendar, error) {
	f, err1 := time.Parse(dateOnly, from)
	t, err2 := time.Parse(dateOnly, to)
	if err1 != nil || err2 != nil || t.Before(f) {
		return Calendar{}, &ValidationError{"from and to must be dates (YYYY-MM-DD), from on or before to"}
	}
	if t.Sub(f) > MaxCalendarDays*24*time.Hour {
		return Calendar{}, &ValidationError{"That date range is too long."}
	}
	lo := f.AddDate(0, 0, -1).Format(dateOnly)
	hi := t.AddDate(0, 0, 2).Format(dateOnly) // exclusive upper bound, as a prefix compare

	var boards []Board
	if boardID != "" {
		b, err := s.Get(ctx, userID, boardID)
		if err != nil {
			return Calendar{}, err
		}
		boards = []Board{b}
	} else {
		if boards, err1 = s.List(ctx, userID); err1 != nil {
			return Calendar{}, err1
		}
	}

	out := Calendar{Boards: boards, Columns: []Column{}, Labels: []Label{}, Cards: []Card{}, Undated: []Card{}}
	for _, b := range boards {
		d, err := s.Detail(ctx, userID, b.ID)
		if err != nil {
			return Calendar{}, err
		}
		out.Columns = append(out.Columns, d.Columns...)
		out.Labels = append(out.Labels, d.Labels...)
		for _, c := range d.Cards {
			switch {
			case c.DueAt != nil && *c.DueAt >= lo && *c.DueAt < hi:
				out.Cards = append(out.Cards, c)
			case c.DueAt == nil && undated && c.CompletedAt == nil:
				if len(out.Undated) < MaxUndated {
					out.Undated = append(out.Undated, c)
				} else {
					out.UndatedMore = true
				}
			}
		}
	}
	sort.SliceStable(out.Cards, func(i, j int) bool { return *out.Cards[i].DueAt < *out.Cards[j].DueAt })
	return out, nil
}
