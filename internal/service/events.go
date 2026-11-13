// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	"github.com/runlevel-six/chrysalis/internal/sse"
)

// Notifier receives a change notice after a change is committed (live sync,
// docs/DESIGN.md §5.5). Nil means nobody is listening (tests, tools).
type Notifier func(sse.Event)

type originKey struct{}

// WithOrigin records which browser tab made the request (the
// X-Chrysalis-Client header), so its own change notices can be skipped.
func WithOrigin(ctx context.Context, origin string) context.Context {
	return context.WithValue(ctx, originKey{}, origin)
}

// SetNotifier connects the service to the live-sync hub.
func (s *Boards) SetNotifier(n Notifier) { s.notify = n }

func (s *Boards) emit(ctx context.Context, e sse.Event) {
	if s.notify == nil {
		return
	}
	if o, ok := ctx.Value(originKey{}).(string); ok {
		e.Origin = o
	}
	s.notify(e)
}

// emitCard notifies the board a card is on (looked up after the change).
func (s *Boards) emitCard(ctx context.Context, cardID, kind, entityID string) {
	if s.notify == nil {
		return
	}
	c, err := s.st.GetCard(ctx, cardID)
	if err != nil {
		return
	}
	s.emit(ctx, sse.Event{BoardID: c.BoardID, Kind: kind, EntityID: entityID, CardID: cardID})
}
