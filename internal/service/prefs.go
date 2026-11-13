// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/runlevel-six/chrysalis/internal/sse"
	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

// Prefs are a person's own settings, the same on every device.
type Prefs struct {
	Theme             string `json:"theme"`   // a built-in or branding theme id
	Mode              string `json:"mode"`    // system | light | dark
	Density           string `json:"density"` // comfortable | compact
	Motion            string `json:"motion"`  // system | reduce | allow
	CompletionFlutter bool   `json:"completionFlutter"`
	HighlightOverdue  bool   `json:"highlightOverdue"`
	// RemindersEnabled turns reminders on (§3.9); off by default.
	RemindersEnabled bool `json:"remindersEnabled"`
	// TimeZone is the person's IANA zone, as their browser reports it.
	TimeZone string `json:"timeZone"`
	// QuietStart and QuietEnd ("HH:MM", local) bound when nothing is pushed;
	// both empty means no quiet hours.
	QuietStart string `json:"quietStart"`
	QuietEnd   string `json:"quietEnd"`
}

// DefaultPrefs follow calm-by-default (§3.6).
var DefaultPrefs = Prefs{Theme: "default", Mode: "system", Density: "comfortable", Motion: "system", CompletionFlutter: true}

// PrefsService stores preferences. knownThemes reports whether a theme id
// exists (built-in or from branding).
type PrefsService struct {
	st          *store.Store
	b           *Boards
	knownThemes func(id string) bool
}

func NewPrefs(st *store.Store, b *Boards, knownThemes func(id string) bool) *PrefsService {
	return &PrefsService{st: st, b: b, knownThemes: knownThemes}
}

func (p *PrefsService) Get(ctx context.Context, userID string) (Prefs, error) {
	row, err := p.st.GetPrefs(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultPrefs, nil
	}
	if err != nil {
		return Prefs{}, err
	}
	return Prefs{
		Theme: row.Theme, Mode: row.Mode, Density: row.Density, Motion: row.Motion,
		CompletionFlutter: row.CompletionFlutter != 0, HighlightOverdue: row.HighlightOverdue != 0,
		RemindersEnabled: row.RemindersEnabled != 0, TimeZone: row.TimeZone,
		QuietStart: row.QuietStart, QuietEnd: row.QuietEnd,
	}, nil
}

// PrefsPatch changes some preferences; nil fields stay as they are.
type PrefsPatch struct {
	Theme             *string `json:"theme"`
	Mode              *string `json:"mode"`
	Density           *string `json:"density"`
	Motion            *string `json:"motion"`
	CompletionFlutter *bool   `json:"completionFlutter"`
	HighlightOverdue  *bool   `json:"highlightOverdue"`
	RemindersEnabled  *bool   `json:"remindersEnabled"`
	TimeZone          *string `json:"timeZone"`
	QuietStart        *string `json:"quietStart"`
	QuietEnd          *string `json:"quietEnd"`
}

func (p *PrefsService) Update(ctx context.Context, userID string, patch PrefsPatch) (Prefs, error) {
	cur, err := p.Get(ctx, userID)
	if err != nil {
		return Prefs{}, err
	}
	check := func(v *string, allowed []string, field string, dst *string) error {
		if v == nil {
			return nil
		}
		if !slices.Contains(allowed, *v) {
			return &ValidationError{field + " has an unknown value"}
		}
		*dst = *v
		return nil
	}
	if patch.Theme != nil {
		if !p.knownThemes(*patch.Theme) {
			return Prefs{}, &ValidationError{"That theme isn't available."}
		}
		cur.Theme = *patch.Theme
	}
	if err := check(patch.Mode, []string{"system", "light", "dark"}, "mode", &cur.Mode); err != nil {
		return Prefs{}, err
	}
	if err := check(patch.Density, []string{"comfortable", "compact"}, "density", &cur.Density); err != nil {
		return Prefs{}, err
	}
	if err := check(patch.Motion, []string{"system", "reduce", "allow"}, "motion", &cur.Motion); err != nil {
		return Prefs{}, err
	}
	if patch.CompletionFlutter != nil {
		cur.CompletionFlutter = *patch.CompletionFlutter
	}
	if patch.HighlightOverdue != nil {
		cur.HighlightOverdue = *patch.HighlightOverdue
	}
	if patch.RemindersEnabled != nil {
		cur.RemindersEnabled = *patch.RemindersEnabled
	}
	if patch.TimeZone != nil {
		tz, ok := cleanZone(*patch.TimeZone)
		if !ok {
			return Prefs{}, &ValidationError{"That time zone isn't known."}
		}
		cur.TimeZone = tz
	}
	for _, q := range []struct {
		v   *string
		dst *string
	}{{patch.QuietStart, &cur.QuietStart}, {patch.QuietEnd, &cur.QuietEnd}} {
		if q.v == nil {
			continue
		}
		if *q.v != "" && !validClock(*q.v) {
			return Prefs{}, &ValidationError{"Quiet hours must be times like 22:00."}
		}
		*q.dst = *q.v
	}
	b2i := func(b bool) int64 {
		if b {
			return 1
		}
		return 0
	}
	_, err = p.st.UpsertPrefs(ctx, db.UpsertPrefsParams{
		UserID: userID, Theme: cur.Theme, Mode: cur.Mode, Density: cur.Density, Motion: cur.Motion,
		CompletionFlutter: b2i(cur.CompletionFlutter), HighlightOverdue: b2i(cur.HighlightOverdue),
		RemindersEnabled: b2i(cur.RemindersEnabled), TimeZone: cur.TimeZone,
		QuietStart: cur.QuietStart, QuietEnd: cur.QuietEnd,
		UpdatedAt: store.FormatTime(p.b.now()),
	})
	if err == nil {
		p.b.emit(ctx, sse.Event{UserID: userID, Kind: "prefs"})
	}
	return cur, err
}

// validClock reports whether s is a 24-hour "HH:MM".
func validClock(s string) bool {
	_, err := time.Parse("15:04", s)
	return err == nil && len(s) == 5
}
