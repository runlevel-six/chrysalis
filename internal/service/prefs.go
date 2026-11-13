// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"database/sql"
	"errors"
	"slices"

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
	b2i := func(b bool) int64 {
		if b {
			return 1
		}
		return 0
	}
	_, err = p.st.UpsertPrefs(ctx, db.UpsertPrefsParams{
		UserID: userID, Theme: cur.Theme, Mode: cur.Mode, Density: cur.Density, Motion: cur.Motion,
		CompletionFlutter: b2i(cur.CompletionFlutter), HighlightOverdue: b2i(cur.HighlightOverdue),
		UpdatedAt: store.FormatTime(p.b.now()),
	})
	if err == nil {
		p.b.emit(ctx, sse.Event{UserID: userID, Kind: "prefs"})
	}
	return cur, err
}
